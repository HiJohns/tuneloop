package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func main() {
	db := database.GetDB()
	storage := services.NewMediaStorage()

	if err := MigrateMediaKeys(db, storage); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	fmt.Println("Migration completed successfully.")
}

func MigrateMediaKeys(db *gorm.DB, storage services.MediaStorage) error {
	var records []models.InstrumentMedia
	db.Find(&records)

	migrated := 0
	skipped := 0
	failed := 0

	for _, r := range records {
		if strings.Contains(r.StorageKey, "/") {
			skipped++
			continue
		}

		fileName := filepath.Base(r.StorageKey)
		newKey := fmt.Sprintf("%s/%s/%s_%s_%d_%s",
			r.TenantID, r.OrgID,
			uuid.New().String()[:8],
			r.BatchType,
			r.CreatedAt.Unix(),
			fileName)

		if err := storage.Copy(context.Background(), r.StorageKey, newKey); err != nil {
			log.Printf("WARN: Failed to copy %s to %s: %v", r.StorageKey, newKey, err)
			failed++
			continue
		}

		if err := db.Model(&r).Update("storage_key", newKey).Error; err != nil {
			log.Printf("WARN: Failed to update storage_key for %s: %v", r.ID, err)
			failed++
			continue
		}

		migrated++
	}

	fmt.Printf("Migrated: %d, Skipped (already structured): %d, Failed: %d\n", migrated, skipped, failed)
	return nil
}
