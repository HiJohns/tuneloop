package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// #1942 维修服务（单项服务商品，type='service'）后端：
//
//	用户创建（pending_quote，尚无网点/租户）→ 选维修师（回填 site/tenant）
//	→ 师傅报价（pending_payment）→ 用户接受并支付（paid，微信回调置位）
//	→ 用户寄出（shipping）→ 分段物流实填 → 师傅完工（complete → done_repair）
//	→ 网点员工发回并结算（closed，多退少补）→ 用户评价
//
// 加价（docs/cases/repair-service.md RS-06）：repairing → adjust_pending
//   - `new_quote_cents` = 新修理费**总价**；`incurred_cents` = **到此为止已发生**的修理费
//   - accept → 补差 = 新总价 − 原报价修理费（虚拟商品支付，回调 → repairing）
//   - decline → 停止修理 → done_repair，结算修理费基准 = incurred
//
// 结算（RS-08）：actual = 修理费基准（accept→新总价 / decline→incurred / 无加价→原报价）
// + Σ各段实填物流费；prepaid = Σ已付；多退少补。
type RepairServiceHandler struct{}

func NewRepairServiceHandler() *RepairServiceHandler { return &RepairServiceHandler{} }

const repairServiceTypeVal = "service"

// genRepairCode 生成 6 位唯一维修编码（大写字母+数字），warranty 类为 NULL。
func genRepairCode(db *gorm.DB) string {
	for i := 0; i < 10; i++ {
		code := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))[:6]
		var n int64
		db.Model(&models.RepairRequest{}).Where("repair_code = ?", code).Count(&n)
		if n == 0 {
			return code
		}
	}
	return strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))[:8]
}

// repairServicePaymentAmount 支付阶段权威应付金额（分，服务端重算，客户端金额不可信）。
//
//   - 初付（pending_payment）：报价修理费 + 物流费预估
//   - 加价补差（adjust_pending）：`new_quote_cents − quote_repair_cents`
//     —— **补差价**，不是新总价全额（RS-06；审计 F2）
//
// 优惠码/折扣导致的已付与报价差额由结算「多退少补」兜底。
func repairServicePaymentAmount(rr *models.RepairRequest) (models.Cents, string) {
	if rr.QuoteRepairCents == nil {
		return 0, "quote missing"
	}
	if rr.Status == models.RepairReqStatusAdjustPending {
		if rr.AdjustedQuoteCents == nil {
			return 0, "adjustment amount missing"
		}
		diff := *rr.AdjustedQuoteCents - *rr.QuoteRepairCents
		if diff < 0 {
			diff = 0
		}
		return diff, ""
	}
	amount := *rr.QuoteRepairCents
	if rr.QuoteLogisticsCents != nil {
		amount += *rr.QuoteLogisticsCents
	}
	amount += repairServiceMaterialCents(rr) // #2085：料钱
	return amount, ""
}

// repairServiceMaterialCents 报价料钱（nil → 0，#2085）
func repairServiceMaterialCents(rr *models.RepairRequest) models.Cents {
	if rr.QuoteMaterialCents == nil {
		return 0
	}
	return *rr.QuoteMaterialCents
}

// repairServiceActualCents 结算实际应付 = 修理费基准 + 料钱 + Σ物流段实填（#2085）
func repairServiceActualCents(rr *models.RepairRequest, legsTotal models.Cents) models.Cents {
	return repairServiceRepairOnly(rr) + repairServiceMaterialCents(rr) + legsTotal
}

// repairServiceRepairOnly 结算用修理费基准（RS-08）：
// 用户拒绝加价 → incurred（到此为止）；已加价 → 新总价；否则原报价修理费。
func repairServiceRepairOnly(rr *models.RepairRequest) models.Cents {
	if rr.QuoteStatus == "declined" && rr.IncurredRepairCents != nil {
		return *rr.IncurredRepairCents
	}
	if rr.AdjustedQuoteCents != nil {
		return *rr.AdjustedQuoteCents
	}
	if rr.QuoteRepairCents != nil {
		return *rr.QuoteRepairCents
	}
	return 0
}

func loadRepairService(db *gorm.DB, id string) (*models.RepairRequest, bool) {
	var rr models.RepairRequest
	q := db
	// #2134：staff 上下文免除自动租户范围，显式按商户范围 {tid, oid} 过滤
	// （写入链可能存叶组织 id，见 repairServiceTenantScope 注释）。
	if db.Statement != nil {
		if scopes := repairServiceTenantScope(db.Statement.Context); len(scopes) > 0 {
			q = db.WithContext(database.IdentityCtx(db.Statement.Context)).Where("tenant_id IN ?", scopes)
		}
	}
	if err := q.Where("id = ? AND type = ?", id, repairServiceTypeVal).First(&rr).Error; err != nil {
		return nil, false
	}
	return &rr, true
}

// isRepairStaffRole 网点员工/师傅/管理员（可执行报价/完工/发回）。
func isRepairStaffRole(role string) bool {
	switch role {
	case "site_admin", "site_member", "worker", "repair_technician", "merchant_admin", "namespace_admin", "OWNER", "ADMIN", "STAFF":
		return true
	}
	return false
}

// resolveTechnicianTenant（#2122）：维修师直属商户——解析其**商户租户**（不再解析站点）。
// users（双键）→ tenant_id；回退 site_members.tenant_id / technician_profiles.tenant_id（存量兼容）。
func resolveTechnicianTenant(db *gorm.DB, technicianID string) (string, bool) {
	var u models.User
	if err := db.Where("id = ? OR iam_sub = ?", technicianID, technicianID).First(&u).Error; err == nil && u.TenantID != "" && u.TenantID != "00000000-0000-0000-0000-000000000000" {
		return u.TenantID, true
	}
	var tp models.TechnicianProfile
	if err := db.Where("user_id = ?", technicianID).First(&tp).Error; err == nil && tp.TenantID != "" {
		return tp.TenantID, true
	}
	var sm models.SiteMember
	if err := db.Where("user_id = ? AND status = ?", technicianID, "active").First(&sm).Error; err == nil && sm.TenantID != "" {
		return sm.TenantID, true
	}
	return "", false
}

func localUserIDBySub(db *gorm.DB, sub string) string {
	var u models.User
	// #2090（#2078/#2079 同族）：身份键（iam_sub）查询必须免租户作用域——
	// 自注册用户本地行 tenant_id=零UUID，员工上下文中会被 addTenantScope 过滤；
	// 过滤后回落 IAM sub 会与消息列表（按本地 users.id 查询）口径不一致。
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := db.WithContext(database.IdentityCtx(ctx)).Select("id").Where("iam_sub = ?", sub).First(&u).Error; err == nil {
		return u.ID
	}
	return ""
}

