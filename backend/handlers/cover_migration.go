package handlers

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// MigrateInstrumentCoverImages generates cover images for all instruments without one.
// Uses the first display image (instrument_media WHERE is_display=true, file_type='image')
// or falls back to images[0] from the JSONB array.
// dryRun=true: only logs what would be done, no DB writes.
func MigrateInstrumentCoverImages(dryRun bool) (int, error) {
	db := database.GetDB()
	uploadDir := "./uploads/media"

	var instruments []models.Instrument
	if err := db.Where("(cover_image IS NULL OR cover_image = '')").Find(&instruments).Error; err != nil {
		return 0, err
	}

	count := 0
	for _, inst := range instruments {
		// Try display media first
		var media models.InstrumentMedia
		if err := db.Where("instrument_id = ? AND is_display = ? AND file_type = ?", inst.ID, true, "image").
			Order("sort_order ASC").First(&media).Error; err == nil && media.StorageKey != "" {
			// Found display image, generate cover
			srcPath := filepath.Join(uploadDir, media.StorageKey)
			if err := generateCover(srcPath, inst.ID, uploadDir, dryRun); err != nil {
				log.Printf("[MigrateCover] Failed to generate cover for %s: %v", inst.ID, err)
				continue
			}
			count++
			continue
		}

		// Fallback: legacy images[] JSONB
		if inst.Images != "" && inst.Images != "[]" {
			var urls []string
			if err := json.Unmarshal([]byte(inst.Images), &urls); err == nil && len(urls) > 0 {
				srcPath := filepath.Join(uploadDir, filepath.Base(urls[0]))
				if err := generateCover(srcPath, inst.ID, uploadDir, dryRun); err != nil {
					log.Printf("[MigrateCover] Failed to generate cover from legacy for %s: %v", inst.ID, err)
					continue
				}
				count++
			}
		}
	}

	if !dryRun {
		log.Printf("[MigrateCover] Generated %d cover images", count)
	} else {
		log.Printf("[MigrateCover] DRY RUN: would generate %d cover images", count)
	}
	return count, nil
}

func generateCover(srcPath, instrumentID, uploadDir string, dryRun bool) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	coverData, err := services.ResizeToCoverSquare(data, 72)
	if err != nil {
		return err
	}

	if dryRun {
		return nil
	}

	coverKey := "cover_" + instrumentID + ".webp"
	dstPath := filepath.Join(uploadDir, coverKey)
	if err := os.WriteFile(dstPath, coverData, 0644); err != nil {
		return err
	}

	coverURL := "/uploads/media/" + coverKey
	db := database.GetDB()
	return db.Model(&models.Instrument{}).Where("id = ?", instrumentID).Update("cover_image", coverURL).Error
}
