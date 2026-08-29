package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	var expired []models.InstrumentMedia
	if err := s.db.Where("batch_type != ? AND created_at < ?", "display", cutoff).
		Find(&expired).Error; err != nil {
		return fmt.Errorf("failed to query expired media: %w", err)
	}

	if len(expired) == 0 {
		return nil
	}

	log.Printf("[MediaCleanupService] Deleting %d expired process records (older than %d days)", len(expired), retentionDays)

	storage := NewMediaStorage()
	ctx := context.Background()

	for _, m := range expired {
		// Delete the physical file plus derived variants.
		if err := storage.Delete(ctx, m.StorageKey); err != nil {
			log.Printf("[MediaCleanupService] delete file %s failed: %v", m.StorageKey, err)
		}
		base := strings.TrimSuffix(m.StorageKey, filepath.Ext(m.StorageKey))
		for _, suffix := range []string{"_display.webp", "_thumb.jpg"} {
			if err := storage.Delete(ctx, base+suffix); err != nil {
				log.Printf("[MediaCleanupService] delete derived %s failed: %v", base+suffix, err)
			}
		}
		// Mark the asset unreferenced in the registry (physical file already gone).
		if err := NewMediaRegistry().MarkUnreferenced(ctx, m.StorageKey); err != nil {
			log.Printf("[MediaCleanupService] mark %s unreferenced failed: %v", m.StorageKey, err)
		}
	}

	result := s.db.Where("batch_type != ? AND created_at < ?", "display", cutoff).
		Delete(&models.InstrumentMedia{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete expired media: %w", result.Error)
	}

	log.Printf("[MediaCleanupService] Deleted %d records from DB and their physical files.", result.RowsAffected)

	return nil
}

// orphanGraceDays reads media_orphan_grace_days from system_settings (default 30).
func (s *MediaCleanupService) orphanGraceDays() int {
	grace := 30
	var setting models.SystemSetting
	if err := s.db.Where("setting_key = ?", "media_orphan_grace_days").First(&setting).Error; err == nil {
		if days, err := strconv.Atoi(setting.SettingValue); err == nil && days > 0 {
			grace = days
		}
	}
	if grace < 1 {
		grace = 30
	}
	return grace
}

// GCOrphans reconciles the media_assets registry against the uploads/media/
// directory and deletes unreferenced files after the grace period. When dryRun
// is true it only reports what would be deleted.
func (s *MediaCleanupService) GCOrphans(dryRun bool) (int, error) {
	grace := s.orphanGraceDays()
	cutoff := time.Now().AddDate(0, 0, -grace)
	storage := NewMediaStorage()
	ctx := context.Background()
	deleted := 0

	// 1. Registry-driven: unreferenced assets past the grace period.
	// #1790 T2 R2 H3: face_capture 素材（生物特征合规数据）豁免孤儿回收。
	var orphans []models.MediaAsset
	if err := s.db.Where("is_referenced = ? AND last_referenced_at < ? AND source_type != ?",
		false, cutoff, "face_capture").
		Find(&orphans).Error; err != nil {
		return deleted, fmt.Errorf("failed to query orphan assets: %w", err)
	}
	for _, o := range orphans {
		if dryRun {
			log.Printf("[MediaCleanupService] DRY RUN would delete orphan asset %s (source_type=%s source_id=%s)", o.StorageKey, o.SourceType, o.SourceID)
			deleted++
			continue
		}
		if err := storage.Delete(ctx, o.StorageKey); err != nil {
			log.Printf("[MediaCleanupService] delete orphan %s failed: %v", o.StorageKey, err)
			continue
		}
		if err := s.db.Delete(&o).Error; err != nil {
			log.Printf("[MediaCleanupService] delete orphan record %s failed: %v", o.StorageKey, err)
			continue
		}
		deleted++
	}

	// 2. Directory sweep: files on disk absent from media_assets (and not the
	// derived _display.webp / _thumb.jpg variants of a registered key).
	registered, err := s.registeredKeySet()
	if err != nil {
		return deleted, err
	}
	onDisk, err := s.listMediaFiles()
	if err != nil {
		return deleted, err
	}
	for _, f := range onDisk {
		// #1790 T2 R2 H3: face_captures/ 目录（生物特征合规数据）即使注册
		// 异常也保留——跳过目录扫描，防止物理删除。
		if strings.HasPrefix(f, "face_captures/") {
			continue
		}
		if registered[f] {
			continue
		}
		// Derived variants are cleaned up alongside their base key, not here.
		if s.isDerivedVariant(f, registered) {
			continue
		}
		if dryRun {
			log.Printf("[MediaCleanupService] DRY RUN would delete unregistered file %s", f)
			deleted++
			continue
		}
		if err := storage.Delete(ctx, f); err != nil {
			log.Printf("[MediaCleanupService] delete unregistered %s failed: %v", f, err)
			continue
		}
		deleted++
	}

	// 3. Batch-import session directories older than the grace period.
	if err := s.gcBatchDirs(grace, dryRun); err != nil {
		return deleted, err
	}

	return deleted, nil
}

func (s *MediaCleanupService) registeredKeySet() (map[string]bool, error) {
	m := make(map[string]bool)

	// media_assets registry keys.
	var keys []string
	if err := s.db.Model(&models.MediaAsset{}).Pluck("storage_key", &keys).Error; err != nil {
		return nil, fmt.Errorf("failed to load registered keys: %w", err)
	}
	for _, k := range keys {
		m[k] = true
	}

	// instrument_media is the authoritative source for business media — its
	// storage keys must be protected from the directory sweep even when the
	// asset has not (yet) been mirrored into media_assets (e.g. files uploaded
	// before the registry existed).
	var mediaKeys []string
	if err := s.db.Model(&models.InstrumentMedia{}).Pluck("storage_key", &mediaKeys).Error; err != nil {
		return nil, fmt.Errorf("failed to load instrument_media keys: %w", err)
	}
	for _, k := range mediaKeys {
		m[k] = true
	}

	return m, nil
}

// isDerivedVariant reports whether key is "{base}_display.webp" or
// "{base}_thumb.jpg" of a registered base key (e.g. registered "a/b.webp"
// derives "a/b_display.webp" and "a/b_thumb.jpg").
func (s *MediaCleanupService) isDerivedVariant(key string, registered map[string]bool) bool {
	for regKey := range registered {
		base := strings.TrimSuffix(regKey, filepath.Ext(regKey))
		if key == base+"_display.webp" || key == base+"_thumb.jpg" {
			return true
		}
	}
	return false
}

// listMediaFiles returns all file paths under uploads/media/ relative to the
// base directory (storage keys).
func (s *MediaCleanupService) listMediaFiles() ([]string, error) {
	base := filepath.Join(".", "uploads", "media")
	var files []string
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to walk %s: %w", base, err)
	}
	return files, nil
}

// gcBatchDirs removes uploads/batch/{sessionID} directories older than grace days.
func (s *MediaCleanupService) gcBatchDirs(grace int, dryRun bool) error {
	base := filepath.Join(".", "uploads", "batch")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", base, err)
	}
	cutoff := time.Now().AddDate(0, 0, -grace)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		dir := filepath.Join(base, e.Name())
		if dryRun {
			log.Printf("[MediaCleanupService] DRY RUN would delete batch dir %s", dir)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("[MediaCleanupService] delete batch dir %s failed: %v", dir, err)
		}
	}
	return nil
}
