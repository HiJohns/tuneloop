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

// orphanFromPending 按 D2 过滤 IAM pending 列表，并用 localIAMSubs 排除本地存在行。
// 抽成纯函数便于单测覆盖过滤语义（不依赖 IAM/Gin）。
func orphanFromPending(pending []services.User, localIAMSubs map[string]bool, now time.Time) []iamOrphanItem {
	items := make([]iamOrphanItem, 0, len(pending))
	for _, u := range pending {
		if u.ID == "" || u.Status != "pending" {
			continue
		}
		if u.OrgID != "" {
			continue
		}
		if localIAMSubs[u.ID] {
			continue
		}
		if u.CreatedAt == nil || now.Sub(*u.CreatedAt) < orphanMinAge {
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
	pending, err := iamClient.ListPendingUsersWithToken(token)
	if err != nil {
		log.Printf("[IAMOrphan] ListPendingUsersWithToken failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "failed to list pending IAM users: " + err.Error()})
		return
	}

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
	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list":           items,
			"total":          len(items),
			"scanned":        len(pending),
			"min_age_minute": int64(orphanMinAge.Minutes()),
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
	pending, err := iamClient.ListPendingUsersWithToken(token)
	if err != nil {
		log.Printf("[IAMOrphan] purge pre-check list failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "failed to verify IAM pending users: " + err.Error()})
		return
	}
	db := database.GetDB().WithContext(database.IdentityCtx(c.Request.Context()))
	localSubs := localIAMSubSet(db, ids)

	// 逐 ID 复核当前（实时）孤儿状态，避免用过期面板数据误删。
	eligible := make(map[string]bool, len(ids))
	for _, id := range ids {
		for _, u := range pending {
			if u.ID != id {
				continue
			}
			if u.OrgID == "" && !localSubs[id] && u.CreatedAt != nil && time.Since(*u.CreatedAt) >= orphanMinAge {
				eligible[id] = true
			}
			break
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
