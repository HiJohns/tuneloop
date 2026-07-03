package services

import (
	"fmt"
	"log"
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
	log.Println("[MediaCleanupService] started - runs daily (retention: 180 days default)")
}

func (s *MediaCleanupService) Stop() {
	s.done <- true
}

func (s *MediaCleanupService) clean() error {
	retentionDays := 180
	var setting models.SystemSetting
	if err := s.db.Where("setting_key = ?", "media_retention_days").First(&setting).Error; err == nil {
		if days, err := strconv.Atoi(setting.SettingValue); err == nil && days > 0 {
			retentionDays = days
		}
	}
	if retentionDays < 1 {
		retentionDays = 180
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Only delete process record types (not display)
	var count int64
	s.db.Model(&models.InstrumentMedia{}).
		Where("batch_type != ? AND created_at < ?", "display", cutoff).
		Count(&count)

	if count == 0 {
		return nil
	}

	log.Printf("[MediaCleanupService] Deleting %d expired process records (older than %d days)", count, retentionDays)

	result := s.db.Where("batch_type != ? AND created_at < ?", "display", cutoff).
		Delete(&models.InstrumentMedia{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete expired media: %w", result.Error)
	}

	// Also delete the physical files via storage (simplified: log only)
	log.Printf("[MediaCleanupService] Deleted %d records from DB. Physical file cleanup requires storage service sweep.", result.RowsAffected)

	return nil
}
