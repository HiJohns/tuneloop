package handlers

import (
	"encoding/json"
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

	if processPaymentCallback(c, result) {
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "ok"})
	} else {
		c.JSON(http.StatusOK, gin.H{"code": "FAIL", "message": "processing failed"})
	}
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

	switch record.OrderType {
	case "rent":
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).Update("status", models.OrderStatusPaid).Error; err != nil {
			tx.Rollback()
			log.Printf("[processPaymentCallback] failed to update order: %v", err)
			return false
		}

	case "repair":
		if err := tx.Model(&models.RepairRequest{}).Where("id = ?", record.OrderID).Update("status", models.RepairReqStatusPendingShip).Error; err != nil {
			tx.Rollback()
			log.Printf("[processPaymentCallback] failed to update repair: %v", err)
			return false
		}

	case "points":
		if record.OrderID != nil {
			if err := tx.Model(&models.User{}).Where("id = ?", *record.OrderID).
				Updates(map[string]interface{}{
					"prepaid_points": gorm.Expr("prepaid_points + ?", record.Amount),
					"updated_at":     now,
				}).Error; err != nil {
				tx.Rollback()
				log.Printf("[processPaymentCallback] failed to add points: %v", err)
				return false
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
				tx.Rollback()
				log.Printf("[processPaymentCallback] failed to record points transaction: %v", err)
				return false
			}
		}

	case "damage":
		if err := tx.Model(&models.Order{}).Where("id = ?", record.OrderID).Update("status", models.OrderStatusCompleted).Error; err != nil {
			tx.Rollback()
			log.Printf("[processPaymentCallback] failed to complete order: %v", err)
			return false
		}
		if record.OrderID != nil {
			var report models.DamageReport
			if err := tx.Where("lease_id = ?", *record.OrderID).First(&report).Error; err == nil {
				if report.DepositDeducted <= 0 {
					tx.Model(&report).Update("deposit_deducted", gorm.Expr("deposit"))
				}
			}
		}
	}

	tx.Commit()
	log.Printf("[processPaymentCallback] payment processed: out_trade_no=%s transaction_id=%s amount=%.2f type=%s", result.OutTradeNo, result.TransactionID, record.Amount, record.OrderType)
	return true
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

	result, err := client.QueryOrder(nil, *rec.OutTradeNo)
	if err != nil {
		log.Printf("[PaymentScheduler] query failed for %s: %v", *rec.OutTradeNo, err)
		return
	}

	if result.TradeState == "SUCCESS" {
		db.Model(rec).Updates(map[string]interface{}{
			"status":         "paid",
			"transaction_id": result.TransactionID,
			"updated_at":     time.Now(),
		})
		log.Printf("[PaymentScheduler] recovered payment for %s, tx_id=%s", *rec.OutTradeNo, result.TransactionID)
		return
	}

	_ = client.CloseOrder(nil, *rec.OutTradeNo)

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
	}

	log.Printf("[PaymentScheduler] closed timed-out payment: %s", *rec.OutTradeNo)
}
