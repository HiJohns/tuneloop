package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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

// #1948 乐器丢失与找回（docs/cases/instrument-loss.md LS-01~LS-06）。
// 员工裁量制：责任方/比例/赔偿额由员工填写；结算/冲正口径见 LS-03/03a/03b/05a。

var lossActiveOrderStatuses = []string{
	models.OrderStatusReserved, models.OrderStatusPaid, models.OrderStatusPendingShipment,
	models.OrderStatusShipped, models.OrderStatusInLease, models.OrderStatusReturning,
	models.OrderStatusDepositRefunding,
}

type InstrumentLossHandler struct{}

func NewInstrumentLossHandler() *InstrumentLossHandler { return &InstrumentLossHandler{} }

// loadLossInstrument 加载乐器并校验操作者归属：JWT tid 必须匹配乐器租户；
// 有 oid（网点级）时要求乐器**当前物理位置**的站点属于该 oid 组织（#2510）。
//
// ID 域（权威：docs/cases/cases.md:289）：JWT oid = sites.org_id；instruments.current_site_id = sites.id。
// 两者是不同主键域，绝不能字符串比较（旧实现即因此把网点账号全部拒掉）。
func loadLossInstrument(c *gin.Context, db *gorm.DB, id string) (*models.Instrument, bool) {
	ctx := c.Request.Context()
	var inst models.Instrument
	if err := db.Where("id = ?", id).First(&inst).Error; err != nil {
		log.Printf("[InstrumentLoss] WARN: instrument %s not found: %v", id, err)
		return nil, false
	}
	tid := middleware.GetTenantID(ctx)
	if tid == "" || inst.TenantID != tid {
		log.Printf("[InstrumentLoss] WARN: tenant mismatch instrument=%s (tenant=%s, operator=%s)", inst.ID, inst.TenantID, tid)
		return nil, false
	}
	if oid := middleware.GetOrgID(ctx); oid != "" && inst.CurrentSiteID != nil {
		// 网点判定 = 「current_site_id 是 oid 组织下的站点之一」（集合成员，非字符串相等）。
		var n int64
		if err := db.Model(&models.Site{}).
			Where("id = ? AND org_id = ?", *inst.CurrentSiteID, oid).
			Count(&n).Error; err != nil {
			log.Printf("[InstrumentLoss] ERROR: resolve site org failed (site=%s, org=%s): %v", *inst.CurrentSiteID, oid, err)
			return nil, false // fail-closed
		}
		if n == 0 {
			log.Printf("[InstrumentLoss] WARN: access denied — instrument %s at site %s is not in operator org %s", inst.ID, *inst.CurrentSiteID, oid)
			return nil, false
		}
	}
	return &inst, true
}

// lossDailyRentCents 取日租金（分）：优先 term_versions 加权日均，旧单回退
// PricingBreakdown.final_daily_rent（折扣后，元）/ MonthlyRent/30。
func lossDailyRentCents(db *gorm.DB, order *models.Order) models.Cents {
	if days, total, ok := termVersionTotals(db, order.ID); ok && days > 0 && total > 0 {
		return total / models.Cents(days)
	}
	if _, finalDaily, err := parsePricingBreakdown(order.PricingBreakdown); err == nil && finalDaily > 0 {
		return models.FromYuan(finalDaily)
	}
	if order.MonthlyRent > 0 {
		return order.MonthlyRent / 30
	}
	return 0
}

// lossDateParse 解析日期字段（DB date 列回读可能带时间后缀或时区）。
func lossDateParse(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s[:min(10, len(s))]); err == nil {
			return t, true
		}
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// lossLeaseStart 解析租约起点：StartDate（date 字符串）优先，回退订单创建时间。
func lossLeaseStart(order *models.Order) time.Time {
	if order.StartDate != nil {
		if t, ok := lossDateParse(*order.StartDate); ok {
			return t
		}
	}
	return order.CreatedAt
}

// lossReturningTs 返程起点（LS-03a 场景③）：order_status_history 的 →returning
// 时间戳优先，回退 ReturnedAt / 订单更新时间（Work Summary 已声明口径）。
func lossReturningTs(db *gorm.DB, order *models.Order) time.Time {
	var h models.OrderStatusHistory
	if err := db.Where("order_id = ? AND status_to = ?", order.ID, models.OrderStatusReturning).
		Order("changed_at DESC").First(&h).Error; err == nil {
		return h.ChangedAt
	}
	if order.ReturnedAt != nil {
		return *order.ReturnedAt
	}
	return order.UpdatedAt
}

