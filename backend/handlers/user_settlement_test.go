package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// TestSettlement_EarlyReturnRebate verifies #1494: paid 30 days but returned
// at day 28 → 2 unused days are rebated (tier-prorated).
func TestSettlement_EarlyReturnRebate(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageAssessment{}, &models.DamageReport{}, &models.OverdueCharge{}, &models.Settlement{}))

	tenantID := "00000000-0000-0000-0000-0000000000e1"
	orgID := "00000000-0000-0000-0000-0000000000e2"
	userID := "00000000-0000-0000-0000-0000000000e3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "STL-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(100),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	// 30-day lease at tier 0 (30 days, no discount): total = 100×30 = 3000
	pricingBreakdown := `{"base_daily_rent":100,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":30,"rate":100,"discount":1,"subtotal":3000}],"total_amount":3000}`
	startDate := "2026-07-01"
	endDate := "2026-07-30"
	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        30,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          500,
		CashPaid:         3000,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	result := computeSettlement(order, db)

	// Actual days = 2026-07-01 .. 2026-07-28 = 28
	require.Equal(t, 28, result.ActualDays)
	// Rent payable = 28 × 100 = 2800; rebate = 3000 - 2800 = 200
	require.Equal(t, 2800.0, result.RentPayable)
	rebate := result.TotalRentPaid - result.RentPayable
	require.Equal(t, 200.0, rebate, "early-return rebate = paid - actual prorated rent")
	// Refund = 3000 (rent) + 500 (deposit) - 2800 = 700
	require.Equal(t, 700.0, result.TotalRefund)
}

// TestSettlement_OverdueFeeDeducted verifies the overdue fee from return
// inspection is deducted from the deposit and the remainder refunded.
func TestSettlement_OverdueFeeDeducted(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageAssessment{}, &models.DamageReport{}, &models.OverdueCharge{}, &models.Settlement{}))

	tenantID := "00000000-0000-0000-0000-0000000000f1"
	orgID := "00000000-0000-0000-0000-0000000000f2"
	userID := "00000000-0000-0000-0000-0000000000f3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "STL2-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(100),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	pricingBreakdown := `{"base_daily_rent":100,"rent_days":10,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":10,"rate":100,"discount":1,"subtotal":1000}],"total_amount":1000}`
	startDate := "2026-08-01"
	endDate := "2026-08-10"
	returnedAt := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        10,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          500,
		CashPaid:         1000,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	// Overdue fee computed at return inspection (5 days × 15 = 75), persisted
	// on the assessment.
	require.NoError(t, db.Create(&models.DamageAssessment{
		ID:           newTestUUID(),
		TenantID:     tenantID,
		OrgID:        orgID,
		OrderID:      order.ID,
		InstrumentID: instrument.ID,
		UserID:       userID,
		Condition:    "good",
		Photos:       "[]",
		Status:       "completed",
		OverdueDays:  5,
		OverdueFee:   75,
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, 75.0, result.DepositDeductedOverdue, "overdue fee deducted from deposit")
	require.Equal(t, 425.0, result.RemainingDeposit, "deposit 500 - overdue 75")
	// Actual days = 8-01 .. 8-15 = 15 > lease 10; rent payable stays 1000
	require.Equal(t, 1000.0, result.RentPayable)
	// Refund = 1000 (rent) + 425 (remaining deposit) - 1000 (rent payable) = 425
	require.Equal(t, 425.0, result.TotalRefund)
}

// TestSettlement_DamagePlusOverdue verifies combined damage + overdue deduction.
func TestSettlement_DamagePlusOverdue(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageAssessment{}, &models.DamageReport{}, &models.OverdueCharge{}, &models.Settlement{}))

	tenantID := "00000000-0000-0000-0000-0000000000a1"
	orgID := "00000000-0000-0000-0000-0000000000a2"
	userID := "00000000-0000-0000-0000-0000000000a3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "STL3-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(100),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	pricingBreakdown := `{"base_daily_rent":100,"rent_days":10,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":10,"rate":100,"discount":1,"subtotal":1000}],"total_amount":1000}`
	startDate := "2026-09-01"
	endDate := "2026-09-10"
	returnedAt := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        10,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          500,
		CashPaid:         1000,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	// Assessment: overdue 2 days × 15 = 30
	require.NoError(t, db.Create(&models.DamageAssessment{
		ID:           newTestUUID(),
		TenantID:     tenantID,
		OrgID:        orgID,
		OrderID:      order.ID,
		InstrumentID: instrument.ID,
		UserID:       userID,
		Condition:    "damaged",
		Photos:       "[]",
		Status:       "damaged",
		OverdueDays:  2,
		OverdueFee:   30,
	}).Error)
	// Damage report: 100 deducted from deposit
	damageAmount := 100.0
	require.NoError(t, db.Create(&models.DamageReport{
		ID:           newTestUUID(),
		TenantID:     tenantID,
		OrgID:        orgID,
		LeaseID:      order.ID,
		InstrumentID: instrument.ID,
		UserID:       userID,
		DamageAmount: &damageAmount,
		DepositDeducted: 100,
		Status:       "accepted",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, 30.0, result.DepositDeductedOverdue)
	require.Equal(t, 100.0, result.DamageDeducted)
	require.Equal(t, 370.0, result.RemainingDeposit, "500 - 30 - 100")
	// Actual days 12 ≤ 10? No: 09-01..09-12 = 12 > 10, rent payable stays 1000
	require.Equal(t, 1000.0, result.RentPayable)
	require.Equal(t, 370.0, result.TotalRefund, "1000 + 370 - 1000")
}

func newTestUUID() string { return uuid.New().String() }
