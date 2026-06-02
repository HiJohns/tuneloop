package handlers

import (
	"log"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

type LogisticsMonitor struct {
	done chan bool
}

func NewLogisticsMonitor() *LogisticsMonitor {
	return &LogisticsMonitor{
		done: make(chan bool),
	}
}

func (m *LogisticsMonitor) Start() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		m.check()
		for {
			select {
			case <-ticker.C:
				m.check()
			case <-m.done:
				ticker.Stop()
				return
			}
		}
	}()
	log.Println("[LogisticsMonitor] started - checks hourly for stale forwarding sessions")
}

func (m *LogisticsMonitor) Stop() {
	m.done <- true
}

func (m *LogisticsMonitor) check() {
	db := database.GetDB()
	cutoff := time.Now().Add(-7 * 24 * time.Hour) // 7 days stale

	var staleSessions []models.ForwardingSession
	if err := db.Where("status IN ? AND updated_at < ?",
		[]string{models.ForwardingStatusInTransit, models.ForwardingStatusLastMile},
		cutoff).Find(&staleSessions).Error; err != nil {
		log.Printf("[LogisticsMonitor] Failed to query: %v", err)
		return
	}

	for _, s := range staleSessions {
		db.Model(&s).Update("status", models.ForwardingStatusLost)
		if s.InstrumentID != "" {
			db.Table("instruments").Where("id = ?", s.InstrumentID).Update("stock_status", models.StockStatusLost)
		}
		db.Create(&models.Notification{
			ID:        uuid.New().String(),
			TenantID:  s.TenantID,
			OrgID:     s.OrgID,
			Type:      "forwarding_lost",
			Title:     "转发包裹超时丢失",
			Content:   "转发会话 " + s.SessionCode + " 超过 7 天未更新，系统已自动标记为丢失。",
			Status:    "unread",
			CreatedAt: time.Now(),
		})
		log.Printf("[LogisticsMonitor] Auto-lost forwarding session %s (code: %s)", s.ID, s.SessionCode)
	}
}