// lossEndDate 计算场景租金截止时间（LS-03a）：
// ①去程（未签收）→ 0（不走此函数）；②in_lease → 丢失日；③returning → 返程起点。
func lossRentToDateCents(db *gorm.DB, order *models.Order, lossDate time.Time) (models.Cents, string) {
	daily := lossDailyRentCents(db, order)
	if daily <= 0 {
		return 0, ""
	}
	start := lossLeaseStart(order)
	switch order.Status {
	case models.OrderStatusInLease:
		return models.Cents(daily) * models.Cents(services.CalculateLeaseDays(start, lossDate)), "in_lease"
	case models.OrderStatusReturning:
		end := lossReturningTs(db, order)
		return models.Cents(daily) * models.Cents(services.CalculateLeaseDays(start, end)), "returning"
	default:
		// 去程（reserved/paid/pending_shipment/shipped）：未签收不计租（LS-03a 场景①）
		return 0, "pre_lease"
	}
}

// Register POST /api/instruments/:id/lost（LS-01/LS-02，员工/管理员）
func (h *InstrumentLossHandler) Register(c *gin.Context) {
	var body struct {
		Description       string   `json:"description" binding:"required"`
		ResponsibleParty  string   `json:"responsible_party" binding:"required"`
		UserRatio         int      `json:"user_ratio"`
		CompensationCents int64    `json:"compensation_cents"`
		UserBurdenCents   int64    `json:"user_burden_cents"`
		Photos            []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ResponsibleParty == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "description and responsible_party are required"})
		return
	}
	switch body.ResponsibleParty {
	case "user", "logistics", "platform", "site":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "responsible_party must be user/logistics/platform/site"})
		return
	}
	if body.UserRatio < 0 || body.UserRatio > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "user_ratio must be 0-100"})
		return
	}
	if body.CompensationCents < 0 || body.UserBurdenCents < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "amounts must be >= 0"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	inst, ok := loadLossInstrument(c, db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "instrument not found or access denied"})
		return
	}
	if inst.StockStatus == models.StockStatusLost {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "instrument is already marked lost"})
		return
	}
	// 未结丢失记录防重（未恢复/未冲正的记录存在 → 409）
	var openCount int64
	db.Model(&models.InstrumentLossRecord{}).
		Where("instrument_id = ? AND restored_at IS NULL", inst.ID).Count(&openCount)
	if openCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "an unresolved loss record exists"})
		return
	}

	operator := middleware.GetUserID(ctx)
	now := time.Now()
	photosJSON, _ := json.Marshal(body.Photos)

	// 关联进行中的租约订单（非终态）
	var order *models.Order
	var o models.Order
	if err := db.Where("instrument_id = ? AND status IN ?", inst.ID, lossActiveOrderStatuses).
		Order("created_at DESC").First(&o).Error; err == nil {
		order = &o
	}

	// 用户承担赔偿：员工覆盖值优先，否则 赔偿×比例/100
	burden := body.UserBurdenCents
	if burden <= 0 {
		burden = body.CompensationCents * int64(body.UserRatio) / 100
	}

	rec := models.InstrumentLossRecord{
		ID: uuid.New().String(), TenantID: inst.TenantID, InstrumentID: inst.ID,
		ResponsibleParty: body.ResponsibleParty, UserRatio: body.UserRatio,
		CompensationCents: models.Cents(body.CompensationCents), UserBurdenCents: models.Cents(burden),
		Description: body.Description, Photos: string(photosJSON),
		CreatedBy: operator,
	}
	breakdown := map[string]interface{}{}

	// 退款先行准备：租约结算需退差额时，先执行微信退款（失败 502 不写 DB，参照 #1950 F6）
	var refundRec *models.OrderRefundRecord
	if order != nil {
		rec.OrderID = &order.ID
		var diff models.Cents
		if order.Status == models.OrderStatusInLease {
			// 2026-09-18 口径（LS-03a 场景②）：租期中丢失**不退租金**（已付租金全额保留）
			// → 差额 = 押金 − 用户承担赔偿（押金抵扣赔偿，余额退 / 不足补缴）
			diff = order.Deposit - models.Cents(burden)
			breakdown = map[string]interface{}{
				"scene":               "in_lease_no_rent_refund",
				"rent_refunded_cents": 0,
				"deposit_cents":       order.Deposit,
				"user_burden_cents":   burden,
			}
		} else {
			// ①去程（未签收）租金 0、押金全退；③返程租金至归还寄出日 —— 口径不变（2026-09-18 复核）
			rentCents, scene := lossRentToDateCents(db, order, now)
			owed := models.Cents(rentCents) + models.Cents(burden)
			paidCash := effectiveCashPaid(db, *order)
			diff = paidCash - owed
			breakdown = map[string]interface{}{
				"scene": scene, "rent_cents": rentCents, "user_burden_cents": burden,
				"owed_cents": owed, "paid_cash_cents": paidCash,
			}
		}
		breakdown["diff_cents"] = diff
		rec.SettledAt = &now
		if diff > 0 {
			// 退款：取最早的 paid 租金支付单（押金在其中）原路退
			var payRec models.OrderPaymentRecord
			if err := db.Where("order_id = ? AND order_type = ? AND type = ? AND status = ?",
				order.ID, "rent", "payment", "paid").Order("created_at ASC").First(&payRec).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "paid rent record not found for refund"})
				return
			}
			outRefundNo := fmt.Sprintf("loss_re_%s", order.ID[:8])
			refundRec = &models.OrderRefundRecord{
				ID: uuid.New().String(), TenantID: order.TenantID, PaymentRecordID: &payRec.ID,
				OutRefundNo: &outRefundNo, Amount: models.Cents(diff),
				Reason: strPtr("乐器丢失结算退款"), Status: "refunded", CreatedAt: now, UpdatedAt: now,
			}
			if payRec.OutTradeNo != nil {
				resp, err := wechatpay.GetClient().Refund(ctx, wechatpay.RefundParams{
					OutTradeNo: *payRec.OutTradeNo, OutRefundNo: outRefundNo,
					TotalAmount: int64(payRec.Amount), RefundAmount: int64(diff),
					Reason: "乐器丢失结算退款", NotifyURL: wechatpay.GetConfig().RefundNotifyURL,
				})
				if err != nil {
					log.Printf("[InstrumentLoss] refund failed for order %s: %v", order.ID, err)
					c.JSON(http.StatusBadGateway, gin.H{
						"code":    50200,
						"message": "refund failed, loss registration aborted for retry: " + err.Error(),
						"data":    gin.H{"refund_cents": diff},
					})
					return
				}
				refundRec.RefundID = &resp.RefundID
				refundRec.Status = "refunding"
			}
			breakdown["refund_cents"] = diff
		} else if diff < 0 {
			breakdown["shortfall_cents"] = -diff
		}
	}
	if b, err := json.Marshal(breakdown); err == nil {
		rec.SettleBreakdown = string(b)
	}

	// 单事务落库：丢失记录 + 乐器 lost + 订单 cancelled + 会话关闭 + 状态历史 + 退款/补缴记录
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Instrument{}).Where("id = ?", inst.ID).
			Update("stock_status", models.StockStatusLost).Error; err != nil {
			return err
		}
		if order != nil {
			updates := map[string]interface{}{"status": models.OrderStatusCancelled, "updated_at": now}
			// 押金口径（LS-03b）：仅在结算已结清（无补缴欠款）时视为押金已处置
			if diff, ok := breakdown["diff_cents"].(models.Cents); ok && diff >= 0 {
				updates["deposit_refunded"] = true
			}
			if err := tx.Model(&models.Order{}).Where("id = ?", order.ID).Updates(updates).Error; err != nil {
				return err
			}
			uid := order.UserID
			if err := tx.Create(&models.OrderStatusHistory{
				ID: uuid.New().String(), TenantID: order.TenantID, OrgID: &order.OrgID,
				OrderID: order.ID, StatusFrom: order.Status, StatusTo: models.OrderStatusCancelled,
				Notes: "乐器丢失，租约非正常终止（" + body.ResponsibleParty + "）", ChangedBy: &uid,
				ChangedAt: now,
			}).Error; err != nil {
				return err
			}
			if diff, ok := breakdown["shortfall_cents"].(models.Cents); ok && diff > 0 {
				if err := tx.Create(&models.OrderPaymentRecord{
					ID: uuid.New().String(), TenantID: order.TenantID, UserID: order.UserID,
					OrderID: &order.ID, OrderType: "loss", Amount: models.Cents(diff),
					Type: "payment", Status: "pending", Method: strPtr("loss"),
					CreatedAt: now, UpdatedAt: now,
				}).Error; err != nil {
					return err
				}
			}
			if refundRec != nil {
				if err := tx.Create(refundRec).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		log.Printf("[InstrumentLoss.Register] tx failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to register loss"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id": rec.ID, "settle_breakdown": rec.SettleBreakdown,
	}})
}

