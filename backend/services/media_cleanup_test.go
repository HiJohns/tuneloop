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