// repairServiceTenantScope（#2134）：维修服务单的 staff 商户范围口径。
//
// 实测拓扑（预生产）：组织均为**顶级组织**（parent_id=NULL），beaconiam 对所选组织
// 签发 tid = ResolveRootOrg(org) = 组织自身 → staff 的 tid=oid=本组织（如卡丹萨
// bd6bfa4b）。而服务单 tenant 有两种历史/现行形态：
//
//	① 叶组织 id（写入链旧行为：technician_profiles.tenant_id=bd6bfa4b）
//	② 本商户所属租户（merchants.org_id=bd6bfa4b → tenant_id=3cfa99da，规范化后）
//
// 因此可见集合 = {tid, oid} ∪ {merchants(org_id=oid).tenant_id}——涵盖两种形态，
// 且不引入跨商户泄漏（都是「本组织/本商户」维度）。
// 返回空集表示无组织信息（顾客上下文）——不追加显式过滤。
func repairServiceTenantScope(ctx context.Context) []string {
	tid := middleware.GetTenantID(ctx)
	oid := middleware.GetOrgID(ctx)
	scopes := make([]string, 0, 3)
	if tid != "" {
		scopes = append(scopes, tid)
	}
	if oid != "" && oid != tid {
		scopes = append(scopes, oid)
	}
	// #2134 修订2：本组织所属商户的租户（卡丹萨形态：tid=oid=叶组织，服务单落父租户）
	if oid != "" {
		var m models.Merchant
		if err := database.GetDB().WithContext(database.IdentityCtx(ctx)).Select("tenant_id").
			Where("org_id = ?", oid).First(&m).Error; err == nil && m.TenantID != "" &&
			m.TenantID != tid && m.TenantID != oid {
			scopes = append(scopes, m.TenantID)
		}
	}
	return scopes
}

// normalizeRepairTenant（#2134）：把「叶组织 id」规范化为「根租户 id」。
// merchants 表编码了 org_id → tenant_id 映射（本商户数据 tenant_id=org_id 混用，
// 见 #2125）。查不到映射时原样返回，避免破坏既有正确数据。
func normalizeRepairTenant(db *gorm.DB, tenantOrOrg string) string {
	if tenantOrOrg == "" {
		return ""
	}
	ctx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}
	var m models.Merchant
	if err := db.WithContext(database.IdentityCtx(ctx)).Select("tenant_id").
		Where("org_id = ?", tenantOrOrg).First(&m).Error; err == nil && m.TenantID != "" {
		return m.TenantID
	}
	return tenantOrOrg
}

// appendRepairServiceTimeline 写入维修服务单时间线（RS-API-4，#1961）。
// 复用 v3 `repair_request_records` 表（已注册 modelsToValidate/testfixtures，无迁移）：
// `record_type` 承载迁移类型（created/quoted/paid/leg_fee/settled...），
// `worker_id` 存操作者（IAM sub 或 "system"）。写失败仅记日志不阻断主流程。
func appendRepairServiceTimeline(db *gorm.DB, repairID, operatorID, recordType, comment string) {
	if err := db.Create(&models.RepairRequestRecord{
		ID:              uuid.New().String(),
		RepairRequestID: repairID,
		WorkerID:        operatorID,
		RecordType:      recordType,
		Comment:         comment,
		CreatedAt:       time.Now(),
	}).Error; err != nil {
		log.Printf("[RepairService.Timeline] write failed for %s (%s): %v", repairID, recordType, err)
	}
}

