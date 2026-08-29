package services

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

// TestGCOrphans_ExemptsFaceCapture (#1790 T2 R2 H3): face_capture 素材
// （生物特征合规数据）在孤儿回收 + 目录扫描两路径均豁免。
func TestGCOrphans_ExemptsFaceCapture(t *testing.T) {
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

	// 过期未引用 face_capture asset（应豁免——即使 is_referenced=false 且超期）。
	old := "2020-01-01"
	db.Exec(`INSERT INTO media_assets (id, storage_key, source_type, source_id, is_referenced, ref_count, created_at, last_referenced_at)
		VALUES ('fc1', 'face_captures/u1/b1/selfie.jpg', 'face_capture', 'b1', 0, 0, ?, ?)`, old, old)
	// 普通内容 asset（不应豁免——验证查询仍会命中非 face_capture）。
	db.Exec(`INSERT INTO media_assets (id, storage_key, source_type, source_id, is_referenced, ref_count, created_at, last_referenced_at)
		VALUES ('c1', 'content.webp', 'content_image', 's1', 0, 0, ?, ?)`, old, old)

	// 创建真实 uploads/media 目录 + face_captures 文件（目录扫描豁免验证）。
	base := filepath.Join(".", "uploads", "media")
	requireNoErr(t, os.MkdirAll(filepath.Join(base, "face_captures", "u1", "b1"), 0o755))
	requireNoErr(t, os.WriteFile(filepath.Join(base, "face_captures", "u1", "b1", "selfie.jpg"), []byte("x"), 0o644))
	// 未注册的普通文件（应被目录扫描删除——验证扫描仍工作）。
	requireNoErr(t, os.WriteFile(filepath.Join(base, "unregistered.webp"), []byte("x"), 0o644))
	defer os.RemoveAll(filepath.Join(".", "uploads", "media"))

	s := &MediaCleanupService{db: db}
	deleted, err := s.GCOrphans(false)
	if err != nil {
		t.Fatalf("GCOrphans failed: %v", err)
	}

	// 孤儿查询：face_capture 豁免（fc1 保留），content_image 被删（c1 删除）。
	var fc1, c1 models.MediaAsset
	if err := db.Where("id = ?", "fc1").First(&fc1).Error; err != nil {
		t.Errorf("face_capture orphan asset was deleted — GC exemption broken")
	}
	if err := db.Where("id = ?", "c1").First(&c1).Error; err == nil {
		t.Errorf("content_image orphan asset not deleted — GC query regression")
	}

	// 目录扫描：face_captures/ 文件保留（即使注册异常），未注册普通文件被删。
	if _, err := os.Stat(filepath.Join(base, "face_captures", "u1", "b1", "selfie.jpg")); err != nil {
		t.Errorf("face_captures/ file deleted by directory sweep — GC exemption broken")
	}
	if _, err := os.Stat(filepath.Join(base, "unregistered.webp")); err == nil {
		t.Errorf("unregistered file not deleted by directory sweep — sweep regression")
	}
	_ = deleted
}
