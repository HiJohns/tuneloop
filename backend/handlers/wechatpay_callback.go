package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WechatPayCallback handles POST /api/wechatpay/notify
func WechatPayCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("[WechatPayCallback] failed to read body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid body"})
		return
	}

	signature := c.GetHeader("Wechatpay-Signature")
	serial := c.GetHeader("Wechatpay-Serial")
	timestamp := c.GetHeader("Wechatpay-Timestamp")
	nonce := c.GetHeader("Wechatpay-Nonce")

	client := wechatpay.GetClient()
	result, err := client.VerifyPaymentCallback(c.Request.Context(), body, signature, serial, timestamp, nonce)
	if err != nil {
		log.Printf("[WechatPayCallback] verification failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"code": "FAIL", "message": "verification failed"})
		return
	}

	if result.EventType == "REFUND.SUCCESS" {
		if processRefundCallback(c, result) {
			c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "ok"})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": "FAIL", "message": "refund processing failed"})
		}
		return
	}

	if processPaymentCallback(c, result) {
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "ok"})
	} else {
		c.JSON(http.StatusOK, gin.H{"code": "FAIL", "message": "processing failed"})
	}
}

// processRefundCallback handles WeChat Pay refund result notifications (REFUND.SUCCESS).
// It updates the matching OrderRefundRecord and the associated settlement's refund status.
func processRefundCallback(c *gin.Context, result *wechatpay.CallbackResult) bool {
	if result.OutRefundNo == "" {
		log.Printf("[processRefundCallback] missing out_refund_no for refund callback")
		return false
	}

	db := database.GetDB().WithContext(c.Request.Context())
	now := time.Now()

	var refundRecord models.OrderRefundRecord
	if err := db.Where("out_refund_no = ?", result.OutRefundNo).First(&refundRecord).Error; err != nil {
		log.Printf("[processRefundCallback] refund record not found for %s: %v", result.OutRefundNo, err)
		return false
	}

	if err := db.Model(&refundRecord).Updates(map[string]interface{}{
		"status":     "refunded",
		"refund_id":  result.RefundID,
		"updated_at": now,
	}).Error; err != nil {
		log.Printf("[processRefundCallback] failed to update refund record %s: %v", result.OutRefundNo, err)
		return false
	}

	// Update the associated settlement refund status (if any)
	if refundRecord.PaymentRecordID != nil {
		var settlement models.Settlement
		if err := db.Where("order_id IN (SELECT order_id FROM order_payment_records WHERE id = ?)", *refundRecord.PaymentRecordID).
			First(&settlement).Error; err == nil {
			if err := db.Model(&settlement).Updates(map[string]interface{}{
				"refund_status": "completed",
				"updated_at":    now,
			}).Error; err != nil {
				log.Printf("[processRefundCallback] failed to update settlement for %s: %v", result.OutRefundNo, err)
			}
		}

		// Re-evaluate membership level after refund (aggregated spending decreased;
		// level only upgrades, so this is a no-op unless other spending crossed a threshold)
		var payment models.OrderPaymentRecord
		if err := db.Where("id = ?", *refundRecord.PaymentRecordID).First(&payment).Error; err == nil {
			if err := services.CheckAndUpgradeLevel(payment.UserID, nil); err != nil {
				log.Printf("[processRefundCallback] membership level check failed: %v", err)
			}
		}
	}

	log.Printf("[processRefundCallback] refund processed: out_refund_no=%s refund_id=%s", result.OutRefundNo, result.RefundID)
	return true
}