// repairServiceStaffAllowed 员工/师傅操作目标服务单的归属校验（审计 F1，#688 清单）：
// 目标单的 tenant/site 必须与 JWT 的 tid/oid 匹配；未选定网点（tenant 为空）的服务单
// 不允许任何 staff 操作。namespace/merchant 级（oid 空）只校验租户。
func repairServiceStaffAllowed(rr *models.RepairRequest, ctx context.Context) bool {
	if rr.TenantID == "" {
		return false
	}
	// #2134：接受根租户(tid) 或 用户自身组织(oid) 两种历史口径
	scopes := repairServiceTenantScope(ctx)
	if len(scopes) == 0 {
		return false
	}
	matched := false
	for _, s := range scopes {
		if rr.TenantID == s {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	// #1974 T1（2026-09-18）：服务单**无 site 维度**（师傅直属商户）→ 租户匹配即可；
	// 若历史/异常单带 site，则网点级账号仍需匹配 site
	if rr.SiteID != "" {
		if oid := middleware.GetOrgID(ctx); oid != "" && rr.SiteID != oid {
			return false
		}
	}
	return true
}

// isAssignedTechnician（#2118 修订）：technician_id 可能存本地 users.id 或 IAM sub
// （#2090 双形态，66771F 实测）——双键匹配，与 ListTasks scope=mine 口径一致。
// resolveLocalUserIDAny（#2124）：任意身份键（本地 id / IAM sub）→ **本地 users.id**。
// 消息列表（GetNotifications）按本地 id 查询（#1742 口径）——通知 user_id 必须落本地 id，
// 否则顾客永远看不到（#2090 双形态实测：rr.UserID 存 IAM sub）。
// parseTimeParam（#2128）：解析 ISO8601 时间参数（兼容带/不带毫秒与 Z/偏移）。
func parseTimeParam(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func resolveLocalUserIDAny(db *gorm.DB, anyID string) string {
	if anyID == "" || anyID == "system" {
		return anyID
	}
	var u models.User
	if err := db.WithContext(database.IdentityCtx(context.Background())).
		Where("id = ? OR iam_sub = ?", anyID, anyID).
		Select("id").First(&u).Error; err == nil && u.ID != "" {
		return u.ID
	}
	return anyID
}

// yuanCents 分→元字符串（保留两位，时间线展示用；#2122）
func yuanCents(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func isAssignedTechnician(rr *models.RepairRequest, uid string, db *gorm.DB) bool {
	if rr.TechnicianID == nil || *rr.TechnicianID == "" {
		return false
	}
	if *rr.TechnicianID == uid {
		return true
	}
	var u models.User
	if err := db.Select("id").Where("iam_sub = ?", uid).First(&u).Error; err == nil {
		return *rr.TechnicianID == u.ID
	}
	return false
}

// resolveUserNamesByAnyID（#2120）：按 id/iam_sub 双键批量解析用户名
// （ListTasks 行与详情时间线操作者共用；豁免租户作用域——零租户顾客行 #2078 同族）。
func resolveUserNamesByAnyID(db *gorm.DB, ids []string) map[string]string {
	nameByUser := map[string]string{}
	clean := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || id == "system" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nameByUser
	}
	var us []models.User
	if err := db.WithContext(database.IdentityCtx(context.Background())).
		Where("id IN ? OR iam_sub IN ?", clean, clean).
		Select("id, iam_sub, name, nickname, username").Find(&us).Error; err == nil {
		for _, u := range us {
			n := u.Name
			if n == "" {
				n = u.Nickname
			}
			if n == "" {
				n = u.Username
			}
			nameByUser[u.ID] = n
			if u.IAMSub != "" {
				nameByUser[u.IAMSub] = n
			}
		}
	}
	return nameByUser
}

// Create POST /api/user/repair-services
func (h *RepairServiceHandler) Create(c *gin.Context) {
	var body struct {
		Description      string   `json:"description"`
		Photos           []string `json:"photos"`
		Video            string   `json:"video"` // #2060: 试奏视频 file_key（可选，≤1 段）
		UserInstrumentID string   `json:"user_instrument_id"`
		TechnicianID     string   `json:"technician_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "authentication required"})
		return
	}
	if strings.TrimSpace(body.Description) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "description is required"})
		return
	}
	// #2060: 试奏视频可选；仅做长度校验（与 photos 的宽松口径一致，不做存在性强校验）
	if len(strings.TrimSpace(body.Video)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "video 链接过长（≤500）"})
		return
	}
	if body.UserInstrumentID != "" {
		var ui models.UserInstrument
		if err := db.Where("id = ? AND user_id = ?", body.UserInstrumentID, userID).First(&ui).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "user instrument not found or not owned by caller"})
			return
		}
	}

	code := genRepairCode(db)
	now := time.Now()
	rr := models.RepairRequest{
		ID:               uuid.New().String(),
		UserID:           userID,
		UserInstrumentID: body.UserInstrumentID,
		Status:           models.RepairReqStatusPendingQuote,
		Type:             repairServiceTypeVal,
		RepairCode:       &code,
		Description:      body.Description,
		VideoURL:         strings.TrimSpace(body.Video), // #2060: 可选试奏视频
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if body.Photos != nil {
		if b, err := json.Marshal(body.Photos); err == nil {
			rr.Photos = string(b)
		}
	}
	// 创建时可选指定维修师 → 回填其网点/租户（不指定则保持 NULL，选师时回填）
	// #1974 T1（2026-09-18 设计变更）：创建即**锁定师傅**（必填），师傅**直属商户**
	// → 回填 technician_id + tenant_id（**不再有 site 维度**）
	if strings.TrimSpace(body.TechnicianID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "technician_id is required"})
		return
	}
	var profile models.TechnicianProfile
	if err := db.Where("user_id = ? AND status = ?", body.TechnicianID, "active").First(&profile).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "technician not found or inactive"})
		return
	}
	// #2134：规范化叶组织→根租户（写入链曾把商户 org_id 存进 tenant_id）
	rr.TenantID = normalizeRepairTenant(db, profile.TenantID)
	tid := body.TechnicianID
	rr.TechnicianID = &tid
	// 未选维修师时无网点/租户归属：uuid 列不可写空串，必须 Omit 以存 NULL
	// （迁移 20260917003 已放开 site_id/tenant_id NOT NULL）。
	create := db
	var omit []string
	// 分别判空：师傅直属商户（#1974 T1）下 tenant_id 有值而 site_id 为空 → 只 Omit site_id
	if rr.SiteID == "" {
		omit = append(omit, "site_id")
	}
	if rr.TenantID == "" {
		omit = append(omit, "tenant_id")
	}
	if rr.UserInstrumentID == "" {
		omit = append(omit, "user_instrument_id")
	}
	if len(omit) > 0 {
		create = db.Omit(omit...)
	}
	if err := create.Create(&rr).Error; err != nil {
		log.Printf("[RepairService.Create] create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create repair service"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, userID, "created", "创建维修服务单")
	// #2090：通知锁定的维修师（有新单待报价）；tenant_id 为 not null uuid，空值不发
	if rr.TenantID != "" && rr.TechnicianID != nil && *rr.TechnicianID != "" {
		services.Notify(db, rr.TenantID, *rr.TechnicianID, "repair", "有新维修单待报价",
			fmt.Sprintf("顾客提交了维修服务单（%s），请及时报价。", code), rr.ID, "repair_service", "repair_svc_quote")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id":          rr.ID,
		"repair_code": code,
		"status":      rr.Status,
	}})
}

// ListMine GET /api/user/repair-services
func (h *RepairServiceHandler) ListMine(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "authentication required"})
		return
	}
	var list []models.RepairRequest
	query := db.Model(&models.RepairRequest{}).Where("user_id = ? AND type = ?", userID, repairServiceTypeVal)
	// RS-API-6：状态过滤（逗号分隔），供分状态列表/分组
	if statusParam := c.Query("status"); statusParam != "" {
		query = query.Where("status IN ?", strings.Split(statusParam, ","))
	}
	// 排序：提交时间/最后更新时间 × 正/逆序（默认提交时间逆序）
	orderClause := "created_at DESC"
	switch c.DefaultQuery("sort", "-created_at") {
	case "created_at":
		orderClause = "created_at ASC"
	case "updated_at":
		orderClause = "updated_at ASC"
	case "-updated_at":
		orderClause = "updated_at DESC"
	}
	// #2170: 服务端分页 + total
	var total int64
	query.Count(&total)
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)
	if err := query.Order(orderClause).Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list repair services"})
		return
	}
	// RS-12 列表待办提示数据源：每单已评价标记 + 待补缴额（#1955 分状态列表）
	ids := make([]string, 0, len(list))
	for _, rr := range list {
		ids = append(ids, rr.ID)
	}
	reviewedSet := map[string]bool{}
	shortfallMap := map[string]int64{}
	if len(ids) > 0 {
		var reviews []models.RepairReview
		db.Where("repair_id IN ?", ids).Find(&reviews)
		for _, rv := range reviews {
			reviewedSet[rv.RepairID] = true
		}
		type shortfallRow struct {
			OrderID string
			Total   int64
		}
		var shorts []shortfallRow
		db.Model(&models.OrderPaymentRecord{}).
			Where("order_id IN ? AND order_type = ? AND status = ? AND method = ?", ids, "repair", "pending", "shortfall").
			Select("order_id, COALESCE(SUM(amount), 0) AS total").Group("order_id").Scan(&shorts)
		for _, s := range shorts {
			shortfallMap[s.OrderID] = s.Total
		}
	}
	out := make([]gin.H, 0, len(list))
	for _, rr := range list {
		b, err := json.Marshal(rr)
		if err != nil {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		m["reviewed"] = reviewedSet[rr.ID]
		m["pending_shortfall_cents"] = shortfallMap[rr.ID]
		out = append(out, m)
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": out, "total": total, "page": page, "page_size": pageSize}})
}

// Get GET /api/user/repair-services/:id （顾客本人或员工）
func (h *RepairServiceHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	userID := middleware.GetUserID(ctx)
	role := middleware.GetRole(ctx)
	// 双上下文可见性（RS-API-3）：顾客按本人归属（JWT 无 tid/oid）；
	// 员工按 JWT 归属（有 tid/oid，repairServiceStaffAllowed），不得跨租户读。
	if rr.UserID != userID {
		if !isRepairStaffRole(role) || !repairServiceStaffAllowed(rr, ctx) {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
			return
		}
	}
	var fees []models.RepairLogisticsFee
	db.Where("repair_id = ?", rr.ID).Order("leg ASC").Find(&fees)
	var review models.RepairReview
	db.Where("repair_id = ?", rr.ID).First(&review)
	data := gin.H{"repair": rr, "logistics_fees": fees}
	if review.ID != "" {
		data["review"] = review
	}
	// RS-API-3：寄件地址
	// - 历史/异常单带 site → 网点地址（兼容）
	// - #1974 T1（师傅直属商户，2026-09-18）：新单无 site → **商户地址**（用户寄件地址来源）
	if rr.SiteID != "" {
		var site models.Site
		if err := db.Where("id = ?", rr.SiteID).First(&site).Error; err == nil {
			data["site"] = gin.H{
				"id":           site.ID,
				"name":         site.Name,
				"address":      site.Address,
				"contact_name": site.ContactName,
				"phone":        site.Phone,
			}
		}
	}
	// #2122：商户始终返回（新模型：维修师直属商户 → 寄件主地址=商户，不再依赖 site）
	if rr.TenantID != "" {
		var m models.Merchant
		// #2125：容错——部分商户/技师数据的 tenant 值实为其 org_id（06A3E5 实测）
		if err := db.Where("tenant_id = ? OR org_id = ?", rr.TenantID, rr.TenantID).Order("created_at ASC").First(&m).Error; err == nil {
			data["merchant"] = gin.H{
				"id":           m.ID,
				"name":         m.Name,
				"address":      m.Address,
				"contact_name": m.ContactName,
				"phone":        m.ContactPhone,
			}
		}
	}
	// #2122：指派维修师名（寄件面板「收件人」）+ 顾客收件信息（寄回面板目的地）
	if rr.TechnicianID != nil && *rr.TechnicianID != "" {
		names := resolveUserNamesByAnyID(db, []string{*rr.TechnicianID})
		if n := names[*rr.TechnicianID]; n != "" {
			data["technician"] = gin.H{"id": *rr.TechnicianID, "name": n}
		}
	}
	if rr.UserID != "" {
		custName := ""
		custPhone := ""
		if names := resolveUserNamesByAnyID(db, []string{rr.UserID}); names[rr.UserID] != "" {
			custName = names[rr.UserID]
		}
		var cu models.User
		cdb := db.WithContext(database.IdentityCtx(context.Background()))
		if err := cdb.Where("id = ? OR iam_sub = ?", rr.UserID, rr.UserID).First(&cu).Error; err == nil {
			if custName == "" {
				custName = cu.Name
			}
			custPhone = cu.Phone
		}
		custAddr := ""
		if cu.ID != "" {
			var addr models.UserAddress
			if err := cdb.Where("user_id = ?", cu.ID).Order("is_default DESC, created_at DESC").First(&addr).Error; err == nil {
				parts := []string{addr.Province, addr.City, addr.District, addr.Detail}
				nonEmpty := make([]string, 0, len(parts))
				for _, pp := range parts {
					if pp != "" {
						nonEmpty = append(nonEmpty, pp)
					}
				}
				custAddr = strings.Join(nonEmpty, "")
				if addr.RecipientName != "" && custName == "" {
					custName = addr.RecipientName
				}
				if addr.Phone != "" {
					custPhone = addr.Phone
				}
			}
		}
		data["customer"] = gin.H{"name": custName, "phone": custPhone, "address": custAddr}
	}
	// RS-API-4：状态时间线（RS-12 详情规格）
	var timeline []models.RepairRequestRecord
	db.Where("repair_request_id = ?", rr.ID).Order("created_at ASC, id ASC").Find(&timeline)
	// #2120：时间线操作者名（worker_id 双键解析；system 直显）
	workerIDs := make([]string, 0, len(timeline))
	for _, t := range timeline {
		workerIDs = append(workerIDs, t.WorkerID)
	}
	opNames := resolveUserNamesByAnyID(db, workerIDs)
	tl := make([]map[string]interface{}, 0, len(timeline))
	for _, t := range timeline {
		b, _ := json.Marshal(t)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		op := "系统"
		if t.WorkerID != "" && t.WorkerID != "system" {
			if n := opNames[t.WorkerID]; n != "" {
				op = n
			}
		}
		m["operator"] = op
		tl = append(tl, m)
	}
	data["timeline"] = tl
	// RS-API-5：支付/结算汇总（已付 / 待补缴 / 退款 + 明细）
	var recs []models.OrderPaymentRecord
	db.Where("order_id = ? AND order_type = ?", rr.ID, "repair").Order("created_at ASC").Find(&recs)
	type svcPaymentItem struct {
		ID             string    `json:"id"`
		Kind           string    `json:"kind"` // payment | refund
		Status         string    `json:"status"`
		Amount         int64     `json:"amount_cents"`
		CouponDiscount int64     `json:"coupon_discount_cents"` // #2120：优惠码抵扣（对照订单详情标准）
		Method         string    `json:"method,omitempty"`
		CreatedAt      time.Time `json:"created_at"`
	}
	items := []svcPaymentItem{}
	payIDs := make([]string, 0, len(recs))
	var madeCents, pendingShortfall int64
	for _, r := range recs {
		m := ""
		if r.Method != nil {
			m = *r.Method
		}
		items = append(items, svcPaymentItem{ID: r.ID, Kind: "payment", Status: r.Status,
			Amount: int64(r.Amount), CouponDiscount: int64(r.CouponDiscount), Method: m, CreatedAt: r.CreatedAt})
		payIDs = append(payIDs, r.ID)
		if r.Status == "paid" {
			madeCents += int64(r.Amount)
		}
		if r.Status == "pending" && m == "shortfall" {
			pendingShortfall += int64(r.Amount)
		}
	}
	var refundCents int64
	if len(payIDs) > 0 {
		var refunds []models.OrderRefundRecord
		db.Where("payment_record_id IN ?", payIDs).Order("created_at ASC").Find(&refunds)
		for _, rf := range refunds {
			items = append(items, svcPaymentItem{ID: rf.ID, Kind: "refund", Status: rf.Status,
				Amount: int64(rf.Amount), CreatedAt: rf.CreatedAt})
			if rf.Status == "refunded" || rf.Status == "refunding" {
				refundCents += int64(rf.Amount)
			}
		}
	}
	data["payments"] = gin.H{
		"made_cents":              madeCents,
		"pending_shortfall_cents": pendingShortfall,
		"refund_cents":            refundCents,
		"records":                 items,
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": data})
}

// SelectTechnician POST /api/user/repair-services/:id/select-technician
func (h *RepairServiceHandler) SelectTechnician(c *gin.Context) {
	var body struct {
		TechnicianID string `json:"technician_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "technician_id is required"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusPendingQuote {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "technician can only be selected before quoting"})
		return
	}
	// #2122：维修师直属商户 → 只回填商户租户，**不再挂靠网点**（清空 site_id）
	tenantID, ok := resolveTechnicianTenant(db, body.TechnicianID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "technician not found"})
		return
	}
	tenantID = normalizeRepairTenant(db, tenantID) // #2134：叶组织→根租户
	tid := body.TechnicianID
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"technician_id": tid,
			"site_id":       nil,
			"tenant_id":     tenantID,
			"updated_at":    time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to select technician"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "technician_selected", "选择维修师")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID}})
}

// Quote POST /api/repair-services/:id/quote （师傅/员工）
func (h *RepairServiceHandler) Quote(c *gin.Context) {
	var body struct {
		QuoteRepairCents    int64 `json:"quote_repair_cents"`
		QuoteLogisticsCents int64 `json:"quote_logistics_cents"`
		QuoteMaterialCents  int64 `json:"quote_material_cents"` // #2085 料钱
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.QuoteRepairCents < 0 || body.QuoteMaterialCents < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid quote"})
		return
	}
	ctx := c.Request.Context()
	role := middleware.GetRole(ctx)
	if !isRepairStaffRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.Status != models.RepairReqStatusPendingQuote {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "quote is only allowed in pending_quote"})
		return
	}
	// 报价必须先选师（才有网点/租户归属），且操作者须属该网点（F1）
	if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	now := time.Now()
	updates := map[string]interface{}{
		"quote_repair_cents":    models.Cents(body.QuoteRepairCents),
		"quote_logistics_cents": models.Cents(body.QuoteLogisticsCents),
		"quote_material_cents":  models.Cents(body.QuoteMaterialCents), // #2085
		"quote_status":          "pending",
		"status":                models.RepairReqStatusPendingPay,
		"updated_at":            now,
	}
	if rr.TechnicianID == nil {
		// 报价人即维修师（本地用户 id 缺省时回落 IAM sub）
		localID := localUserIDBySub(db, middleware.GetUserID(ctx))
		if localID == "" {
			localID = middleware.GetUserID(ctx)
		}
		updates["technician_id"] = localID
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to quote"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "quoted",
		fmt.Sprintf("报价：修理费 %s 元，料钱 %s 元，物流预估 %s 元", yuanCents(body.QuoteRepairCents), yuanCents(body.QuoteMaterialCents), yuanCents(body.QuoteLogisticsCents)))
	// #2090：通知顾客（报价已提交，请查看并支付）
	if rr.TenantID != "" {
		customerID := localUserIDBySub(db, rr.UserID)
		if customerID == "" {
			customerID = rr.UserID
		}
		codeStr := ""
		if rr.RepairCode != nil {
			codeStr = *rr.RepairCode
		}
		services.Notify(db, rr.TenantID, customerID, "repair", "您的维修单已报价",
			fmt.Sprintf("维修单（%s）已报价，请查看并确认支付。", codeStr), rr.ID, "repair_service", "repair_svc_review")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id":      rr.ID,
		"status":  models.RepairReqStatusPendingPay,
		"payable": body.QuoteRepairCents + body.QuoteMaterialCents + body.QuoteLogisticsCents,
	}})
}

