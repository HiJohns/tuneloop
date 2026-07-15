package handlers

import (
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StartReservedOrderScheduler starts a goroutine that cancels reserved orders past their payment deadline.
func StartReservedOrderScheduler() {
	go func() {
		db := database.GetDB()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		cancelOverdueReservedOrders(db)
		for range ticker.C {
			cancelOverdueReservedOrders(db)
		}
	}()
	log.Println("[ReservedOrderScheduler] started (30s interval)")
}

func cancelOverdueReservedOrders(db *gorm.DB) {
	var orders []models.Order
	if err := db.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ? AND COALESCE(payment_deadline, created_at + INTERVAL '10 minutes') < ?",
			models.OrderStatusReserved, time.Now()).
		Find(&orders).Error; err != nil {
		log.Printf("[ReservedOrderScheduler] query failed: %v", err)
		return
	}

	for _, order := range orders {
		orderID := order.ID
		if err := db.Transaction(func(tx *gorm.DB) error {
			// Lock the instrument row
			var inst models.Instrument
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", order.InstrumentID).First(&inst).Error; err != nil {
				return err
			}

			// Cancel the order
			if err := tx.Model(&order).Update("status", models.OrderStatusCancelled).Error; err != nil {
				return err
			}

			// Release instrument
			if err := tx.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).
				Update("stock_status", models.StockStatusAvailable).Error; err != nil {
				return err
			}

			// Record status history
			history := models.OrderStatusHistory{
				ID:         uuid.New().String(),
				TenantID:   order.TenantID,
				OrderID:    orderID,
				StatusFrom: models.OrderStatusReserved,
				StatusTo:   models.OrderStatusCancelled,
				Notes:      "支付超时自动取消",
				ChangedAt:  time.Now(),
			}
			if err := tx.Create(&history).Error; err != nil {
				return err
			}

			// Refund points
			refundOrderPoints(tx, &order)

			return nil
		}); err != nil {
			log.Printf("[ReservedOrderScheduler] failed to cancel order %s: %v", orderID, err)
		} else {
			log.Printf("[ReservedOrderScheduler] cancelled overdue order %s (deadline: %v)", orderID, order.PaymentDeadline)
		}
	}
}