func processPaymentCallback(c *gin.Context, result *wechatpay.CallbackResult) bool {
	if !result.Success {
		log.Printf("[processPaymentCallback] payment not successful for %s", result.OutTradeNo)
		return false
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var record models.OrderPaymentRecord
	if err := db.Where("out_trade_no = ?", result.OutTradeNo).First(&record).Error; err != nil {
		log.Printf("[processPaymentCallback] record not found for %s", result.OutTradeNo)
		return false
	}

	if record.Status == "paid" {
		log.Printf("[processPaymentCallback] already processed: %s", result.OutTradeNo)
		return true
	}

	if int64(record.Amount) != result.Amount {
		log.Printf("[processPaymentCallback] amount mismatch: record=%d callback=%d", int64(record.Amount), result.Amount)
		return false
	}

	now := time.Now()
	record.Status = "paid"
	record.TransactionID = &result.TransactionID
	// #1731: persist the payer.openid from the payment callback — the
	// authoritative source for later upload_shipping_info / notify_confirm_receive.
	record.OpenID = result.OpenID
	record.UpdatedAt = now

	raw, _ := json.Marshal(result)
	rawStr := string(raw)
	record.RawResponse = &rawStr

	tx := db.Begin()

	if err := tx.Save(&record).Error; err != nil {
		tx.Rollback()
		log.Printf("[processPaymentCallback] failed to update record: %v", err)
		return false
	}

	if err := applySideEffects(tx, &record, now); err != nil {
		tx.Rollback()
		return false
	}

	tx.Commit()
	log.Printf("[processPaymentCallback] payment processed: out_trade_no=%s transaction_id=%s amount=%d type=%s", result.OutTradeNo, result.TransactionID, int64(record.Amount), record.OrderType)

	// #1730: virtual/service goods (no physical delivery) must report
	// shipping info (logistics_type=3) so WeChat settles the frozen funds.
	switch record.OrderType {
	case "membership", "renewal", "damage", "repair", "payment_shortfall":
		reportVirtualGoodsShipping(db, &record)
	}

	return true
}

// reportVirtualGoodsShipping reports a logistics_type=3 (virtual goods)
// shipment to WeChat right after payment (#1730): membership/renewal/damage/
// repair fees have no physical delivery, and without upload_shipping_info the
// platform keeps funds frozen forever. Non-fatal — failures are logged.
func reportVirtualGoodsShipping(db *gorm.DB, record *models.OrderPaymentRecord) {
	if record.OutTradeNo == nil || *record.OutTradeNo == "" {
		return
	}
	transactionID := ""
	if record.TransactionID != nil {
		transactionID = *record.TransactionID
	}
	itemDesc := "会员/服务费"
	switch record.OrderType {
	case "membership":
		itemDesc = "会员费"
	case "renewal":
		itemDesc = "租赁续费"
	case "damage":
		itemDesc = "定损赔付"
	case "payment_shortfall":
		itemDesc = "补缴差额"
	case "repair":
		itemDesc = "维修服务费"
	}
	// #1731: prefer the payer.openid persisted on this record (callback) —
	// users.wx_openid cache is only a fallback.
	openid := record.OpenID
	if openid == "" {
		openid = openidOfUser(db, record.UserID)
	}
	if openid == "" {
		log.Printf("[WechatShipping] virtual goods %s: no openid for user %s", record.OrderType, record.UserID)
	}
	go func() {
		services.UploadShippingInfoWithRetry(openid, *record.OutTradeNo, transactionID, "", "", itemDesc, 3, fmt.Sprintf("virtual %s", record.OrderType))
	}()
}

// applySideEffects updates order/repair/points state after payment is confirmed.
// Called from both processPaymentCallback (real callback) and PrepayOrder (mock mode).
func applySideEffects(tx *gorm.DB, record *models.OrderPaymentRecord, now time.Time) error {
	// Deduct points if the payment record carries prepaid/gift usage data
	if err := deductPointsFromRecord(tx, record, now); err != nil {
		return err
	}

	switch record.OrderType {
	case "rent":
		// #1743: record.Amount is the server-recomputed discounted actual
		// payment (coupon codes applied at prepay) — write it back to
		// cash_paid so settlement uses the real paid base, not the pre-order
		// full-price snapshot. gift_points_used stays untouched. Renewal has
		// its own accumulation logic (renewal.go) — never touch it here.
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).
			Updates(map[string]interface{}{"status": models.OrderStatusPaid, "cash_paid": record.Amount}).Error; err != nil {
			return err
		}
		// Order timeline (order_logs) — payment must be visible to customer
		if record.OrderID != nil {
			if err := tx.Create(&models.OrderLog{
				OrderID:   *record.OrderID,
				Event:     "已支付",
				CreatedAt: now,
			}).Error; err != nil {
				log.Printf("[applySideEffects] failed to write payment order log: %v", err)
			}
		}
		return tx.Model(&models.Instrument{}).
			Where("id = (SELECT instrument_id FROM orders WHERE id = ? LIMIT 1)", record.OrderID).
			Update("stock_status", "rented").Error
	case "repair":
		return tx.Model(&models.RepairRequest{}).Where("id = ?", record.OrderID).Update("status", models.RepairReqStatusPendingShip).Error
	case "membership":
		// Two-phase registration (session flow, #1663): payment completes
		// and the account is created server-side from form_data. Guarded by
		// the dedicated SessionID column — RawResponse is overwritten by the
		// callback result at processPaymentCallback, so the session link can
		// never live there (#1664 audit).
		if record.SessionID != nil && *record.SessionID != "" {
			return completeRegistrationFromSession(tx, record, now)
		}
		// Legacy membership fee (#1532): activate the highest level whose
		// MinAmount <= paid amount. OrderID stores the local user id.
		// Gift points and referral bonuses are credited during
		// registration (PostRegister) — not repeated here.
		if record.OrderID != nil {
			var levels []models.MembershipLevel
			tx.Order("min_amount ASC").Find(&levels)
			newLevelID := 0
			for _, l := range levels {
				if record.Amount >= l.MinAmount {
					newLevelID = l.ID
				}
			}
			if newLevelID > 0 {
				if err := tx.Model(&models.User{}).Where("id = ?", *record.OrderID).
					Update("membership_level_id", newLevelID).Error; err != nil {
					log.Printf("[applySideEffects] membership level activate failed: %v", err)
				}
			}
		}
		return nil
	case "damage":
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).Update("status", models.OrderStatusCompleted).Error; err != nil {
			return err
		}
		if record.OrderID != nil {
			var report models.DamageReport
			if err := tx.Where("lease_id = ?", *record.OrderID).First(&report).Error; err == nil {
				if report.DepositDeducted <= 0 {
					tx.Model(&report).Update("deposit_deducted", gorm.Expr("deposit"))
				}
			}
		}
		// Execute final settlement + refund (#1530): damage payment completes
		// the order; compute actual rent and refund the difference
		// (rent overpayment + remaining deposit after damage deduction).
		if record.OrderID != nil {
			var completedOrder models.Order
			if err := tx.Where("id = ?", *record.OrderID).First(&completedOrder).Error; err != nil {
				log.Printf("[applySideEffects] Failed to reload order for settlement: %v", err)
			} else {
				if _, err := executeRefund(tx, completedOrder); err != nil {
					log.Printf("[applySideEffects] Settlement failed for order %s: %v", *record.OrderID, err)
				}
			}
		}
		return nil
	case "payment_shortfall":
		// #1746 L-04C 流程 4/5：补缴支付成功 → 订单 completed → 结算闭环。
		// settlement 已存在（shortfall 时创建，pending）→ 标记完成
		// （此时已付 ≥ 应付，无退款/走幂等 executeRefund）。
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).Update("status", models.OrderStatusCompleted).Error; err != nil {
			return err
		}
		// Restore instrument stock_status to available (#1767)
		if record.OrderID != nil {
			if err := tx.Model(&models.Instrument{}).Where("id = (SELECT instrument_id FROM orders WHERE id = ? LIMIT 1)", *record.OrderID).
				Update("stock_status", models.StockStatusAvailable).Error; err != nil {
				log.Printf("[applySideEffects] Failed to restore instrument status for order %s: %v", *record.OrderID, err)
			}
		}
		if record.OrderID != nil {
			if err := tx.Model(&models.Settlement{}).Where("order_id = ?", *record.OrderID).
				Update("refund_status", "completed").Error; err != nil {
				log.Printf("[applySideEffects] Failed to complete settlement for order %s: %v", *record.OrderID, err)
			}
			var completedOrder models.Order
			if err := tx.Where("id = ?", *record.OrderID).First(&completedOrder).Error; err == nil {
				if _, err := executeRefund(tx, completedOrder); err != nil {
					log.Printf("[applySideEffects] Settlement failed for order %s: %v", *record.OrderID, err)
				}
			}
		}
		return nil
	case "renewal":
		return applyRenewalSideEffects(tx, record, now)
	default:
		return nil
	}
}

