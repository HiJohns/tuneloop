package handlers

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

const bannerMaxWidth = 1040
const bannerMaxHeight = 1440

var bannerImageExts = []string{".jpg", ".jpeg", ".png"}

// MigrateBannerImagesToWebP converts legacy JPEG/PNG banner images to WebP
// and updates banners.image_url accordingly. Source files are kept for safety.
func MigrateBannerImagesToWebP(dryRun bool) (int, error) {
	db := database.GetDB()
	uploadDir := "./uploads/media"
	ctx := context.Background()
	storage := services.NewMediaStorage()

	var banners []models.Banner
	if err := db.Find(&banners).Error; err != nil {
		return 0, err
	}

	count := 0
	skipped := 0

	for _, banner := range banners {
		if !strings.HasPrefix(banner.ImageURL, "/uploads/media/") {
			skipped++
			continue
		}

		key := strings.TrimPrefix(banner.ImageURL, "/uploads/media/")
		ext := strings.ToLower(filepath.Ext(key))

		isLegacy := false
		for _, e := range bannerImageExts {
			if ext == e {
				isLegacy = true
				break
			}
		}
		if !isLegacy {
			skipped++
			continue
		}

		webpKey := strings.TrimSuffix(key, ext) + ".webp"
		newURL := "/uploads/media/" + webpKey

		_, webpExists, statErr := storage.Stat(ctx, webpKey)
		if statErr == nil && webpExists {
			// WebP already on disk — just update DB reference
			if !dryRun {
				if err := db.Model(&banner).Update("image_url", newURL).Error; err != nil {
					log.Printf("[MigrateBannerWebP] Failed to update banner %s: %v", banner.ID, err)
					continue
				}
			}
			count++
			continue
		}

		// Convert source image
		srcPath := filepath.Join(uploadDir, key)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			log.Printf("[MigrateBannerWebP] Cannot read source %s for banner %s: %v", key, banner.ID, err)
			continue
		}

		webpData, err := services.GenerateThumbnailWebP(data, bannerMaxWidth, bannerMaxHeight)
		if err != nil {
			log.Printf("[MigrateBannerWebP] Failed to convert %s for banner %s: %v", key, banner.ID, err)
			continue
		}

		if dryRun {
			count++
			continue
		}

		if err := storage.Upload(ctx, webpKey, bytes.NewReader(webpData), "image/webp"); err != nil {
			log.Printf("[MigrateBannerWebP] Failed to write %s: %v", webpKey, err)
			continue
		}

		if err := db.Model(&banner).Update("image_url", newURL).Error; err != nil {
			log.Printf("[MigrateBannerWebP] Failed to update banner %s after writing webp: %v", banner.ID, err)
			continue
		}

		count++
	}

	if dryRun {
		log.Printf("[MigrateBannerWebP] DRY RUN: would convert %d banners (%d skipped)", count, skipped)
	} else {
		log.Printf("[MigrateBannerWebP] Converted %d banners to WebP (%d skipped)", count, skipped)
	}

	return count, nil
}
