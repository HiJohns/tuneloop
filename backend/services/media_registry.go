package services

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// Media source types for the media_assets registry.
const (
	SourceTypeContentImage     = "content_image"
	SourceTypeAvatar           = "avatar"
	SourceTypeIDPhoto          = "id_photo"
	SourceTypeInstrumentMedia  = "instrument_media"
)

// mediaKeyPattern matches physical files under /uploads/media/ embedded in rich
// text (e.g. <img src="/uploads/media/123_abcd.webp">).
var mediaKeyPattern = regexp.MustCompile(`/uploads/media/([A-Za-z0-9._/-]+)`)

// MediaRegistry reconciles physical media files with the media_assets index.
type MediaRegistry struct {
	db *gorm.DB
}

func NewMediaRegistry() *MediaRegistry {
	return &MediaRegistry{db: database.GetDB()}
}

// RegisterAsset upserts a media_assets record. Idempotent: repeated
// registration of the same storage_key does not duplicate; the existing row is
// refreshed, its ref_count bumped, and last_referenced_at refreshed.
func (r *MediaRegistry) RegisterAsset(ctx context.Context, storageKey, sourceType, sourceID string, fileSize int64, fileType string) error {
	db := r.db.WithContext(ctx)
	now := time.Now()

	asset := models.MediaAsset{
		ID:               uuid.New().String(),
		StorageKey:       storageKey,
		SourceType:       sourceType,
		SourceID:         sourceID,
		IsReferenced:     true,
		RefCount:         1,
		FileSize:         fileSize,
		FileType:         fileType,
		CreatedAt:        now,
		LastReferencedAt: now,
	}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "storage_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"is_referenced":      true,
			"source_type":        sourceType,
			"source_id":          sourceID,
			"file_size":          fileSize,
			"file_type":          fileType,
			"ref_count":          gorm.Expr("media_assets.ref_count + 1"),
			"last_referenced_at": now,
		}),
	}).Create(&asset).Error
}

// MarkReferenced marks a storage_key as still referenced and refreshes its
// last_referenced_at timestamp.
func (r *MediaRegistry) MarkReferenced(ctx context.Context, storageKey string) error {
	return r.db.WithContext(ctx).Model(&models.MediaAsset{}).
		Where("storage_key = ?", storageKey).
		Updates(map[string]interface{}{
			"is_referenced":      true,
			"last_referenced_at": time.Now(),
		}).Error
}

// MarkUnreferenced marks a storage_key as no longer referenced (e.g. rich-text
// image removed, avatar/id_photo replaced). The orphan GC deletes it after the
// grace period.
func (r *MediaRegistry) MarkUnreferenced(ctx context.Context, storageKey string) error {
	return r.db.WithContext(ctx).Model(&models.MediaAsset{}).
		Where("storage_key = ?", storageKey).
		Updates(map[string]interface{}{
			"is_referenced":      false,
			"last_referenced_at": time.Now(),
		}).Error
}

// ReconcileHTMLRefs extracts every /uploads/media/<key> reference from an HTML
// blob and marks the corresponding assets as referenced (refreshing their
// last_referenced_at), binding them to sourceID. Keys not yet registered (e.g.
// files uploaded before the registry existed) are registered on the fly so the
// directory sweep never treats a referenced rich-text image as an orphan.
// Assets previously bound to the same source_id that are no longer present in
// the HTML are marked unreferenced.
func (r *MediaRegistry) ReconcileHTMLRefs(ctx context.Context, sourceID, html string) error {
	db := r.db.WithContext(ctx)

	seen := make(map[string]bool)
	for _, m := range mediaKeyPattern.FindAllStringSubmatch(html, -1) {
		key := strings.TrimSuffix(m[1], "/")
		if key == "" {
			continue
		}
		seen[key] = true
	}

	now := time.Now()
	for key := range seen {
		var cnt int64
		if err := db.Model(&models.MediaAsset{}).Where("storage_key = ?", key).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			// Historical rich-text image not in the registry yet — register it
			// so the GC directory sweep keeps it (audit #1692 CRITICAL-2).
			if err := r.RegisterAsset(ctx, key, SourceTypeContentImage, sourceID, 0, "image"); err != nil {
				return err
			}
			continue
		}
		// Existing asset: refresh reference state + bind to this source.
		if err := db.Model(&models.MediaAsset{}).
			Where("storage_key = ?", key).
			Updates(map[string]interface{}{
				"is_referenced":      true,
				"source_id":          sourceID,
				"source_type":        SourceTypeContentImage,
				"last_referenced_at": now,
			}).Error; err != nil {
			return err
		}
	}

	// Mark assets bound to this source_id but absent from the HTML as unreferenced.
	return db.Model(&models.MediaAsset{}).
		Where("source_id = ? AND source_type = ? AND is_referenced = ?", sourceID, SourceTypeContentImage, true).
		Where("storage_key NOT IN (?)", r.selectableKeys(seen)).
		Updates(map[string]interface{}{
			"is_referenced":      false,
			"last_referenced_at": now,
		}).Error
}

// selectableKeys converts the seen set into a slice usable by NOT IN (?).
// Returns a placeholder key when empty so NOT IN never receives an empty list.
func (r *MediaRegistry) selectableKeys(seen map[string]bool) []string {
	if len(seen) == 0 {
		return []string{"__no_assets__"}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}
