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
	return amount, ""
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
	if err := db.Where("id = ? AND type = ?", id, repairServiceTypeVal).First(&rr).Error; err != nil {
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

// resolveTechnicianSite 由维修师（本地用户 id 或 IAM sub）解析其所属网点/租户。
func resolveTechnicianSite(db *gorm.DB, technicianID string) (string, string, bool) {
	var sm models.SiteMember
	if err := db.Where("user_id = ? AND status = ?", technicianID, "active").First(&sm).Error; err != nil {
		var u models.User
		if err := db.Where("iam_sub = ?", technicianID).First(&u).Error; err != nil {
			return "", "", false
		}
		if err := db.Where("user_id = ? AND status = ?", u.ID, "active").First(&sm).Error; err != nil {
			return "", "", false
		}
	}
	return sm.SiteID, sm.TenantID, true
}

func localUserIDBySub(db *gorm.DB, sub string) string {
	var u models.User
	if err := db.Select("id").Where("iam_sub = ?", sub).First(&u).Error; err == nil {
		return u.ID
	}
	return ""
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
	tid := middleware.GetTenantID(ctx)
	if tid == "" || rr.TenantID != tid {
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

// Create POST /api/user/repair-services
func (h *RepairServiceHandler) Create(c *gin.Context) {
	var body struct {
		Description      string   `json:"description"`
		Photos           []string `json:"photos"`
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
	rr.TenantID = profile.TenantID
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
	query := db.Where("user_id = ? AND type = ?", userID, repairServiceTypeVal)
	// RS-API-6：状态过滤（逗号分隔），供分状态列表/分组
	if statusParam := c.Query("status"); statusParam != "" {
		query = query.Where("status IN ?", strings.Split(statusParam, ","))
	}
	if err := query.Order("created_at DESC").Find(&list).Error; err != nil {
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
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": out, "total": len(out)}})
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
	// RS-API-3：寄件地址（所选网点）；未选师时无 site
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
	// RS-API-4：状态时间线（RS-12 详情规格）
	var timeline []models.RepairRequestRecord
	db.Where("repair_request_id = ?", rr.ID).Order("created_at ASC, id ASC").Find(&timeline)
	data["timeline"] = timeline
	// RS-API-5：支付/结算汇总（已付 / 待补缴 / 退款 + 明细）
	var recs []models.OrderPaymentRecord
	db.Where("order_id = ? AND order_type = ?", rr.ID, "repair").Order("created_at ASC").Find(&recs)
	type svcPaymentItem struct {
		ID        string    `json:"id"`
		Kind      string    `json:"kind"` // payment | refund
		Status    string    `json:"status"`
		Amount    int64     `json:"amount_cents"`
		Method    string    `json:"method,omitempty"`
		CreatedAt time.Time `json:"created_at"`
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
			Amount: int64(r.Amount), Method: m, CreatedAt: r.CreatedAt})
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
	siteID, tenantID, ok := resolveTechnicianSite(db, body.TechnicianID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "technician not found"})
		return
	}
	tid := body.TechnicianID
	if err := db.Model(&models.RepairRequest{}).Where("id = ?", rr.ID).
		Updates(map[string]interface{}{
			"technician_id": tid,
			"site_id":       siteID,
			"tenant_id":     tenantID,
			"updated_at":    time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to select technician"})
		return
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "technician_selected", "选择维修师")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "site_id": siteID}})
}

// Quote POST /api/repair-services/:id/quote （师傅/员工）
func (h *RepairServiceHandler) Quote(c *gin.Context) {
	var body struct {
		QuoteRepairCents    int64 `json:"quote_repair_cents"`
		QuoteLogisticsCents int64 `json:"quote_logistics_cents"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.QuoteRepairCents < 0 {
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
		fmt.Sprintf("报价：修理费 %d 分，物流预估 %d 分", body.QuoteRepairCents, body.QuoteLogisticsCents))
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id":      rr.ID,
		"status":  models.RepairReqStatusPendingPay,
		"payable": body.QuoteRepairCents + body.QuoteLogisticsCents,
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
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": rr.ID, "status": models.RepairReqStatusShipping}})
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
	case models.RepairReqStatusShipping, models.RepairReqStatusRepairing, models.RepairReqStatusDoneRepair, models.RepairReqStatusPaid:
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
		fmt.Sprintf("第 %d 段物流费 %d 分", body.Leg, body.LogisticsFeeCents))
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
	// 乐器已寄出（shipping）或维修中（repairing）均可发起加价——计划未设「开始维修」
	// 端点，首次加价发生在师傅收货时（RS-06 主流程 7），故 shipping 必须可加价。
	switch rr.Status {
	case models.RepairReqStatusShipping, models.RepairReqStatusRepairing:
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
		fmt.Sprintf("发起加价：新总价 %d 分，到此为止 %d 分", body.NewQuoteCents, body.IncurredCents))
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
	case models.RepairReqStatusPaid, models.RepairReqStatusShipping, models.RepairReqStatusRepairing:
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

	// 实际应付 = 修理费基准 + Σ各段实填物流费（含本次末段费）
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
	actual := repairServiceRepairOnly(rr) + legsTotal

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
	settleNote := fmt.Sprintf("发回结算：应收 %d 分，已付 %d 分", actual, prepaid)
	if v, ok := result["refund_cents"]; ok {
		settleNote += fmt.Sprintf("，退 %v 分", v)
	}
	if v, ok := result["shortfall_cents"]; ok {
		settleNote += fmt.Sprintf("，补缴 %v 分", v)
	}
	appendRepairServiceTimeline(db, rr.ID, middleware.GetUserID(ctx), "settled", settleNote)
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

	query := db.Model(&models.RepairRequest{}).Where("type = ?", repairServiceTypeVal)
	switch scope {
	case "mine":
		// 指派给我的（technician_id 存本地 users.id；兼容直接存 IAM sub 的历史行）
		me := localUserIDBySub(db, middleware.GetUserID(ctx))
		if me == "" {
			me = middleware.GetUserID(ctx)
		}
		query = query.Where("technician_id = ?", me)
	case "site":
		// #1974 T1：服务单可能无 site（师傅直属商户）→ 网点账号也可见本租户的无 site 单
		if orgID := middleware.GetOrgID(ctx); orgID != "" {
			query = query.Where("(site_id = ? OR site_id IS NULL)", orgID)
			if tid := middleware.GetTenantID(ctx); tid != "" {
				query = query.Where("tenant_id = ?", tid)
			}
		} else if tid := middleware.GetTenantID(ctx); tid != "" {
			query = query.Where("tenant_id = ?", tid)
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
	var list []models.RepairRequest
	if err := query.Order("updated_at DESC").Limit(200).Find(&list).Error; err != nil {
		log.Printf("[RepairService.ListTasks] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list tasks"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}