// Restore POST /api/instruments/:id/restore（LS-05/LS-05a）
func (h *InstrumentLossHandler) Restore(c *gin.Context) {
	var body struct {
		Damaged     bool     `json:"damaged"`
		Description string   `json:"description"`
		Photos      []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	inst, ok := loadLossInstrument(c, db, c.Param("id"))
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "instrument not found or access denied"})
		return
	}
	var rec models.InstrumentLossRecord
	if err := db.Where("instrument_id = ? AND restored_at IS NULL", inst.ID).
		Order("created_at DESC").First(&rec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "no open loss record"})
		return
	}
	now := time.Now()
	photosJSON, _ := json.Marshal(body.Photos)

	// 2026-09-18（LS-05 简化 / LS-05a 作废）：找回仅「**恢复上架**」——
	// 不做结算重算与冲正（无 loss_rv_* 退款、无追加租金/定损扣除/hold 语义）。
	updates := map[string]interface{}{
		"restored_at": now, "restored_damaged": body.Damaged,
		"restore_description": body.Description,
	}
	if len(body.Photos) > 0 {
		updates["restore_photos"] = string(photosJSON)
	}
	if err := db.Model(&models.InstrumentLossRecord{}).Where("id = ?", rec.ID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record restore"})
		return
	}
	// 债务冲销（幂等，必做）：关闭 pending loss 补缴 → M-08 解除
	if rec.OrderID != nil {
		if err := db.Model(&models.OrderPaymentRecord{}).
			Where("order_id = ? AND order_type = ? AND status = ? AND method = ?",
				*rec.OrderID, "loss", "pending", "loss").
			Update("status", "closed").Error; err != nil {
			log.Printf("[InstrumentLoss.Restore] close shortfall failed: %v", err)
		}
	}
	result := gin.H{"id": rec.ID, "restored_damaged": body.Damaged, "action": "put_back_on_shelf"}
	// 乐器恢复可租；订单不复活（保持 cancelled）
	if err := db.Model(&models.Instrument{}).Where("id = ?", inst.ID).
		Update("stock_status", models.StockStatusAvailable).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to restore instrument"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// List GET /api/instrument-loss?page=&pageSize=&status=open|restored|all&instrument_id=
