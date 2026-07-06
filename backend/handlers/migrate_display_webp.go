package handlers

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// MigrateDisplayImagesToWebP converts existing JPEG/PNG display images to WebP format (1080×1440).
func MigrateDisplayImagesToWebP(dryRun bool) (int, error) {
	db := database.GetDB()
	uploadDir := "./uploads/media"

	var media []models.InstrumentMedia
	if err := db.Where("file_type = ?", "image").Find(&media).Error; err != nil {
		return 0, err
	}

	count := 0
	skipped := 0
	for _, m := range media {
		baseKey := strings.TrimSuffix(m.StorageKey, filepath.Ext(m.StorageKey))
		displayKey := baseKey + "_display.webp"
		dstPath := filepath.Join(uploadDir, displayKey)

		if _, err := os.Stat(dstPath); err == nil {
			skipped++
			continue
		}

		srcPath := filepath.Join(uploadDir, m.StorageKey)
		webpData, err := convertFileToWebP(srcPath)
		if err != nil {
			log.Printf("[MigrateWebP] Failed to convert %s: %v", m.StorageKey, err)
			continue
		}

		if dryRun {
			count++
			continue
		}

		if err := os.WriteFile(dstPath, webpData, 0644); err != nil {
			log.Printf("[MigrateWebP] Failed to write %s: %v", dstPath, err)
			continue
		}
		count++
	}

	if dryRun {
		log.Printf("[MigrateWebP] DRY RUN: would convert %d images (%d skipped, already have WebP)", count, skipped)
	} else {
		log.Printf("[MigrateWebP] Converted %d images to WebP (%d skipped, already have WebP)", count, skipped)
	}
	return count, nil
}

func convertFileToWebP(srcPath string) ([]byte, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	return services.GenerateThumbnailWebP(data, 1080, 1440)
}

// PreviewMigrateDisplayImages shows a summary of what would be converted.
func PreviewMigrateDisplayImages() (total, already, needsConvert int, err error) {
	db := database.GetDB()
	uploadDir := "./uploads/media"

	var media []models.InstrumentMedia
	if err := db.Where("file_type = ?", "image").Find(&media).Error; err != nil {
		return 0, 0, 0, err
	}

	for _, m := range media {
		total++
		baseKey := strings.TrimSuffix(m.StorageKey, filepath.Ext(m.StorageKey))
		displayKey := baseKey + "_display.webp"
		dstPath := filepath.Join(uploadDir, displayKey)

		if _, err := os.Stat(dstPath); err == nil {
			already++
		} else if _, err := os.Stat(filepath.Join(uploadDir, m.StorageKey)); err == nil {
			needsConvert++
		}
	}
	return
}
