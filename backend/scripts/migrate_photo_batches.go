// +build ignore

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

// One-time migration: convert old InstrumentPhotoBatch ZIPs to instrument_media records.
// Usage: go run scripts/migrate_photo_batches.go
func main() {
	db := database.GetDB()

	var batches []models.InstrumentPhotoBatch
	if err := db.Find(&batches).Error; err != nil {
		log.Fatalf("Failed to query photo batches: %v", err)
	}
	fmt.Printf("Found %d legacy photo batches\n", len(batches))

	for _, batch := range batches {
		zipPath := batch.StoragePath
		if !filepath.IsAbs(zipPath) {
			execDir, _ := os.Getwd()
			zipPath = filepath.Join(execDir, zipPath)
		}

		reader, err := zip.OpenReader(zipPath)
		if err != nil {
			log.Printf("SKIP batch %s: cannot open ZIP %s: %v", batch.ID, zipPath, err)
			continue
		}

		batchID := uuid.New().String()
		var created int

		for _, f := range reader.File {
			if f.FileInfo().IsDir() {
				continue
			}

			ext := strings.ToLower(filepath.Ext(f.Name))
			fileType := "image"
			if ext == ".mp4" || ext == ".webm" || ext == ".mov" {
				fileType = "video"
			}

			destKey := fmt.Sprintf("%s_%s", batchID[:8], f.Name)
			destPath := filepath.Join("./uploads/media", destKey)

			os.MkdirAll(filepath.Dir(destPath), 0755)

			src, err := f.Open()
			if err != nil {
				log.Printf("  SKIP %s: cannot open in ZIP: %v", f.Name, err)
				continue
			}

			dst, err := os.Create(destPath)
			if err != nil {
				src.Close()
				log.Printf("  SKIP %s: cannot create %s: %v", f.Name, destPath, err)
				continue
			}

			if _, err := io.Copy(dst, src); err != nil {
				src.Close()
				dst.Close()
				log.Printf("  SKIP %s: copy error: %v", f.Name, err)
				continue
			}
			src.Close()
			dst.Close()

			media := models.InstrumentMedia{
				TenantID:     "", // filled manually
				InstrumentID: batch.InstrumentID,
				BatchID:      batchID,
				BatchType:    batch.BatchType,
				FileName:     f.Name,
				FileType:     fileType,
				StorageKey:   destKey,
				IsDisplay:    false,
			}
			if err := db.Create(&media).Error; err != nil {
				log.Printf("  FAIL %s: DB error: %v", f.Name, err)
				continue
			}
			created++
		}

		reader.Close()
		fmt.Printf("  Batch %s (%s): migrated %d files → batch_id=%s\n", batch.ID, batch.BatchType, created, batchID)
	}

	fmt.Println("Migration complete.")
	fmt.Println("Note: tenant_id fields need manual update. Run: UPDATE instrument_media SET tenant_id = i.tenant_id FROM instruments i WHERE instrument_media.instrument_id = i.id;")
}
