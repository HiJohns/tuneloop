package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1763 regression tests for the c() amount conversion bug:
//   - math.Round already rounds half away from zero; the legacy `+0.5`
//     caused a systematic +1-cent shift: 0 → 1, 0.01 → 2, whole yuan +1.
//   - After the fix, zero deductions stay 0 and fractional yuan map exactly.

// TestComputeSettlement_NoDamageNoOverdue_ZeroCents (#1763): an order with
// no damage report and no overdue must report damage_deducted / overdue_fee
// = 0 — never an inflated 1 cent (legacy c(0) = Round(0.5) = 1).
func TestComputeSettlement_NoDamageNoOverdue_ZeroCents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) // within lease → no overdue
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      0,
		CashPaid:     models.Cents(1), // 1 cent paid
		// Snapshot: single 1-day segment at ¥0.01/day (=1 cent)
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1,"rent_days":1,"tiers":[{"days_max":1,"discount_percent":0,"daily_rate":1}],"tier_segments":[{"tier":1,"days":1,"rate":1,"discount":1,"subtotal":1}],"total_amount":1}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CBUG-ZERO", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	// Zero deductions must stay zero (legacy c(0) = 1).
	require.Equal(t, int64(0), result.Breakdown["damage_deducted"], "no damage report → 0 cents")
	require.Equal(t, int64(0), result.Breakdown["overdue_fee"], "no overdue → 0 cents")
	require.Equal(t, int64(0), result.Breakdown["deposit_deducted_damage"], "no damage → 0 cents")
	require.Equal(t, int64(0), result.Breakdown["deposit_deducted_overdue"], "no overdue → 0 cents")
	require.Equal(t, int64(0), result.Breakdown["deposit_deducted_shipping"], "no shipping fee → 0 cents")

	// 1 cent amounts must stay 1 (legacy c(0.01) = 2).
	require.Equal(t, int64(1), result.Breakdown["total_rent_paid"], "1 cent paid → 1 cent")
	require.Equal(t, int64(1), result.Breakdown["rent_payable"], "¥0.01 payable → 1 cent")
}

// TestComputeSettlement_WholeYuanNotShifted (#1763): whole-yuan amounts must
// not gain +1 cent (legacy c(36.0) = Round(3600.5) = 3601).
func TestComputeSettlement_WholeYuanNotShifted(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC) // 47h → 2 days (TierOverflowDays fixture)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      0,
		CashPaid:     models.FromYuan(36), // ¥36.00
		// Snapshot: single 1-day segment at ¥36/day (=3600 cents)
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":1,"tiers":[{"days_max":1,"discount_percent":0,"daily_rate":3600}],"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}],"total_amount":3600}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CBUG-YUAN", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, int64(3600), result.Breakdown["total_rent_paid"], "¥36.00 → 3600 cents, not 3601")
	require.Equal(t, int64(3600), result.Breakdown["rent_payable"], "¥36.00 → 3600 cents, not 3601")
	// Rent exactly consumed (1 covered day @ ¥36) → refund 0 cents, never 1.
	require.Equal(t, int64(0), result.Breakdown["total_refund"], "refund 0 → 0 cents, not 1")
}

// TestComputeSettlement_FractionalCents (#1763): floating-point boundaries
// like ¥0.29 must map to 29 cents (legacy c(0.29) = Round(29.0000...02+0.5)).
func TestComputeSettlement_FractionalCents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) // within lease → no overdue
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      0,
		CashPaid:     models.Cents(29), // 29 cents
		// Snapshot: single 1-day segment at ¥0.29/day (=29 cents)
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":29,"rent_days":1,"tiers":[{"days_max":1,"discount_percent":0,"daily_rate":29}],"tier_segments":[{"tier":1,"days":1,"rate":29,"discount":1,"subtotal":29}],"total_amount":29}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CBUG-FRAC", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, int64(29), result.Breakdown["total_rent_paid"], "¥0.29 → 29 cents")
	require.Equal(t, int64(29), result.Breakdown["rent_payable"], "¥0.29 → 29 cents")
	// Rent exactly consumed (1 covered day @ ¥0.29) → refund 0 cents, never 1.
	require.Equal(t, int64(0), result.Breakdown["total_refund"], "refund 0 → 0 cents, not 1")
}