func deductPointsFromRecord(tx *gorm.DB, record *models.OrderPaymentRecord, now time.Time) error {
	if record.RawResponse == nil || *record.RawResponse == "" {
		return nil
	}
	var points struct {
		GiftUsed float64 `json:"gift_used"`
	}
	// #1728 P3：raw_response gift_used 为分；#1757：promo_points 分运算。
	if err := json.Unmarshal([]byte(*record.RawResponse), &points); err != nil {
		return nil // silently ignore malformed raw_response
	}
	giftUsedCents := points.GiftUsed
	if giftUsedCents <= 0 {
		return nil
	}
	if err := tx.Model(&models.User{}).Where("iam_sub = ?", record.UserID).
		Updates(map[string]interface{}{
			"promo_points": gorm.Expr("GREATEST(promo_points - ?, 0)", giftUsedCents),
			"updated_at":   now,
		}).Error; err != nil {
		return fmt.Errorf("deduct user points: %w", err)
	}
	if record.OrderID != nil {
		updates := map[string]interface{}{
			"gift_points_used": giftUsedCents,
		}
		// Deduct gift points from cash_paid so settlement does not double
		// count them (L-06). cash_paid was set to the full total at order
		// creation; the gift-covered portion is not cash.
		updates["cash_paid"] = gorm.Expr("GREATEST(cash_paid - ?, 0)", giftUsedCents)
		if err := tx.Model(&models.Order{}).Where("id = ?", *record.OrderID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update order points: %w", err)
		}
	}
	localUserID := resolveLocalUserID(tx, record)
	if localUserID == "" {
		return fmt.Errorf("local user not found for iam_sub %s", record.UserID)
	}
	pt := models.PointsTransaction{
		ID:          uuid.New().String(),
		UserID:      localUserID,
		TenantID:    record.TenantID,
		Type:        "gift_used",
		Amount:      models.Cents(giftUsedCents),
		OrderID:     record.OrderID,
		Description: fmt.Sprintf("订单支付使用赠送点数: gift=%.2f", giftUsedCents/100),
		CreatedAt:   now,
	}
	if err := tx.Create(&pt).Error; err != nil {
		return fmt.Errorf("create points transaction: %w", err)
	}
	return nil
}

