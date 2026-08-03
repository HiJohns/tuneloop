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
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WechatPayCallback handles POST /api/wechatpay/notify
func WechatPayCallback(c *gin.Context) {
	cfg := wechatpay.GetConfig()
	if cfg.MockMode {
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "mock mode, no callbacks expected"})
		return
	}

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

	if record.Amount != wechatpay.GetConfig().CentsToYuan(result.Amount) {
		log.Printf("[processPaymentCallback] amount mismatch: record=%.2f callback=%d", record.Amount, result.Amount)
		return false
	}

	now := time.Now()
	record.Status = "paid"
	record.TransactionID = &result.TransactionID
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
	log.Printf("[processPaymentCallback] payment processed: out_trade_no=%s transaction_id=%s amount=%.2f type=%s", result.OutTradeNo, result.TransactionID, record.Amount, record.OrderType)
	return true
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
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).Update("status", models.OrderStatusPaid).Error; err != nil {
			return err
		}
		return tx.Model(&models.Instrument{}).
			Where("id = (SELECT instrument_id FROM orders WHERE id = ? LIMIT 1)", record.OrderID).
			Update("stock_status", "rented").Error
	case "repair":
		return tx.Model(&models.RepairRequest{}).Where("id = ?", record.OrderID).Update("status", models.RepairReqStatusPendingShip).Error
	case "points":
		return applyPointsPurchase(tx, record, now)
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
		PrepaidUsed float64 `json:"prepaid_used"`
		GiftUsed    float64 `json:"gift_used"`
	}
	if err := json.Unmarshal([]byte(*record.RawResponse), &points); err != nil {
		return nil // silently ignore malformed raw_response
	}
	if points.PrepaidUsed <= 0 && points.GiftUsed <= 0 {
		return nil
	}
	if err := tx.Model(&models.User{}).Where("iam_sub = ?", record.UserID).
		Updates(map[string]interface{}{
			"prepaid_points": gorm.Expr("GREATEST(prepaid_points - ?, 0)", points.PrepaidUsed),
			"promo_points":   gorm.Expr("GREATEST(promo_points - ?, 0)", points.GiftUsed),
			"updated_at":     now,
		}).Error; err != nil {
		return fmt.Errorf("deduct user points: %w", err)
	}
	if record.OrderID != nil {
		if err := tx.Model(&models.Order{}).Where("id = ?", *record.OrderID).
			Updates(map[string]interface{}{
				"prepaid_points_used": points.PrepaidUsed,
				"gift_points_used":    points.GiftUsed,
			}).Error; err != nil {
			return fmt.Errorf("update order points: %w", err)
		}
	}
	pt := models.PointsTransaction{
		ID:          uuid.New().String(),
		UserID:      record.UserID,
		TenantID:    record.TenantID,
		Type:        "prepaid_used",
		Amount:      points.PrepaidUsed + points.GiftUsed,
		OrderID:     record.OrderID,
		Description: fmt.Sprintf("订单支付使用预付点: prepaid=%.2f, gift=%.2f", points.PrepaidUsed, points.GiftUsed),
		CreatedAt:   now,
	}
	if err := tx.Create(&pt).Error; err != nil {
		return fmt.Errorf("create points transaction: %w", err)
	}
	return nil
}

func applyPointsPurchase(tx *gorm.DB, record *models.OrderPaymentRecord, now time.Time) error {
	if record.OrderID == nil {
		return nil
	}
	if err := tx.Model(&models.User{}).Where("id = ?", *record.OrderID).
		Updates(map[string]interface{}{
			"prepaid_points": gorm.Expr("prepaid_points + ?", record.Amount),
			"updated_at":     now,
		}).Error; err != nil {
		log.Printf("[applySideEffects] failed to add points: %v", err)
		return err
	}
	pt := models.PointsTransaction{
		ID:          uuid.New().String(),
		UserID:      record.UserID,
		TenantID:    record.TenantID,
		Type:        "prepaid_purchase",
		Amount:      record.Amount,
		Description: "微信支付充值预付点",
		CreatedAt:   now,
	}
	if err := tx.Create(&pt).Error; err != nil {
		log.Printf("[applySideEffects] failed to record points transaction: %v", err)
		return err
	}
	return nil
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
	cfg := wechatpay.GetConfig()
	client := wechatpay.GetClient()

	if cfg.MockMode {
		db.Model(rec).Update("status", "closed")
		log.Printf("[PaymentScheduler] closed pending payment (mock): %s", *rec.OutTradeNo)
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

	db.Model(rec).Updates(map[string]interface{}{
		"status":     "closed",
		"updated_at": time.Now(),
	})

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
	}

	log.Printf("[PaymentScheduler] closed timed-out payment: %s", *rec.OutTradeNo)
}

func TestSimulatePaymentCallback(c *gin.Context) {
	if !wechatpay.GetConfig().MockMode {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "mock payment disabled"})
		return
	}
	var req struct {
		OutTradeNo string `json:"out_trade_no" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters"})
		return
	}
	db := database.GetDB().WithContext(c.Request.Context())
	var record models.OrderPaymentRecord
	if err := db.Where("out_trade_no = ?", req.OutTradeNo).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "payment record not found"})
		return
	}
	log.Printf("[TestCallback] simulating payment callback: out_trade_no=%s type=%s amount=%.2f",
		req.OutTradeNo, record.OrderType, record.Amount)
	go func() {
		time.Sleep(1 * time.Second)
		db2 := database.GetDB()
		tx := db2.Begin()
		now := time.Now()
		tid := "test_" + uuid.New().String()[:12]
		if err := tx.Model(&record).Updates(map[string]interface{}{
			"status": "paid", "transaction_id": tid, "updated_at": now,
		}).Error; err != nil {
			tx.Rollback()
			return
		}
		record.Status = "paid"
		record.TransactionID = &tid
		if err := applySideEffects(tx, &record, now); err != nil {
			tx.Rollback()
			log.Printf("[TestCallback] side effects failed: %v", err)
			return
		}
		tx.Commit()
		log.Printf("[TestCallback] completed: out_trade_no=%s", req.OutTradeNo)
	}()
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "test callback queued"})
}
