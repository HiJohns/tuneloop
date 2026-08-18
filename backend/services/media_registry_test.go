package services

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func newTestRegistry(t *testing.T) *MediaRegistry {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	// sqlite does not support gen_random_uuid(); use a plain TEXT primary key
	// with a gorm hook-free schema so Create does not attempt the Postgres default.
	if err := db.Exec(`CREATE TABLE media_assets (
		id TEXT PRIMARY KEY,
		storage_key TEXT NOT NULL UNIQUE,
		source_type TEXT NOT NULL,
		source_id TEXT,
		is_referenced NUMERIC NOT NULL DEFAULT 1,
		ref_count INTEGER NOT NULL DEFAULT 1,
		file_size INTEGER,
		file_type TEXT,
		created_at DATETIME,
		last_referenced_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create media_assets table: %v", err)
	}
	database.SetDB(db)
	return &MediaRegistry{db: db}
}

func TestRegisterAsset_IdempotentUpsert(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if err := r.RegisterAsset(ctx, "a/b.webp", SourceTypeContentImage, "", 100, "image"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	// Re-register same key: must not create a duplicate, must refresh.
	if err := r.RegisterAsset(ctx, "a/b.webp", SourceTypeAvatar, "u1", 200, "image"); err != nil {
		t.Fatalf("second register failed: %v", err)
	}

	var count int64
	r.db.Model(&models.MediaAsset{}).Where("storage_key = ?", "a/b.webp").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", count)
	}

	var asset models.MediaAsset
	if err := r.db.Where("storage_key = ?", "a/b.webp").First(&asset).Error; err != nil {
		t.Fatalf("failed to load asset: %v", err)
	}
	if asset.SourceType != SourceTypeAvatar || asset.SourceID != "u1" || asset.FileSize != 200 {
		t.Fatalf("upsert did not refresh fields: %+v", asset)
	}
	if !asset.IsReferenced {
		t.Fatalf("expected is_referenced true after register")
	}
}

func TestMarkUnreferenced(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if err := r.RegisterAsset(ctx, "x.webp", SourceTypeContentImage, "", 10, "image"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := r.MarkUnreferenced(ctx, "x.webp"); err != nil {
		t.Fatalf("mark unreferenced failed: %v", err)
	}
	var asset models.MediaAsset
	if err := r.db.Where("storage_key = ?", "x.webp").First(&asset).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if asset.IsReferenced {
		t.Fatalf("expected is_referenced false")
	}
}

func TestReconcileHTMLRefs(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	// Pre-register two assets bound to source "setting_a".
	if err := r.RegisterAsset(ctx, "keep.webp", SourceTypeContentImage, "setting_a", 10, "image"); err != nil {
		t.Fatalf("register keep failed: %v", err)
	}
	if err := r.RegisterAsset(ctx, "drop.webp", SourceTypeContentImage, "setting_a", 10, "image"); err != nil {
		t.Fatalf("register drop failed: %v", err)
	}

	html := `<p>hello <img src="/uploads/media/keep.webp"></p>`
	if err := r.ReconcileHTMLRefs(ctx, "setting_a", html); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var keep, drop models.MediaAsset
	r.db.Where("storage_key = ?", "keep.webp").First(&keep)
	r.db.Where("storage_key = ?", "drop.webp").First(&drop)

	if !keep.IsReferenced {
		t.Fatalf("keep.webp should remain referenced")
	}
	if keep.SourceID != "setting_a" {
		t.Fatalf("keep.webp source_id should be setting_a, got %s", keep.SourceID)
	}
	if drop.IsReferenced {
		t.Fatalf("drop.webp should be marked unreferenced (absent from html)")
	}
}

// TestReconcileHTMLRefs_RegistersHistoricalImage verifies the audit #1692
// CRITICAL-2 fix: a rich-text image that was never registered (uploaded before
// the registry existed) gets registered on the fly during reconcile, so the GC
// directory sweep keeps it.
func TestReconcileHTMLRefs_RegistersHistoricalImage(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	html := `<p>legacy <img src="/uploads/media/historical.webp"></p>`
	if err := r.ReconcileHTMLRefs(ctx, "setting_b", html); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var asset models.MediaAsset
	if err := r.db.Where("storage_key = ?", "historical.webp").First(&asset).Error; err != nil {
		t.Fatalf("historical image was not registered: %v", err)
	}
	if !asset.IsReferenced {
		t.Fatalf("historical image should be referenced")
	}
	if asset.SourceID != "setting_b" {
		t.Fatalf("historical image source_id should be setting_b, got %s", asset.SourceID)
	}
}

// TestRegisterAsset_RefCountBumps verifies the audit #1692 MEDIUM-3 fix:
// re-registering the same storage_key bumps ref_count instead of staying 1.
func TestRegisterAsset_RefCountBumps(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if err := r.RegisterAsset(ctx, "shared.webp", SourceTypeContentImage, "", 10, "image"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := r.RegisterAsset(ctx, "shared.webp", SourceTypeContentImage, "s1", 10, "image"); err != nil {
		t.Fatalf("second register failed: %v", err)
	}

	var asset models.MediaAsset
	if err := r.db.Where("storage_key = ?", "shared.webp").First(&asset).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if asset.RefCount != 2 {
		t.Fatalf("ref_count should be 2 after re-register, got %d", asset.RefCount)
	}
}
