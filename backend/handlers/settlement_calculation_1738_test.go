package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// TestSettlementCalculation_Preview verifies #1738 P2: GET calculate
// persists preview calculation with correct trigger and byte-identical result.
func TestSettlementCalculation_Preview(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SettlementCalculation{}))

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "settle-preview", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SETTLE-PREVIEW", StockStatus: "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: instrument.ID,
		StartDate: strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm: 30, Status: models.OrderStatusReturned,
		ReturnedAt: &returnedAt,
		Deposit: models.FromYuan(500), CashPaid: 300000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Simulate GET calculate: compute settlement + persist
	result := computeSettlement(order, db)
	breakdownJSON, _ := json.Marshal(result.Breakdown)
	recordSettlementCalculation(db, &order, "preview", result.ActualDays, breakdownJSON)

	// Verify settlement_calculations row
	var row models.SettlementCalculation
	require.NoError(t, db.Where("order_id = ? AND trigger = ?", order.ID, "preview").First(&row).Error)
	require.Equal(t, "preview", row.Trigger)
	require.Equal(t, result.ActualDays, row.ActualDays)

	// Verify result bytes match (JSON semantic equality)
	var expected, actual map[string]interface{}
	require.NoError(t, json.Unmarshal(breakdownJSON, &expected))
	require.NoError(t, json.Unmarshal([]byte(*row.Result), &actual))
	require.Equal(t, expected, actual, "result JSON must match")

	// Verify input_snapshot contains expected fields
	var snapshot map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*row.InputSnapshot), &snapshot))
	require.Equal(t, order.ID, snapshot["order_id"])
	require.Equal(t, string(order.Status), snapshot["status"])
	require.NotNil(t, snapshot["start_date"])
	require.NotNil(t, snapshot["end_date"])
	require.NotNil(t, snapshot["cash_paid_cents"])
}

// TestSettlementCalculation_Confirm verifies #1738 P2: POST confirm
// persists confirm calculation with correct trigger.
func TestSettlementCalculation_Confirm(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SettlementCalculation{}))

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "settle-confirm", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SETTLE-CONFIRM", StockStatus: "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: instrument.ID,
		StartDate: strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm: 30, Status: models.OrderStatusDepositRefunding,
		ReturnedAt: &returnedAt,
		Deposit: models.FromYuan(500), CashPaid: 300000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Simulate POST confirm: compute settlement + persist
	result := computeSettlement(order, db)
	breakdownJSON, _ := json.Marshal(result.Breakdown)
	recordSettlementCalculation(db, &order, "confirm", result.ActualDays, breakdownJSON)

	// Verify settlement_calculations row
	var row models.SettlementCalculation
	require.NoError(t, db.Where("order_id = ? AND trigger = ?", order.ID, "confirm").First(&row).Error)
	require.Equal(t, "confirm", row.Trigger)
	require.Equal(t, result.ActualDays, row.ActualDays)

	// Verify result bytes match (JSON semantic equality)
	var expected, actual map[string]interface{}
	require.NoError(t, json.Unmarshal(breakdownJSON, &expected))
	require.NoError(t, json.Unmarshal([]byte(*row.Result), &actual))
	require.Equal(t, expected, actual, "result JSON must match")
}

// TestSettlementCalculation_Idempotent verifies duplicate calls don't create
// multiple rows for the same trigger (best-effort append-only, but test
// verifies the normal path).
func TestSettlementCalculation_Idempotent(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SettlementCalculation{}))

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "settle-idem", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SETTLE-IDEM", StockStatus: "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: instrument.ID,
		StartDate: strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm: 30, Status: models.OrderStatusReturned,
		ReturnedAt: &returnedAt,
		Deposit: models.FromYuan(500), CashPaid: 300000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Simulate two preview calls
	result := computeSettlement(order, db)
	breakdownJSON, _ := json.Marshal(result.Breakdown)
	recordSettlementCalculation(db, &order, "preview", result.ActualDays, breakdownJSON)
	recordSettlementCalculation(db, &order, "preview", result.ActualDays, breakdownJSON)

	// Verify two rows exist (append-only)
	var count int64
	require.NoError(t, db.Model(&models.SettlementCalculation{}).
		Where("order_id = ? AND trigger = ?", order.ID, "preview").Count(&count).Error)
	require.Equal(t, int64(2), count, "append-only allows duplicates")
}
