package services

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"

	"gorm.io/gorm"
)

type DepositRefundScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewDepositRefundScheduler() *DepositRefundScheduler {
	return &DepositRefundScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *DepositRefundScheduler) Start() {
	s.ticker = time.NewTicker(5 * time.Minute)
	go func() {
		s.process()
		for {
			select {
			case <-s.ticker.C:
				if err := s.process(); err != nil {
					log.Printf("[DepositRefundScheduler] error: %v", err)
				}
			case <-s.done:
				s.ticker.Stop()
				return
			}
		}
	}()
	log.Println("[DepositRefundScheduler] started - checks every 5 minutes for expired deposit_refunding orders")
}

func (s *DepositRefundScheduler) Stop() {
	s.done <- true
}

func (s *DepositRefundScheduler) process() error {
	cutoff := time.Now().Add(-24 * time.Hour)

	var expiredOrders []models.Order
	if err := s.db.Where("status = ? AND updated_at < ?", models.OrderStatusDepositRefunding, cutoff).Find(&expiredOrders).Error; err != nil {
		return err
	}

	for _, order := range expiredOrders {
		if err := s.closeOrder(order); err != nil {
			log.Printf("[DepositRefundScheduler] failed to close order %s: %v", order.ID, err)
		}
	}

	if len(expiredOrders) > 0 {
		log.Printf("[DepositRefundScheduler] closed %d expired deposit_refunding orders", len(expiredOrders))
	}
	return nil
}

func (s *DepositRefundScheduler) closeOrder(order models.Order) error {
	cfg := wechatpay.GetConfig()
	depositToRefund := order.Deposit
	if depositToRefund <= 0 {
		depositToRefund = 0
	}

	tx := s.db.Begin()

	// Find the original payment record for this order
	var paymentRecord models.OrderPaymentRecord
	var paymentRecordID string
	if err := tx.Where("order_id = ? AND order_type = ? AND status = ?", order.ID, "rent", "paid").First(&paymentRecord).Error; err == nil {
		paymentRecordID = paymentRecord.ID
	}

	outRefundNo := fmt.Sprintf("refund_%s_%d", order.ID[:8], time.Now().Unix())

	refundRecord := models.OrderRefundRecord{
		ID:              uuid.New().String(),
		TenantID:        order.TenantID,
		PaymentRecordID: paymentRecordID,
		OutRefundNo:     &outRefundNo,
		Amount:          depositToRefund,
		Reason:          strPtr("押金原路退还"),
		Status:          "pending",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if cfg.MockMode {
		refundRecord.Status = "refunded"
		if err := tx.Create(&refundRecord).Error; err != nil {
			tx.Rollback()
			return err
		}
	} else {
		client := wechatpay.GetClient()
		if paymentRecord.OutTradeNo != nil {
			result, err := client.Refund(nil, wechatpay.RefundParams{
				OutTradeNo:   *paymentRecord.OutTradeNo,
				OutRefundNo:  outRefundNo,
				TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
				RefundAmount: cfg.AmountToCents(depositToRefund),
				Reason:       "押金原路退还",
				NotifyURL:    cfg.RefundNotifyURL,
			})
			if err != nil {
				refundRecord.Status = "failed"
				fr := err.Error()
				refundRecord.FailReason = &fr
				if err := tx.Create(&refundRecord).Error; err != nil {
					tx.Rollback()
					return err
				}
				log.Printf("[DepositRefundScheduler] refund API failed for order %s: %v", order.ID, err)
				tx.Commit()
				return nil
			}
			refundRecord.RefundID = &result.RefundID
		}
		if err := tx.Create(&refundRecord).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"status":           models.OrderStatusCompleted,
		"deposit_refunded": true,
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable).Error; err != nil {
		tx.Rollback()
		return err
	}

	notification := models.Notification{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		UserID:     order.UserID,
		Type:       "refund",
		Title:      "押金已退还",
		Content:    "押金退还已完成，订单已关闭",
		RefID:      order.ID,
		RefType:    "order",
		ActionType: "info",
		Status:     "unread",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := tx.Create(&notification).Error; err != nil {
		tx.Rollback()
		return err
	}

	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		OrderID:    order.ID,
		StatusFrom: models.OrderStatusDepositRefunding,
		StatusTo:   models.OrderStatusCompleted,
		Notes:      "押金退还超24小时，自动关闭",
		ChangedAt:  time.Now(),
	}
	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func strPtr(s string) *string { return &s }
