package database

import (
	"testing"

	"tuneloop-backend/models"
)

// TestValidateModelColumns_MissingTableFails verifies #1716: a persistence
// model whose table is missing must fail validation (previously returned nil,
// letting merchant_members-type incidents slip through). Exempt models are
// skipped.
func TestValidateModelColumns_MissingTableFails(t *testing.T) {
	cfg := LoadConfig()
	db, err := InitDB(cfg)
	if err != nil {
		t.Skip("dev db not available")
		return
	}
	SetDB(db)

	// MerchantMember exists (migration 20260818005) → passes.
	if err := validateModelColumns(db, &models.MerchantMember{}); err != nil {
		t.Errorf("MerchantMember should validate: %v", err)
	}

	// Simulate a missing-table model: create a model with a table that does
	// not exist in dev DB — use a fake type whose GORM table name is unique.
	type MissingTableModel struct{}
	// GORM table name would be "missing_table_models" — not present.
	var tblCount int64
	db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'missing_table_models'`).Scan(&tblCount)
	if tblCount > 0 {
		t.Skip("missing_table_models unexpectedly exists")
	}
	err = validateModelColumns(db, &MissingTableModel{})
	if err == nil {
		t.Errorf("expected missing-table failure for unknown model, got nil")
	}
}

// fakeUserModel maps to the real users table via TableName() but includes a
// column that does NOT exist in the real schema (#1771 M2). Without
// TableName(), GORM would derive "fake_user_models" (a missing table) and the
// test would pass for the wrong reason (#1770 tautological lesson).
type fakeUserModel struct {
	ID      string `gorm:"column:id"`
	FakeCol string `gorm:"column:nonexistent_column_xyz"`
}

func (fakeUserModel) TableName() string { return "users" }

// TestValidateModelColumns_MissingColumnFails verifies #1771 M2: a model whose
// table exists but is missing a column must fail validation. This catches
// models that drift out of sync with the schema (e.g. a new field added to the
// Go struct but no migration applied).
func TestValidateModelColumns_MissingColumnFails(t *testing.T) {
	cfg := LoadConfig()
	db, err := InitDB(cfg)
	if err != nil {
		t.Skip("dev db not available")
		return
	}
	SetDB(db)

	// users table is guaranteed to exist. fakeUserModel maps to it but
	// includes a column that does NOT exist in the real schema.
	err = validateModelColumns(db, &fakeUserModel{})
	if err == nil {
		t.Errorf("expected missing-column failure for fakeUserModel (nonexistent_column_xyz), got nil")
	}
}
