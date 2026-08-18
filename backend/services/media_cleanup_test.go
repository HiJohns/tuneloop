package services

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"tuneloop-backend/database"
)

func TestIsDerivedVariant(t *testing.T) {
	registered := map[string]bool{"a/b.webp": true}
	s := &MediaCleanupService{}

	cases := []struct {
		key  string
		want bool
	}{
		{"a/b_display.webp", true},
		{"a/b_thumb.jpg", true},
		{"a/b.webp", false},
		{"unregistered_display.webp", false},
	}
	for _, c := range cases {
		if got := s.isDerivedVariant(c.key, registered); got != c.want {
			t.Errorf("isDerivedVariant(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestOrphanGraceDays_DefaultAndConfigured(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.Exec(`CREATE TABLE system_settings (
		id TEXT PRIMARY KEY,
		tenant_id TEXT,
		setting_key TEXT,
		setting_value TEXT,
		updated_at DATETIME,
		updated_by TEXT
	)`)
	database.SetDB(db)
	s := &MediaCleanupService{db: db}

	if got := s.orphanGraceDays(); got != 30 {
		t.Errorf("default grace = %d, want 30", got)
	}

	db.Exec(`INSERT INTO system_settings (id, tenant_id, setting_key, setting_value) VALUES ('1','','media_orphan_grace_days','45')`)
	if got := s.orphanGraceDays(); got != 45 {
		t.Errorf("configured grace = %d, want 45", got)
	}
}

// TestRegisteredKeySet_MergesInstrumentMedia verifies the audit #1692
// CRITICAL-1 fix: the directory sweep protection set includes instrument_media
// storage keys (authoritative source) even when the asset is absent from the
// media_assets registry (historical files).
func TestRegisteredKeySet_MergesInstrumentMedia(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.Exec(`CREATE TABLE media_assets (
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
	)`)
	db.Exec(`CREATE TABLE instrument_media (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		org_id TEXT,
		instrument_id TEXT,
		batch_id TEXT NOT NULL,
		batch_type TEXT,
		file_name TEXT,
		file_type TEXT,
		file_size INTEGER,
		storage_key TEXT NOT NULL,
		is_display NUMERIC,
		sort_order INTEGER,
		created_at DATETIME
	)`)
	database.SetDB(db)

	db.Exec(`INSERT INTO media_assets (id, storage_key, source_type, source_id, is_referenced, ref_count, created_at, last_referenced_at) VALUES ('m1', 'content.webp', 'content_image', 'setting_a', 1, 1, '2026-08-18', '2026-08-18')`)
	db.Exec(`INSERT INTO instrument_media (id, tenant_id, org_id, batch_id, batch_type, storage_key, created_at) VALUES ('i1', 't1', 'o1', 'b1', 'display', 't1/o1/legacy_display.webp', '2026-08-01')`)

	s := &MediaCleanupService{db: db}
	keys, err := s.registeredKeySet()
	if err != nil {
		t.Fatalf("registeredKeySet failed: %v", err)
	}
	if !keys["content.webp"] {
		t.Errorf("media_assets key missing from protection set")
	}
	if !keys["t1/o1/legacy_display.webp"] {
		t.Errorf("instrument_media key (historical, unregistered in media_assets) missing from protection set — CRITICAL-1 not fixed")
	}
}