// AcceptQuote POST /api/user/repair-services/:id/accept （用户接受报价，随后调用 /pay/prepay）
func (h *RepairServiceHandler) AcceptQuote(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusPendingPay || rr.QuoteStatus != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "no pending quote to accept"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{"quote_status": "accepted", "updated_at": time.Now()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to accept quote"})
		return
	}
	amount, msg := repairServicePaymentAmount(rr)
	if msg != "" {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": msg})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "quote_accepted", "用户接受报价")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "payable_cents": amount}})
}

// repairQuoteDeclineReasons 拒绝理由枚举 → 中文（#2093；前端同枚举）
var repairQuoteDeclineReasons = map[string]string{
	"too_expensive": "太贵了",
	"found_other":   "已找别人修了",
	"solved":        "问题已解决",
	"other":         "其他",
}

// DeclineQuote POST /api/user/repair-services/:id/quote/decline （用户拒绝报价，终态 cancelled）
// #2093：仅 pending_payment + quote_status=pending 可拒；无支付发生 → 无退款；通知师傅。
func (h *RepairServiceHandler) DeclineQuote(c *gin.Context) {
	var body struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}
	reasonLabel, ok := repairQuoteDeclineReasons[body.Reason]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid decline reason"})
		return
	}
	if len([]rune(body.Note)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "note too long (max 200)"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, okLoad := loadRepairService(db, c.Param("id"))
	if !okLoad {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusPendingPay || rr.QuoteStatus != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "no pending quote to decline"})
		return
	}

	updates := map[string]interface{}{
		"status":               models.RepairReqStatusCancelled,
		"quote_status":         "declined",
		"quote_decline_reason": body.Reason,
		"updated_at":           time.Now(),
	}
	if body.Note != "" {
		updates["quote_decline_note"] = body.Note
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to decline quote"})
		return
	}

	comment := "用户拒绝报价：" + reasonLabel
	if body.Note != "" {
		comment += "（" + body.Note + "）"
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "quote_declined", comment)

	// #2090/#2093：通知师傅（报价被拒绝，含理由）
	if rr.TenantID != "" && rr.TechnicianID != nil && *rr.TechnicianID != "" {
		codeStr := ""
		if rr.RepairCode != nil {
			codeStr = *rr.RepairCode
		}
		services.Notify(db, rr.TenantID, *rr.TechnicianID, "repair", "报价被拒绝",
			fmt.Sprintf("维修单（%s）的报价被顾客拒绝（%s），服务已关闭。", codeStr, reasonLabel), rr.ID, "repair_service", "repair_svc_declined")
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusCancelled}})
}

