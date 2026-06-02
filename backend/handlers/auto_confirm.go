package handlers

import (
	"log"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AutoConfirmService struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewAutoConfirmService() *AutoConfirmService {
	return &AutoConfirmService{
		done: make(chan bool),
	}
}

func (s *AutoConfirmService) Start() {
	s.db = database.GetDB()
	s.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		s.process()
		for {
			select {
			case <-s.ticker.C:
				s.process()
			case <-s.done:
				s.ticker.Stop()
				return
			}
		}
	}()
	log.Println("AutoConfirm service started - checks every hour for shipped orders > 48h")
}

func (s *AutoConfirmService) Stop() {
	s.done <- true
}

func (s *AutoConfirmService) process() {
	now := time.Now()
	var orders []models.Order
	if err := s.db.Where("status = ? AND shipped_at < NOW() - INTERVAL '48 hours'", models.OrderStatusShipped).Find(&orders).Error; err != nil {
		log.Printf("[AutoConfirm] Failed to query orders: %v", err)
		return
	}

	for _, order := range orders {
		if err := s.db.Model(&order).Updates(map[string]interface{}{
			"status":       models.OrderStatusInLease,
			"delivered_at": now,
		}).Error; err != nil {
			log.Printf("[AutoConfirm] Failed to update order %s: %v", order.ID, err)
			continue
		}

		// Update instrument stock status
		if order.InstrumentID != "" {
			s.db.Table("instruments").Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusRented)
		}

		// Create OrderStatusHistory
		s.db.Create(&models.OrderStatusHistory{
			ID:         uuid.New().String(),
			TenantID:   order.TenantID,
			OrderID:    order.ID,
			StatusFrom: models.OrderStatusShipped,
			StatusTo:   models.OrderStatusInLease,
			Notes:      "系统自动确认（48h 未手动确认）",
			ChangedAt:  now,
		})

		// Update LeaseSession start time
		s.db.Model(&models.LeaseSession{}).Where("order_id = ?", order.ID).Updates(map[string]interface{}{
			"start_date": now,
			"status":     models.LeaseStatusActive,
		})

		log.Printf("[AutoConfirm] Auto-confirmed order %s (shipped -> in_lease)", order.ID)
	}
}
