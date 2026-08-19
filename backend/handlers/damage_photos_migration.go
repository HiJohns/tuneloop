package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// MigrateDamageAssessmentPhotos backfills staff photos from the deprecated
// damage_assessments.photos JSONB into instrument_media (batch_type=receiving,
// is_display=false) — the authoritative media store (#1708/#1710). Idempotent:
// URLs already present in instrument_media are skipped. dryRun only reports.
func MigrateDamageAssessmentPhotos(dryRun bool) (int, error) {
	db := database.GetDB()
	var assessments []models.DamageAssessment
	if err := db.Where("photos IS NOT NULL AND photos != '' AND photos != '[]'").
		Find(&assessments).Error; err != nil {
		return 0, fmt.Errorf("failed to query assessments: %w", err)
	}

	backfilled := 0
	for _, a := range assessments {
		var urls []string
		if err := json.Unmarshal([]byte(a.Photos), &urls); err != nil || len(urls) == 0 {
			continue
		}
		for _, u := range urls {
			if u == "" {
				continue
			}
			// Normalize storage_key (URLs may be full paths).
			key := u
			if len(key) > len("/uploads/media/") && key[:len("/uploads/media/")] == "/uploads/media/" {
				key = key[len("/uploads/media/"):]
			}
			// Skip if already present for this instrument.
			var cnt int64
			db.Model(&models.InstrumentMedia{}).
				Where("instrument_id = ? AND storage_key = ?", a.InstrumentID, key).
				Count(&cnt)
			if cnt > 0 {
				continue
			}
			if dryRun {
				log.Printf("[DamagePhotosMigrate] DRY RUN would backfill %s for instrument %s (assessment %s)", key, a.InstrumentID, a.ID)
				backfilled++
				continue
			}
			media := models.InstrumentMedia{
				ID:           uuid.New().String(),
				TenantID:     a.TenantID,
				OrgID:        a.OrgID,
				InstrumentID: &a.InstrumentID,
				BatchID:      uuid.New().String(),
				BatchType:    "receiving",
				FileName:     key,
				FileType:     "image",
				StorageKey:   key,
				IsDisplay:    false,
				CreatedAt:    time.Now(),
			}
			if err := db.Create(&media).Error; err != nil {
				log.Printf("[DamagePhotosMigrate] backfill %s failed: %v", key, err)
				continue
			}
			backfilled++
		}
	}
	return backfilled, nil
}
