package services

import (
	"log"
	"time"

	"tuneloop-backend/database"

	"gorm.io/gorm"
)

// PointBatchScheduler 乐币批次每日清扫（#1947 Sub-D）：
// 过期失效（ExpireDueBatches）→ 到期提醒（RemindExpiringBatches）。
type PointBatchScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewPointBatchScheduler() *PointBatchScheduler {
	return &PointBatchScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *PointBatchScheduler) Start() {
	s.ticker = time.NewTicker(24 * time.Hour)
	go func() {
		// Run immediately on start, then daily.
		s.run()

		for range s.ticker.C {
			s.run()
		}
	}()
	log.Println("[PointBatchScheduler] started")
}

func (s *PointBatchScheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.done <- true
	log.Println("[PointBatchScheduler] stopped")
}

// RunForTest runs one pass. Exported for tests.
func (s *PointBatchScheduler) RunForTest() {
	s.run()
}

// SetDBForTest replaces the scheduler DB handle. Exported for tests.
func (s *PointBatchScheduler) SetDBForTest(db *gorm.DB) {
	s.db = db
}

func (s *PointBatchScheduler) run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PointBatchScheduler] panic: %v", r)
		}
	}()
	if n, total, err := ExpireDueBatches(s.db); err != nil {
		log.Printf("[PointBatchScheduler] expire failed: %v", err)
	} else if n > 0 {
		log.Printf("[PointBatchScheduler] expired %d batches, total %.2f", n, float64(total)/100)
	}
	if n, err := RemindExpiringBatches(s.db); err != nil {
		log.Printf("[PointBatchScheduler] remind failed: %v", err)
	} else if n > 0 {
		log.Printf("[PointBatchScheduler] reminded %d expiring batches", n)
	}
}
