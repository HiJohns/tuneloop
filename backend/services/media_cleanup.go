package services

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type MediaCleanupService struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewMediaCleanupService() *MediaCleanupService {
	return &MediaCleanupService{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *MediaCleanupService) Start() {
	s.ticker = time.NewTicker(24 * time.Hour)
	go func() {
		s.clean()
		for {
			select {
			case <-s.ticker.C:
				if err := s.clean(); err != nil {
					log.Printf("[MediaCleanupService] clean error: %v", err)
				}
			case <-s.done:
				s.ticker.Stop()
				return
			}
		}
	}()
	log.Println("[MediaCleanupService] started - runs daily")
}

func (s *MediaCleanupService) Stop() {
	s.done <- true
}

func (s *MediaCleanupService) clean() error {
	retentionYears := 5
	if envVal := os.Getenv("MEDIA_RETENTION_YEARS"); envVal != "" {
		if parsed, err := strconv.Atoi(envVal); err == nil && parsed > 0 {
			retentionYears = parsed
		}
	}

	cutoff := time.Now().AddDate(-retentionYears, 0, 0)

	var expiredBatches []struct {
		BatchID   string
		BatchType string
		Count     int
	}
	if err := s.db.Model(&models.InstrumentMedia{}).
		Select("batch_id, batch_type, count(*) as count").
		Where("created_at < ?", cutoff).
		Group("batch_id, batch_type").
		Find(&expiredBatches).Error; err != nil {
		return fmt.Errorf("failed to query expired media: %w", err)
	}

	if len(expiredBatches) == 0 {
		return nil
	}

	log.Printf("[MediaCleanupService] Found %d expired media batch(es) older than %d years", len(expiredBatches), retentionYears)
	for _, b := range expiredBatches {
		log.Printf("[MediaCleanupService]   Batch %s (%s): %d files", b.BatchID, b.BatchType, b.Count)
	}

	log.Printf("[MediaCleanupService] WARNING: %d expired batch(es) requires manual confirmation to delete. Run: UPDATE instrument_media SET deleted_at = NOW() WHERE created_at < NOW() - INTERVAL '%d years'", len(expiredBatches), retentionYears)

	return nil
}
