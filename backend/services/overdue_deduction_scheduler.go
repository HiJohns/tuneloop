package services

import (
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

const (
	DefaultOverdueDiscountRate = 1.5
)

type OverdueDeductionScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewOverdueDeductionScheduler() *OverdueDeductionScheduler {
	return &OverdueDeductionScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *OverdueDeductionScheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		// Run immediately on start, then hourly
		s.processOverdue()

		for range s.ticker.C {
			s.processOverdue()
		}
	}()
	log.Println("[OverdueDeductionScheduler] started")
}

func (s *OverdueDeductionScheduler) Stop() {
	s.ticker.Stop()
	s.done <- true
	log.Println("[OverdueDeductionScheduler] stopped")
}

// ProcessOverdueForTest runs one overdue-processing pass. Exported for tests.
func (s *OverdueDeductionScheduler) ProcessOverdueForTest() {
	s.processOverdue()
}

// SetDBForTest replaces the scheduler DB handle. Exported for tests.
func (s *OverdueDeductionScheduler) SetDBForTest(db *gorm.DB) {
	s.db = db
}

func (s *OverdueDeductionScheduler) processOverdue() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OverdueDeductionScheduler] panic: %v", r)
		}
	}()

	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")

	// Status transition — in_lease → expired (always run).
	// Daily overdue deduction was removed by design (#1490): overdue fees are
	// collected once at return inspection, no failed/partial arrears accrue.
	s.transitionToExpired(todayStr)
}

// transitionToExpired moves in_lease orders past their end_date to expired status.
func (s *OverdueDeductionScheduler) transitionToExpired(todayStr string) {
	now := time.Now()

	var orders []models.Order
	s.db.Where("status = ? AND end_date <= ?", models.OrderStatusInLease, todayStr).Find(&orders)
	if len(orders) == 0 {
		return
	}

	for _, order := range orders {
		tx := s.db.Begin()
		if err := tx.Model(&order).Update("status", models.OrderStatusExpired).Error; err != nil {
			tx.Rollback()
			log.Printf("[OverdueDeductionScheduler] failed to expire order %s: %v", order.ID, err)
			continue
		}
		tx.Create(&models.OrderStatusHistory{
			OrderID:    order.ID,
			TenantID:   order.TenantID,
			StatusFrom: order.Status,
			StatusTo:   models.OrderStatusExpired,
			ChangedAt:  now,
			CreatedAt:  now,
		})
		tx.Create(&models.OrderLog{
			OrderID:   order.ID,
			Event:     "expired",
			CreatedAt: now,
		})
		tx.Commit()
	}
	log.Printf("[OverdueDeductionScheduler] transitioned %d orders in_lease → expired", len(orders))
}
