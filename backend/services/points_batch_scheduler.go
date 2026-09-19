package services

import (
	"log"
	"time"

	"tuneloop-backend/database"

	"gorm.io/gorm"
)

// PointBatchScheduler 乐币批次每日清扫（#1947 Sub-D / B 条裁定）：
// 每日**北京时间 00:00**执行——过期失效（ExpireDueBatches，按用户合并通知）
// → 到期提醒（RemindExpiringBatches）。
type PointBatchScheduler struct {
	db   *gorm.DB
	stop chan struct{}
}

func NewPointBatchScheduler() *PointBatchScheduler {
	return &PointBatchScheduler{
		db:   database.GetDB(),
		stop: make(chan struct{}),
	}
}

// nextBeijingMidnight 返回下一个北京时间零点。
func nextBeijingMidnight(now time.Time) time.Time {
	n := now.In(pointBatchLocation)
	y, m, d := n.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, pointBatchLocation)
}

func (s *PointBatchScheduler) Start() {
	go func() {
		// 启动即补扫一次，随后对齐到北京时间每日零点。
		s.run()
		for {
			timer := time.NewTimer(time.Until(nextBeijingMidnight(time.Now())))
			select {
			case <-timer.C:
				s.run()
			case <-s.stop:
				timer.Stop()
				return
			}
		}
	}()
	log.Println("[PointBatchScheduler] started (daily @ 00:00 Asia/Shanghai)")
}

func (s *PointBatchScheduler) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
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