// （员工/管理员台账，#2615：分页 + 状态过滤 + 乐器摘要 + photos 数组归一化）
func (h *InstrumentLossHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	rawSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	pageSize := clampPageSize(rawSize, 20, maxPageSize)

	// status 默认 all；旧参数 open=true 等价 status=open（兼容既有调用方）。
	status := c.DefaultQuery("status", "all")
	if c.Query("open") == "true" {
		status = "open"
	}

	query := db.Model(&models.InstrumentLossRecord{}).Where("tenant_id = ?", tid)
	if iid := c.Query("instrument_id"); iid != "" {
		query = query.Where("instrument_id = ?", iid)
	}
	switch status {
	case "open":
		query = query.Where("restored_at IS NULL")
	case "restored":
		query = query.Where("restored_at IS NOT NULL")
	case "all":
		// no status filter
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "status must be open/restored/all"})
		return
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Printf("[InstrumentLoss.List] count failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to count loss records"})
		return
	}
	var list []models.InstrumentLossRecord
	if err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		log.Printf("[InstrumentLoss.List] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list loss records"})
		return
	}

	briefs := lossInstrumentBriefs(db, tid, list)
	out := make([]gin.H, 0, len(list))
	for _, r := range list {
		out = append(out, lossRecordJSON(r, briefs[r.InstrumentID]))
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"list": out, "total": total, "page": page, "page_size": pageSize,
	}})
}