// Ship POST /api/user/repair-services/:id/ship （用户寄出，填写物流单号）
func (h *RepairServiceHandler) Ship(c *gin.Context) {
	var body struct {
		TrackingCompany string `json:"tracking_company"`
		TrackingNumber  string `json:"tracking_number" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "tracking_number is required"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusPaid {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "shipping is only allowed after payment"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"tracking_company": body.TrackingCompany,
			"tracking_number":  body.TrackingNumber,
			"status":           models.RepairReqStatusShipping,
			"updated_at":       time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update shipping"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "shipped", "用户寄出 "+body.TrackingNumber)
	code := ""
	if rr.RepairCode != nil {
		code = *rr.RepairCode
	}
	if rr.TechnicianID != nil {
		services.Notify(db, rr.TenantID, *rr.TechnicianID, "repair", "顾客已寄出",
			"维修单（"+code+"）顾客已寄出乐器，请留意收货并拍照确认。", rr.ID, "repair_service", "repair_svc_receive")
	}
	actionData := fmt.Sprintf(`{"repair_id":%q}`, rr.ID)
	services.NotifyMerchantAdmins(db, rr.TenantID, "repair", "顾客已寄出",
		"维修单（"+code+"）顾客已寄出乐器，请留意收货。", rr.ID, "repair_service", "info", &actionData)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusShipping}})
}

// Receive POST /api/repair-services/:id/receive （#2116 收货确认：拍照留档）
// 双路径：维修师本人收货 → repairing；商户直属员工代收 → pending_repair（待师傅「开始维修」）。
func (h *RepairServiceHandler) Receive(c *gin.Context) {
	var body struct {
		Photos []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Photos) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "请先拍照留档"})
		return
	}
	if len(body.Photos) > 9 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "收货照片最多 9 张"})
		return
	}
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.Status != models.RepairReqStatusShipping {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "当前状态不可收货确认"})
		return
	}
	uid := middleware.GetUserID(ctx)
	isTech := isAssignedTechnician(rr, uid, db)
	newStatus := models.RepairReqStatusPendingRepair
	timelineType := "received_by_staff"
	timelineMsg := "网点员工代收货（拍照留档），待维修师开始维修"
	if isTech {
		newStatus = models.RepairReqStatusRepairing
		timelineType = "received_by_tech"
		timelineMsg = "维修师确认收货（拍照留档）"
	} else if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	photosJSON, _ := json.Marshal(body.Photos)
	now := time.Now()
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"status":         newStatus,
			"receive_photos": string(photosJSON),
			"received_by":    uid,
			"received_at":    now,
			"updated_at":     now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record receive"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, uid, timelineType, timelineMsg)
	if !isTech && rr.TechnicianID != nil {
		code := ""
		if rr.RepairCode != nil {
			code = *rr.RepairCode
		}
		services.Notify(db, rr.TenantID, *rr.TechnicianID, "repair", "网点已代收货",
			"维修单（"+code+"）乐器已由网点代收，请打开维修单点击「开始维修」。", rr.ID, "repair_service", "repair_svc_start")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": newStatus}})
}

// Start POST /api/repair-services/:id/start （#2116：代收后维修师本人开始维修）
func (h *RepairServiceHandler) Start(c *gin.Context) {
	ctx := c.Request.Context()
	uid := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.Status != models.RepairReqStatusPendingRepair {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "仅待维修状态可开始维修"})
		return
	}
	if !isAssignedTechnician(rr, uid, db) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "仅维修师本人可开始维修"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{"status": models.RepairReqStatusRepairing, "updated_at": time.Now()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to start repair"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, uid, "repair_started", "维修师开始维修")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusRepairing}})
}

// AddLegFee POST /api/repair-services/:id/legs （员工实填某段物流费）
// 请求体：{leg, logistics_fee_cents}（契约见 RS-07）
func (h *RepairServiceHandler) AddLegFee(c *gin.Context) {
	var body struct {
		Leg               int   `json:"leg" binding:"required"`
		LogisticsFeeCents int64 `json:"logistics_fee_cents"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Leg < 1 || body.LogisticsFeeCents < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid leg fee"})
		return
	}
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	switch rr.Status {
	case models.RepairReqStatusShipping, models.RepairReqStatusPendingRepair, models.RepairReqStatusRepairing, models.RepairReqStatusDoneRepair, models.RepairReqStatusPaid:
	default:
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "leg fee not allowed in current status"})
		return
	}
	fee := models.RepairLogisticsFee{
		ID:          uuid.New().String(),
		RepairID:    rr.ID,
		Leg:         body.Leg,
		AmountCents: models.Cents(body.LogisticsFeeCents),
		FilledBy:    middleware.GetUserID(ctx),
		CreatedAt:   time.Now(),
	}
	if err := db.Create(&fee).Error; err != nil {
		log.Printf("[RepairService.AddLegFee] create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to add leg fee"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "leg_fee",
		fmt.Sprintf("第 %d 段物流费 %s 元", body.Leg, yuanCents(body.LogisticsFeeCents)))
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": fee.ID}})
}

