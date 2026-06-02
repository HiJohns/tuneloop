package handlers

import (
	"log"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

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
	result := s.db.Model(&models.Order{}).
		Where("status = ? AND shipped_at < NOW() - INTERVAL '48 hours'", models.OrderStatusShipped).
		Update("status", models.OrderStatusInLease)
	if result.Error != nil {
		log.Printf("[AutoConfirm] Failed to auto-confirm orders: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[AutoConfirm] Auto-confirmed %d orders (shipped -> in_lease)", result.RowsAffected)
	}
}