// Get GET /api/instrument-loss/:id（#2615：记录详情页的深链入口）
// 不存在 / 他人租户 / 非法 id 一律 404/40400（不区分，避免探测存在性）。
func (h *InstrumentLossHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	var rec models.InstrumentLossRecord
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&rec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "loss record not found"})
		return
	}
	briefs := lossInstrumentBriefs(db, tid, []models.InstrumentLossRecord{rec})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": lossRecordJSON(rec, briefs[rec.InstrumentID])})
}

// lossPhotosArray 把 jsonb 列的原始字符串归一化为字符串数组：
// 空值 / null / 非法 JSON → []（前端无需区分 null 与空数组）。
func lossPhotosArray(raw string) []string {
	out := []string{}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return out
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return out
	}
	return append(out, arr...)
}

// lossInstrumentBriefs 批量取乐器摘要（一次 IN 查询，禁 N+1）。
// 未命中（乐器被删/跨租户）→ 不放入 map，调用方输出 instrument_brief=null。
func lossInstrumentBriefs(db *gorm.DB, tenantID string, recs []models.InstrumentLossRecord) map[string]gin.H {
	briefs := map[string]gin.H{}
	ids := make([]string, 0, len(recs))
	seen := make(map[string]struct{}, len(recs))
	for _, r := range recs {
		if r.InstrumentID == "" {
			continue
		}
		if _, ok := seen[r.InstrumentID]; ok {
			continue
		}
		seen[r.InstrumentID] = struct{}{}
		ids = append(ids, r.InstrumentID)
	}
	if len(ids) == 0 {
		return briefs
	}
	var insts []models.Instrument
	if err := db.Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&insts).Error; err != nil {
		log.Printf("[InstrumentLoss] WARN: load instrument briefs failed: %v", err)
		return briefs
	}
	for _, inst := range insts {
		briefs[inst.ID] = gin.H{
			"id":            inst.ID,
			"sn":            inst.SN,
			"category_name": inst.CategoryName,
			"level_name":    inst.LevelName,
			"cover_image":   inst.CoverImage,
			"stock_status":  inst.StockStatus,
		}
	}
	return briefs
}

// lossRecordJSON 记录行 → API 形态：原始字段全保留，photos/restore_photos 数组化，
// 并挂 instrument_brief（未命中为 null）。
func lossRecordJSON(r models.InstrumentLossRecord, brief gin.H) gin.H {
	return gin.H{
		"id":                  r.ID,
		"tenant_id":           r.TenantID,
		"instrument_id":       r.InstrumentID,
		"order_id":            r.OrderID,
		"responsible_party":   r.ResponsibleParty,
		"user_ratio":          r.UserRatio,
		"compensation_cents":  r.CompensationCents,
		"user_burden_cents":   r.UserBurdenCents,
		"description":         r.Description,
		"photos":              lossPhotosArray(r.Photos),
		"settled_at":          r.SettledAt,
		"settle_breakdown":    r.SettleBreakdown,
		"created_by":          r.CreatedBy,
		"restored_at":         r.RestoredAt,
		"restored_damaged":    r.RestoredDamaged,
		"restore_description": r.RestoreDescription,
		"restore_photos":      lossPhotosArray(r.RestorePhotos),
		// ⛔ 冲正列已作废（LS-05a，2026-09-18）但**列保留以兼容历史行**：响应必须原样输出，
		// 否则旧 `GET /instrument-loss` 的字段会静默消失（#2615 计划「原始字段全保留」）。
		"reversed_at":           r.ReversedAt,
		"reversed_amount_cents": r.ReversedAmountCents,
		"deducted_damage_cents": r.DeductedDamageCents,
		"deducted_idle_cents":   r.DeductedIdleCents,
		"reverse_note":          r.ReverseNote,
		"created_at":            r.CreatedAt,
		"updated_at":            r.UpdatedAt,
		"instrument_brief":      brief,
	}
}