// Adjust POST /api/repair-services/:id/adjust （师傅加价）
// 请求体（RS-06）：{new_quote_cents（新修理费总价）, incurred_cents（到此为止修理费）}
func (h *RepairServiceHandler) Adjust(c *gin.Context) {
	var body struct {
		NewQuoteCents int64 `json:"new_quote_cents" binding:"required"`
		IncurredCents int64 `json:"incurred_cents" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.NewQuoteCents < 0 || body.IncurredCents < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "new_quote_cents and incurred_cents are required"})
		return
	}
	if body.IncurredCents > body.NewQuoteCents {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "incurred_cents must not exceed new_quote_cents"})
		return
	}
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	// #2116 修订（用户裁定）：加价只能在**收货之后维修中**发起——
	// 原口径 shipping 可加价是收货环节缺失时期的过渡（发现加价点的前提是已实物收货开修）。
	switch rr.Status {
	case models.RepairReqStatusRepairing:
	default:
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "adjustment is only allowed while shipping or repairing"})
		return
	}
	if rr.QuoteRepairCents == nil {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "quote missing"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"incurred_repair_cents": models.Cents(body.IncurredCents),
			"adjusted_quote_cents":  models.Cents(body.NewQuoteCents),
			"quote_status":          "pending",
			"status":                models.RepairReqStatusAdjustPending,
			"updated_at":            time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to submit adjustment"})
		return
	}
	// 用户继续时应付**差价**（新总价 − 原报价修理费），非全额
	diff := body.NewQuoteCents - int64(*rr.QuoteRepairCents)
	if diff < 0 {
		diff = 0
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "adjust_requested",
		fmt.Sprintf("发起加价：新总价 %s 元，到此为止 %s 元", yuanCents(body.NewQuoteCents), yuanCents(body.IncurredCents)))
	// #2090：通知顾客（加价待确认）
	if rr.TenantID != "" {
		customerID := localUserIDBySub(db, rr.UserID)
		if customerID == "" {
			customerID = rr.UserID
		}
		codeStr := ""
		if rr.RepairCode != nil {
			codeStr = *rr.RepairCode
		}
		services.Notify(db, rr.TenantID, customerID, "repair", "维修单有新的加价申请",
			fmt.Sprintf("维修单（%s）有新的加价申请，请查看并确认。", codeStr), rr.ID, "repair_service", "repair_svc_adjust")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id": rr.ID, "status": models.RepairReqStatusAdjustPending,
		"payable_cents": diff, "incurred_cents": body.IncurredCents,
	}})
}

// AdjustAccept POST /api/user/repair-services/:id/adjust/accept
// 用户同意加价 → 保留 adjust_pending（前端调用 /pay/prepay 支付**差价**，回调置 repairing）
func (h *RepairServiceHandler) AdjustAccept(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusAdjustPending {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "no pending adjustment"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{"quote_status": "accepted", "updated_at": time.Now()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to accept adjustment"})
		return
	}
	payable, msg := repairServicePaymentAmount(rr)
	if msg != "" {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": msg})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "adjust_accepted", "用户同意加价")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": rr.Status, "payable_cents": payable}})
}

// AdjustDecline POST /api/user/repair-services/:id/adjust/decline
// 用户拒绝加价 → 师傅停止修理 → done_repair（待发回）；结算修理费基准 = incurred_cents
func (h *RepairServiceHandler) AdjustDecline(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.UserID != middleware.GetUserID(ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusAdjustPending {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "no pending adjustment"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"quote_status": "declined",
			"status":       models.RepairReqStatusDoneRepair,
			"updated_at":   time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to decline adjustment"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "adjust_declined", "用户拒绝加价，停止修理")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusDoneRepair, "action": "stopped"}})
}

// Complete POST /api/repair-services/:id/complete （师傅完工；契约见 RS-07）
func (h *RepairServiceHandler) Complete(c *gin.Context) {
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	switch rr.Status {
	case models.RepairReqStatusRepairing: // #2116：收货后才可完成修理（shipping 直达完成已关闭）
	default:
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "repair cannot be completed in current status"})
		return
	}
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{"status": models.RepairReqStatusDoneRepair, "updated_at": time.Now()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to complete repair"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "repair_completed", "师傅完成修理")
	code := ""
	if rr.RepairCode != nil {
		code = *rr.RepairCode
	}
	services.Notify(db, rr.TenantID, resolveLocalUserIDAny(db, rr.UserID), "repair", "维修完成",
		"您的维修单（"+code+"）已完成修理，待网点发回结算。", rr.ID, "repair_service", "info")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusDoneRepair}})
}

// Dispatch POST /api/repair-services/:id/dispatch （员工发回末段 + 结算，多退少补）
func (h *RepairServiceHandler) Dispatch(c *gin.Context) {
	var body struct {
		TrackingCompany   string `json:"tracking_company"`
		TrackingNumber    string `json:"tracking_number" binding:"required"`
		LogisticsFeeCents int64  `json:"logistics_fee_cents"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "tracking_number is required"})
		return
	}
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	if rr.Status != models.RepairReqStatusDoneRepair {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "dispatch is only allowed after repair completion"})
		return
	}
	if !repairServiceStaffAllowed(rr, ctx) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	now := time.Now()

	// 实际应付 = 修理费基准 + 料钱 + Σ各段实填物流费（含本次末段费）（#2085）
	// 注意：SUM(numeric) 经 lib/pq 返回 float64，models.Cents.Scan(float64) 会按
	// 「元」再 ×100（cents.go）→ 必须先落 int64 再转换，否则金额放大 100 倍。
	var legsTotalInt int64
	db.Model(&models.RepairLogisticsFee{}).Where("repair_id = ?", rr.ID).
		Select("COALESCE(SUM(amount_cents), 0)").Scan(&legsTotalInt)
	legsTotal := models.Cents(legsTotalInt) + models.Cents(body.LogisticsFeeCents)

	var prepaidInt int64
	db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND type = ? AND status = ?", rr.ID, "repair", "payment", "paid").
		Select("COALESCE(SUM(amount), 0)").Scan(&prepaidInt)
	prepaid := models.Cents(prepaidInt)
	// #2096：优惠折扣不计入结算 actual——否则结算按原价口径会把
	// 顾客已享的优惠码折扣在「多退少补」中吞掉（白用）。
	var couponDiscountInt int64
	db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND type = ? AND status = ?", rr.ID, "repair", "payment", "paid").
		Select("COALESCE(SUM(coupon_discount), 0)").Scan(&couponDiscountInt)
	actual := repairServiceActualCents(rr, legsTotal) - models.Cents(couponDiscountInt)

	result := gin.H{"id": rr.ID, "status": models.RepairReqStatusClosed,
		"actual_cents": actual, "prepaid_cents": prepaid}

	// 退款是不可回滚的外部调用 → **先执行**；失败即中止（不写任何 DB、不闭单），
	// 订单保持 done_repair 可重试。out_refund_no 稳定（`repair_re_<id8>`）→ 重试
	// 时微信按幂等处理，不会重复退款（审计 F6）。
	var refundRecord *models.OrderRefundRecord
	if prepaid > actual {
		diff := prepaid - actual
		var rec models.OrderPaymentRecord
		if err := db.Where("order_id = ? AND order_type = ? AND type = ? AND status = ?", rr.ID, "repair", "payment", "paid").
			Order("created_at DESC").First(&rec).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "paid record not found for refund"})
			return
		}
		outRefundNo := fmt.Sprintf("repair_re_%s", rr.ID[:8])
		refund := models.OrderRefundRecord{
			ID:              uuid.New().String(),
			TenantID:        rr.TenantID,
			PaymentRecordID: &rec.ID,
			OutRefundNo:     &outRefundNo,
			Amount:          diff,
			Reason:          strPtr("维修服务结算退款"),
			Status:          "refunded", // 无线上支付记录（如优惠全免）时视为已退
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if rec.OutTradeNo != nil {
			resp, err := wechatpay.GetClient().Refund(ctx, wechatpay.RefundParams{
				OutTradeNo:   *rec.OutTradeNo,
				OutRefundNo:  outRefundNo,
				TotalAmount:  int64(rec.Amount),
				RefundAmount: int64(diff),
				Reason:       "维修服务结算退款",
				NotifyURL:    wechatpay.GetConfig().RefundNotifyURL,
			})
			if err != nil {
				// 红线：不静默吞错；且不闭单 —— 保持可重试（F6）
				log.Printf("[RepairService.Dispatch] refund failed for %s: %v", rr.ID, err)
				c.JSON(http.StatusBadGateway, gin.H{
					"code":    50200,
					"message": "refund failed, repair remains open for retry: " + err.Error(),
					"data": gin.H{"id": rr.ID, "refund_cents": diff,
						"actual_cents": actual, "prepaid_cents": prepaid},
				})
				return
			}
			refund.RefundID = &resp.RefundID
			refund.Status = "refunding"
		}
		refundRecord = &refund
		result["refund_cents"] = diff
		result["refund_status"] = refund.Status
	}

	// DB 部分（末段物流费 + 退款记录 + 补缴记录 + 闭单）单事务（F6）
	if err := db.Transaction(func(tx *gorm.DB) error {
		var maxLeg int
		if err := tx.Model(&models.RepairLogisticsFee{}).Where("repair_id = ?", rr.ID).
			Select("COALESCE(MAX(leg), 0)").Scan(&maxLeg).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.RepairLogisticsFee{
			ID:          uuid.New().String(),
			RepairID:    rr.ID,
			Leg:         maxLeg + 1,
			AmountCents: models.Cents(body.LogisticsFeeCents),
			FilledBy:    middleware.GetUserID(ctx),
			CreatedAt:   now,
		}).Error; err != nil {
			return err
		}
		if refundRecord != nil {
			if err := tx.Create(refundRecord).Error; err != nil {
				return err
			}
		}
		if actual > prepaid {
			diff := actual - prepaid
			if err := tx.Create(&models.OrderPaymentRecord{
				ID:        uuid.New().String(),
				TenantID:  rr.TenantID,
				UserID:    rr.UserID,
				OrderID:   &rr.ID,
				OrderType: "repair",
				Amount:    diff,
				Type:      "payment",
				Status:    "pending",
				Method:    strPtr("shortfall"),
				CreatedAt: now,
				UpdatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
			Updates(map[string]interface{}{
				"return_company":         body.TrackingCompany,
				"return_tracking_number": body.TrackingNumber,
				"status":                 models.RepairReqStatusClosed,
				"closed_at":              now,
				"updated_at":             now,
			}).Error
	}); err != nil {
		log.Printf("[RepairService.Dispatch] settle tx failed for %s: %v", rr.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to settle repair service"})
		return
	}
	if actual > prepaid {
		result["shortfall_cents"] = actual - prepaid
	}
	settleNote := fmt.Sprintf("发回结算：应收 %s 元，已付 %s 元", yuanCents(int64(actual)), yuanCents(int64(prepaid)))
	if v, ok := result["refund_cents"]; ok {
		settleNote += fmt.Sprintf("，退 %s 元", yuanCents(int64(v.(models.Cents))))
	}
	if v, ok := result["shortfall_cents"]; ok {
		settleNote += fmt.Sprintf("，补缴 %s 元", yuanCents(int64(v.(models.Cents))))
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "settled", settleNote)
	// #2122：实际物流费 > 预估（已付不足）→ 生成可缴费补缴通知（镜像租赁 payment_shortfall，
	// 消息详情「去补缴」→ 支付确认页显示「物流费补缴」）；否则普通发回通知。
	if actual > prepaid {
		diff := actual - prepaid
		ad := map[string]interface{}{
			"shortfall_amount": int64(diff),
			"order_id":         rr.ID,
			"order_type":       "repair_service",
			"label":            "物流费补缴",
		}
		adJSON, _ := json.Marshal(ad)
		notifOrg := rr.SiteID // 服务单无独立 org 维度，沿用 site（空则零 UUID）
		if notifOrg == "" {
			notifOrg = "00000000-0000-0000-0000-000000000000"
		}
		notif := models.Notification{
			TenantID:   rr.TenantID,
			OrgID:      notifOrg,
			UserID:     resolveLocalUserIDAny(db, rr.UserID), // #2124：本地 id（消息列表口径）
			Type:       "payment_shortfall",
			Title:      "维修单需补缴物流费",
			Content:    fmt.Sprintf("维修单已发回（运单号 %s）。实际物流费超出已付，需补缴 ¥%s，补缴完成后订单自动结算。", body.TrackingNumber, yuanCents(int64(diff))),
			RefID:      rr.ID,
			RefType:    "repair",
			ActionType: "payment_shortfall",
			ActionData: strPtr(string(adJSON)),
			Status:     "unread",
			CreatedAt:  now,
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[RepairService.Dispatch] failed to create shortfall notification for %s: %v", rr.ID, err)
		}
	} else {
		services.Notify(db, rr.TenantID, resolveLocalUserIDAny(db, rr.UserID), "repair", "维修单已发回",
			"您的维修单已发回（运单号 "+body.TrackingNumber+"）。"+settleNote, rr.ID, "repair_service", "info")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// Review POST /api/user/repair-services/:id/review
func (h *RepairServiceHandler) Review(c *gin.Context) {
	var body struct {
		Rating  int      `json:"rating" binding:"required"`
		Message string   `json:"message"`
		Photos  []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Rating < 1 || body.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "rating must be between 1 and 5"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rr, ok := loadRepairService(db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair service not found"})
		return
	}
	userID := middleware.GetUserID(ctx)
	if rr.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	if rr.Status != models.RepairReqStatusClosed {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "review is only allowed after settlement"})
		return
	}
	var cnt int64
	db.Model(&models.RepairReview{}).Where("repair_id = ?", rr.ID).Count(&cnt)
	if cnt > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "review already exists"})
		return
	}
	review := models.RepairReview{
		ID:        uuid.New().String(),
		RepairID:  rr.ID,
		UserID:    userID,
		Rating:    body.Rating,
		Message:   body.Message,
		CreatedAt: time.Now(),
	}
	if body.Photos != nil {
		if b, err := json.Marshal(body.Photos); err == nil {
			review.Photos = string(b)
		}
	}
	if err := db.Create(&review).Error; err != nil {
		log.Printf("[RepairService.Review] create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to submit review"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, userID, "reviewed", fmt.Sprintf("评价 %d 星", body.Rating))
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": review.ID}})
}

// ListPendingDispatch GET /api/repair-services/pending-dispatch （员工：待发回清单）
func (h *RepairServiceHandler) ListPendingDispatch(c *gin.Context) {
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	query := db.Where("type = ? AND status = ?", repairServiceTypeVal, models.RepairReqStatusDoneRepair)
	if orgID := middleware.GetOrgID(ctx); orgID != "" {
		query = query.Where("site_id = ?", orgID)
	} else if tenantID := middleware.GetTenantID(ctx); tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	var list []models.RepairRequest
	if err := query.Order("updated_at ASC").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list pending dispatches"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}

// ListTasks GET /api/repair-services?scope=mine|site&status=<csv>
// RS-API-2：师傅/员工任务列表。**员工上下文（JWT 有 tid/oid）**——必须以 JWT
// 作用域：scope=mine → 指派给我（本地用户 id）；scope=site → 本网点（oid，回退 tid）。
func (h *RepairServiceHandler) ListTasks(c *gin.Context) {
	ctx := c.Request.Context()
	if !isRepairStaffRole(middleware.GetRole(ctx)) {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	scope := c.DefaultQuery("scope", "site")

	// #2134：staff 上下文免除自动租户范围，显式按商户范围 {tid, oid} 过滤
	scopes := repairServiceTenantScope(ctx)
	query := db
	if len(scopes) > 0 {
		query = db.WithContext(database.IdentityCtx(ctx)).Where("tenant_id IN ?", scopes)
	}
	query = query.Model(&models.RepairRequest{}).Where("type = ?", repairServiceTypeVal)
	me := ""
	switch scope {
	case "mine":
		// 指派给我的（technician_id 存本地 users.id；兼容直接存 IAM sub 的历史行）
		me = localUserIDBySub(db, middleware.GetUserID(ctx))
		if me == "" {
			me = middleware.GetUserID(ctx)
		}
		query = query.Where("technician_id = ?", me)
	case "site":
		// #2122：维修服务仅商户层级可见——网点账号（oid 非任何商户组织）不涉及维修服务
		oid := middleware.GetOrgID(ctx)
		var merchantOrgCount int64
		db.Model(&models.Merchant{}).Where("org_id = ?", oid).Count(&merchantOrgCount)
		if oid == "" || merchantOrgCount == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": []interface{}{}, "total": 0}})
			return
		}
		// #1974 T1：服务单可能无 site（师傅直属商户）→ 商户账号可见本租户的无 site 单
		if orgID := middleware.GetOrgID(ctx); orgID != "" {
			query = query.Where("(site_id = ? OR site_id IS NULL)", orgID)
			// #2134：显式 {tid, oid} 集合（替代单一 tenant_id = tid，兼容叶组织行）
			if len(scopes) > 0 {
				query = query.Where("tenant_id IN ?", scopes)
			}
		} else if len(scopes) > 0 {
			query = query.Where("tenant_id IN ?", scopes)
		} else {
			// 员工上下文缺组织信息 → 不泄露任何数据（#688）
			c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": []interface{}{}, "total": 0}})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "scope must be mine or site"})
		return
	}
	if statusParam := c.Query("status"); statusParam != "" {
		query = query.Where("status IN ?", strings.Split(statusParam, ","))
	}
	// #2128：可选时间区间（ISO8601；由前端控制显示范围，默认近 30 天）——
	// updated_at ∈ [start, end)（含 start、不含 end）
	if startParam := c.Query("start"); startParam != "" {
		if t, ok := parseTimeParam(startParam); ok {
			query = query.Where("updated_at >= ?", t)
		}
	}
	if endParam := c.Query("end"); endParam != "" {
		if t, ok := parseTimeParam(endParam); ok {
			query = query.Where("updated_at < ?", t)
		}
	}
	// 排序：提交时间/最后更新时间 × 正/逆序（默认提交时间逆序）
	orderClause := "created_at DESC"
	switch c.DefaultQuery("sort", "-created_at") {
	case "created_at":
		orderClause = "created_at ASC"
	case "updated_at":
		orderClause = "updated_at ASC"
	case "-updated_at":
		orderClause = "updated_at DESC"
	}
	var list []models.RepairRequest
	if err := query.Order(orderClause).Limit(200).Find(&list).Error; err != nil {
		log.Printf("[RepairService.ListTasks] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list tasks"})
		return
	}
	// TEMP-DIAG #2134：定位「工作台全 0」——输出上下文与命中行数（确认后移除）
	log.Printf("[RepairService.ListTasks][diag] scope=%s role=%q tid=%q oid=%q me=%q scopes=%v rows=%d",
		scope, middleware.GetRole(ctx), middleware.GetTenantID(ctx), middleware.GetOrgID(ctx), me, scopes, len(list))
	// #2116 修订：工作台瘦身行需要「提交人」——批量回填 user_name（additive 字段）
	userIDs := make([]string, 0, len(list))
	for _, rr := range list {
		if rr.UserID != "" {
			userIDs = append(userIDs, rr.UserID)
		}
	}
	nameByUser := resolveUserNamesByAnyID(db, userIDs)
	rows := make([]gin.H, 0, len(list))
	for _, rr := range list {
		b, _ := json.Marshal(rr)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		m["user_name"] = nameByUser[rr.UserID]
		rows = append(rows, m)
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": rows, "total": len(rows)}})
}
