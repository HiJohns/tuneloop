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
