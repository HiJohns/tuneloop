package handlers

import (
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
// 有 oid（网点级）时要求乐器当前网点一致。
func loadLossInstrument(c *gin.Context, db *gorm.DB, id string) (*models.Instrument, bool) {
	ctx := c.Request.Context()
	var inst models.Instrument
	if err := db.Where("id = ?", id).First(&inst).Error; err != nil {
		return nil, false
	}
	tid := middleware.GetTenantID(ctx)
	if tid == "" || inst.TenantID != tid {
		return nil, false
	}
	if oid := middleware.GetOrgID(ctx); oid != "" && inst.CurrentSiteID != nil && inst.CurrentSiteID.String() != oid {
		return nil, false
	}
	return &inst, true
}

// lossDailyRentCents 取日租金（分）：PricingBreakdown.final_daily_rent（折扣后，元）
// 优先，缺失回退 MonthlyRent/30。
func lossDailyRentCents(order *models.Order) models.Cents {
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
	daily := lossDailyRentCents(order)
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
		rentCents, scene := lossRentToDateCents(db, order, now)
		owed := models.Cents(rentCents) + models.Cents(burden)
		paidCash := order.CashPaid
		breakdown = map[string]interface{}{
			"scene": scene, "rent_cents": rentCents, "user_burden_cents": burden,
			"owed_cents": owed, "paid_cash_cents": paidCash,
		}
		diff := paidCash - owed
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
			if err := tx.Model(&models.LeaseSession{}).Where("order_id = ?", order.ID).
				Update("status", models.LeaseStatusCancelled).Error; err != nil {
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

	result := gin.H{"id": rec.ID, "restored_damaged": body.Damaged}

	// 找回冲正（LS-05a）：仅当丢失时已结算；damaged=true 挂起（方案 B）
	if rec.SettledAt != nil && rec.ReversedAt == nil && rec.UserBurdenCents > 0 {
		// 追加租金 = daily × CalculateLeaseDays(丢失日, min(找回日, 原租期结束日)) × ratio%
		extra := models.Cents(0)
		if rec.OrderID != nil {
			var order models.Order
			if err := db.Where("id = ?", *rec.OrderID).First(&order).Error; err == nil {
				daily := lossDailyRentCents(&order)
				lossDate := rec.CreatedAt
				end := now
				if order.EndDate != nil {
					if te, ok := lossDateParse(*order.EndDate); ok && te.Before(now) {
						end = te
					}
				}
				if daily > 0 && end.After(lossDate) {
					days := services.CalculateLeaseDays(lossDate, end)
					extra = models.Cents(daily) * models.Cents(days) * models.Cents(rec.UserRatio) / 100
				}
			}
		}
		net := rec.UserBurdenCents - extra
		if body.Damaged {
			// 挂起：损坏待定损，暂扣全额（差额处理另立 Issue）
			result["reversal"] = "held_pending_assessment"
			updates := map[string]interface{}{
				"reversed_amount_cents": models.Cents(0),
				"deducted_damage_cents": rec.UserBurdenCents,
				"deducted_idle_cents":   models.Cents(0),
				"reverse_note":          "损坏待定损，暂扣赔偿额，定损后多退少补",
				"restored_at":           now, "restored_damaged": true,
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
		} else if net > 0 {
			// 退款先行（loss_rv_*）
			var payRec models.OrderPaymentRecord
			var outRefundNo string
			if rec.OrderID != nil {
				if err := db.Where("order_id = ? AND order_type = ? AND type = ? AND status = ?",
					*rec.OrderID, "rent", "payment", "paid").Order("created_at ASC").
					First(&payRec).Error; err == nil && payRec.OutTradeNo != nil {
					outRefundNo = fmt.Sprintf("loss_rv_%s", inst.ID[:8])
					if _, err := wechatpay.GetClient().Refund(ctx, wechatpay.RefundParams{
						OutTradeNo: *payRec.OutTradeNo, OutRefundNo: outRefundNo,
						TotalAmount: int64(payRec.Amount), RefundAmount: int64(net),
						Reason: "乐器找回冲正退款", NotifyURL: wechatpay.GetConfig().RefundNotifyURL,
					}); err != nil {
						log.Printf("[InstrumentLoss.Restore] reversal refund failed: %v", err)
						c.JSON(http.StatusBadGateway, gin.H{
							"code":    50200,
							"message": "reversal refund failed, restore aborted for retry: " + err.Error(),
							"data":    gin.H{"refund_cents": net},
						})
						return
					}
				}
			}
			updates := map[string]interface{}{
				"reversed_at": now, "reversed_amount_cents": net,
				"deducted_damage_cents": models.Cents(0), "deducted_idle_cents": models.Cents(0),
				"reverse_note": "找回无损坏，全额冲正（含追加租金扣除 " + fmt.Sprint(extra) + " 分）",
				"restored_at":  now, "restored_damaged": false,
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
			result["reversal"] = "refunded"
			result["refund_cents"] = net
		} else {
			// net <= 0：无需退款，仅终结冲正
			if err := db.Model(&models.InstrumentLossRecord{}).Where("id = ?", rec.ID).
				Updates(map[string]interface{}{
					"reversed_at": now, "reversed_amount_cents": models.Cents(0),
					"deducted_damage_cents": models.Cents(0),
					"reverse_note":          "找回无损坏，追加租金已抵扣全部赔偿额",
					"restored_at":           now, "restored_damaged": false,
					"restore_description": body.Description,
				}).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record restore"})
				return
			}
			result["reversal"] = "no_refund"
		}
		// 债务冲销（必做，幂等）：关闭 pending loss 补缴
		if err := db.Model(&models.OrderPaymentRecord{}).
			Where("order_id = ? AND order_type = ? AND status = ? AND method = ?",
				rec.OrderID, "loss", "pending", "loss").
			Update("status", "closed").Error; err != nil {
			log.Printf("[InstrumentLoss.Restore] close shortfall failed: %v", err)
		}
	} else {
		// 分支 A（未结算/纯库存）：仅恢复 + 留痕
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
	}
	// 乐器恢复可租；订单不复活（保持 cancelled）
	if err := db.Model(&models.Instrument{}).Where("id = ?", inst.ID).
		Update("stock_status", models.StockStatusAvailable).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to restore instrument"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// List GET /api/instrument-loss?instrument_id=&status=（员工/管理员台账）
func (h *InstrumentLossHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	query := db.Where("tenant_id = ?", tid)
	if iid := c.Query("instrument_id"); iid != "" {
		query = query.Where("instrument_id = ?", iid)
	}
	if open := c.Query("open"); open == "true" {
		query = query.Where("restored_at IS NULL")
	}
	var list []models.InstrumentLossRecord
	if err := query.Order("created_at DESC").Limit(200).Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list loss records"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}
