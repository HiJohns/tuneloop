package handlers

// #2369（#2364 C）：IAM 孤儿账号诊断与人工清理。
//
// 背景：创建用户时本地事务失败 → 回滚尝试 PurgeUser（service 身份）被 IAM 拒绝
// （仅 namespace admin 可 purge）→ 留下 pending 且无本地 users 行的 IAM 账号，
// 同邮箱/用户名重试永远 409。
//
// 孤儿判定（D2，四条缺一不可）：
//  1. IAM status = pending
//  2. IAM user.org_id 为空（未被绑定到具体组织）
//  3. 本地 users 表中不存在 iam_sub = 该 ID 的行（创建后本地事务已回滚）
//  4. 创建时间早于 60 分钟（避免与正在进行的创建流程竞争）
//
// 仅按「无 org 关系」判定是错的：正常邀请制用户在被接受前同样是 pending + org_id 空，
// 但他们有本地 users 行，因此第 3 条是关键区分。

import (
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type IAMOrphanHandler struct{}

func NewIAMOrphanHandler() *IAMOrphanHandler {
	return &IAMOrphanHandler{}
}

// orphanMinAge：创建时间必须早于此值才视为孤儿（D2）。
const orphanMinAge = 60 * time.Minute

// purgeQueueNote：IAM purge 不清理 user_task_queues（D3：tuneloop 不直连 IAM 库）。
const purgeQueueNote = "IAM purge 不会清理 user_task_queues 中遗留的 bind_org/role 任务；" +
	"清理后请按 docs/ops/pending-user-orphan-cleanup.md 在 IAM 侧核对/删除对应队列行。"

type iamOrphanItem struct {
	ID         string     `json:"id"`
	Username   string     `json:"username"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	AgeMinutes int64      `json:"age_minutes"`
}

// isEligibleOrphan 是 D2 四条判定的唯一实现（#2470 D4：语义不变）。
//  1. IAM status = pending
//  2. IAM user.org_id 为空
//  3. 本地 users 表无该 iam_sub
//  4. 创建时间早于 orphanMinAge
func isEligibleOrphan(u *services.User, localExists bool, now time.Time) bool {
	if u == nil || u.ID == "" || u.Status != "pending" {
		return false
	}
	if u.OrgID != "" {
		return false
	}
	if localExists {
		return false
	}
	if u.CreatedAt == nil || now.Sub(*u.CreatedAt) < orphanMinAge {
		return false
	}
	return true
}

// orphanFromPending 按 D2 过滤 IAM 候选列表，并用 localIAMSubs 排除本地存在行。
// 抽成纯函数便于单测覆盖过滤语义（不依赖 IAM/Gin）。
func orphanFromPending(pending []services.User, localIAMSubs map[string]bool, now time.Time) []iamOrphanItem {
	items := make([]iamOrphanItem, 0, len(pending))
	for i := range pending {
		u := &pending[i]
		if !isEligibleOrphan(u, localIAMSubs[u.ID], now) {
			continue
		}
		items = append(items, iamOrphanItem{
			ID:         u.ID,
			Username:   u.Username,
			Name:       u.Name,
			Email:      u.Email,
			Phone:      u.Phone,
			CreatedAt:  u.CreatedAt,
			AgeMinutes: int64(now.Sub(*u.CreatedAt).Minutes()),
		})
	}
	return items
}

// eligibleOrphanIDs 在候选列表内按 D2 四条判定逐 ID 复核（Purge 实时校验，
// #2470 D5：List 与 Purge 必须同源判定）。
func eligibleOrphanIDs(ids []string, candidates []services.User, localIAMSubs map[string]bool, now time.Time) map[string]bool {
	byID := make(map[string]*services.User, len(candidates))
	for i := range candidates {
		if candidates[i].ID != "" {
			byID[candidates[i].ID] = &candidates[i]
		}
	}
	eligible := make(map[string]bool, len(ids))
	for _, id := range ids {
		if isEligibleOrphan(byID[id], localIAMSubs[id], now) {
			eligible[id] = true
		}
	}
	return eligible
}

// localIAMSubSet 返回本地 users 表中已登记的 iam_sub 集合（跨租户，D5 误删防护）。
func localIAMSubSet(db *gorm.DB, ids []string) map[string]bool {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result
	}
	var subs []string
	if err := db.Model(&models.User{}).
		Where("iam_sub IN ? AND iam_sub <> ''", ids).
		Pluck("iam_sub", &subs).Error; err != nil {
		log.Printf("[IAMOrphan] local iam_sub lookup failed: %v", err)
		return result
	}
	for _, s := range subs {
		result[s] = true
	}
	return result
}

// List handles GET /admin/iam-orphans.
//
// 返回当前命名空间内所有孤儿（跨页聚合，D6），不修改任何数据。
func (h *IAMOrphanHandler) List(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	token := services.ExtractUserToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "missing user token"})
		return
	}

	iamClient := services.NewIAMClient()
	// #2470 D1/D2：走「不被活跃关系过滤」的孤儿端点；旧 IAM 自动回退并标记。
	candidates, err := iamClient.ListOrphanCandidatesWithToken(token, int(orphanMinAge.Minutes()))
	if err != nil {
		log.Printf("[IAMOrphan] ListOrphanCandidatesWithToken failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "failed to list IAM orphan candidates: " + err.Error()})
		return
	}
	pending := candidates.Users

	// 跨租户查本地 iam_sub：系统管理员视图必须看到全平台的「已登记」集合，
	// 否则会把其它租户的正常账号误判为孤儿（D5 防线的输入）。
	db := database.GetDB().WithContext(database.IdentityCtx(c.Request.Context()))
	ids := make([]string, 0, len(pending))
	for _, u := range pending {
		if u.ID != "" {
			ids = append(ids, u.ID)
		}
	}
	localSubs := localIAMSubSet(db, ids)

	items := orphanFromPending(pending, localSubs, time.Now())
	scanned := candidates.ScannedPending
	if scanned <= 0 {
		scanned = len(pending)
	}
	endpoint := "orphans"
	if candidates.Unsupported {
		endpoint = "pending"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list":                 items,
			"total":                len(items),
			"scanned":              scanned,
			"min_age_minute":       int64(orphanMinAge.Minutes()),
			"fallback_unsupported": candidates.Unsupported,
			"endpoint":             endpoint,
		},
	})
}

// Purge handles POST /admin/iam-orphans/purge.
//
// 请求：{"user_ids": ["...", "..."]}
// 响应：{"code":20000,"data":{"purged":n,"failed":[{"user_id","error"}],"notes":"..."}}
//
// D5：任何在本地 users 中存在的 ID 一律拒绝（即使 IAM 侧仍为 pending）；
// 非孤儿 ID 不进 purged，而是进 failed（逐条可见，不静默跳过）。
func (h *IAMOrphanHandler) Purge(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}
	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_ids is required"})
		return
	}
	// 去重（重复 ID 不计两次，避免 purged 计数虚高）。
	seen := make(map[string]bool, len(req.UserIDs))
	ids := make([]string, 0, len(req.UserIDs))
	for _, id := range req.UserIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_ids contains no valid id"})
		return
	}

	token := services.ExtractUserToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "missing user token"})
		return
	}

	iamClient := services.NewIAMClient()
	// #2470 D5：Purge 复核必须与 List 同源（同一新方法 + 同一回退）。
	candidates, err := iamClient.ListOrphanCandidatesWithToken(token, int(orphanMinAge.Minutes()))
	if err != nil {
		log.Printf("[IAMOrphan] purge pre-check list failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "failed to verify IAM orphan candidates: " + err.Error()})
		return
	}
	db := database.GetDB().WithContext(database.IdentityCtx(c.Request.Context()))
	localSubs := localIAMSubSet(db, ids)

	// 逐 ID 复核当前（实时）孤儿状态，避免用过期面板数据误删。
	now := time.Now()
	eligible := eligibleOrphanIDs(ids, candidates.Users, localSubs, now)

	// 回退路径下旧列表被「活跃关系」过滤，真正的无关系孤儿可能不在列表内
	// （#2470 D5）：逐 ID 实时取 IAM 用户，仍按四条判定本地复算，绝不因
	// 「旧列表未命中」直接判 not an orphan。
	if candidates.Unsupported {
		listed := make(map[string]bool, len(candidates.Users))
		for _, u := range candidates.Users {
			listed[u.ID] = true
		}
		for _, id := range ids {
			if listed[id] || eligible[id] || localSubs[id] {
				continue
			}
			u, getErr := iamClient.GetUser(id)
			if getErr != nil {
				log.Printf("[IAMOrphan] fallback realtime verify failed for %s: %v", id, getErr)
				continue
			}
			if isEligibleOrphan(u, localSubs[id], now) {
				eligible[id] = true
			}
		}
	}

	purged := 0
	failed := make([]gin.H, 0)
	for _, id := range ids {
		if localSubs[id] {
			failed = append(failed, gin.H{"user_id": id, "error": "user exists locally; refuse to purge (D5)"})
			continue
		}
		if !eligible[id] {
			failed = append(failed, gin.H{"user_id": id, "error": "not an orphan (pending/org/age check failed)"})
			continue
		}
		if err := iamClient.PurgeUserWithToken(token, id); err != nil {
			log.Printf("[IAMOrphan] purge failed for %s: %v", id, err)
			failed = append(failed, gin.H{"user_id": id, "error": err.Error()})
			continue
		}
		purged++
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"purged": purged,
			"failed": failed,
			"notes":  []string{purgeQueueNote},
		},
	})
}