// resolveLocalUserID maps the JWT subject (IAM user id) stored in
// record.UserID to the local users.id primary key. PointsTransaction
// and CheckAndUpgradeLevel reference the local users table, while
// order_payment_records.user_id stores the IAM subject — mixing them
// violates points_transactions_user_id_fkey (SQLSTATE 23503).
func resolveLocalUserID(tx *gorm.DB, record *models.OrderPaymentRecord) string {
	var u models.User
	if err := tx.Where("iam_sub = ?", record.UserID).First(&u).Error; err == nil {
		return u.ID
	}
	return ""
}

func StartPaymentScheduler(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			scanPendingPayments(db)
		}
	}()
	log.Println("[PaymentScheduler] started (1m interval)")
}

func scanPendingPayments(db *gorm.DB) {
	cutoff := time.Now().Add(-30 * time.Minute)

	var records []models.OrderPaymentRecord
	if err := db.Where("status = ? AND created_at < ?", "pending", cutoff).Find(&records).Error; err != nil {
		log.Printf("[PaymentScheduler] query failed: %v", err)
		return
	}

	for _, rec := range records {
		processPendingRecord(db, &rec)
	}
}

func processPendingRecord(db *gorm.DB, rec *models.OrderPaymentRecord) {
	// #1787-deploy: guard against records without out_trade_no (e.g. corrupt
	// orphan pending rows) — dereferencing nil panics the payment scheduler
	// and crashes the whole service (pre-prod incident 2026-08-27).
	if rec.OutTradeNo == nil || *rec.OutTradeNo == "" {
		log.Printf("[PaymentScheduler] skip record %s: missing out_trade_no", rec.ID)
		return
	}

	client := wechatpay.GetClient()
	if client == nil {
		log.Printf("[PaymentScheduler] wechatpay client not initialized, skip %s", *rec.OutTradeNo)
		return
	}

	result, err := client.QueryOrder(context.Background(), *rec.OutTradeNo)
	if err != nil {
		log.Printf("[PaymentScheduler] query failed for %s: %v", *rec.OutTradeNo, err)
		return
	}

	if result.TradeState == "SUCCESS" {
		tx := db.Begin()
		rec.Status = "paid"
		rec.TransactionID = &result.TransactionID
		rec.UpdatedAt = time.Now()
		if err := tx.Save(&rec).Error; err != nil {
			tx.Rollback()
			log.Printf("[PaymentScheduler] save failed for %s: %v", *rec.OutTradeNo, err)
			return
		}
		if err := applySideEffects(tx, rec, time.Now()); err != nil {
			tx.Rollback()
			log.Printf("[PaymentScheduler] side effects failed for %s: %v", *rec.OutTradeNo, err)
			return
		}
		tx.Commit()
		log.Printf("[PaymentScheduler] recovered + applied: out_trade_no=%s tx_id=%s", *rec.OutTradeNo, result.TransactionID)
		return
	}

	_ = client.CloseOrder(context.Background(), *rec.OutTradeNo)

	if err := db.Model(rec).Updates(map[string]interface{}{
		"status":     "closed",
		"updated_at": time.Now(),
	}).Error; err != nil {
		log.Printf("[PaymentScheduler] failed to close timed-out payment %s: %v", *rec.OutTradeNo, err)
	}

	switch rec.OrderType {
	case "rent":
		if rec.OrderID != nil {
			db.Model(&models.Order{}).Where("id = ?", *rec.OrderID).
				Where("status = ?", models.OrderStatusReserved).
				Update("status", models.OrderStatusCancelled)
		}
	case "repair":
		if rec.OrderID != nil {
			db.Model(&models.RepairRequest{}).Where("id = ?", *rec.OrderID).
				Where("status = ?", models.RepairReqStatusPendingPay).
				Update("status", models.RepairReqStatusClosed)
		}
	case "damage":
		if rec.OrderID != nil {
			db.Model(&models.Order{}).Where("id = ?", *rec.OrderID).
				Where("status != ?", models.OrderStatusCompleted).
				Update("status", models.OrderStatusDamageAppealing)
		}
	case "renewal":
		// renewal payment timeout — close the record, order state unchanged
	case "payment_shortfall":
		// #1746: 补缴超时 → 记录关闭，订单保持 returning（等待重新补缴，
		// 不关单不催缴——催缴属 #1749 Ticker 职责）
	}

	log.Printf("[PaymentScheduler] closed timed-out payment: %s", *rec.OutTradeNo)
}
