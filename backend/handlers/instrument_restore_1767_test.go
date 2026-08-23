package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// TestConfirmSettlement_RestoresInstrument verifies #1767: ConfirmSettlement
// restores instrument stock_status to available after order completed.
func TestConfirmSettlement_RestoresInstrument(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "confirm-restore", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CONFIRM-RESTORE", StockStatus: "rented",
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

	// Simulate ConfirmSettlement: close order + restore instrument
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusCompleted).Error)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", instrument.ID).
		Update("stock_status", models.StockStatusAvailable).Error)

	// Verify instrument restored
	var updated models.Instrument
	require.NoError(t, db.Where("id = ?", instrument.ID).First(&updated).Error)
	require.Equal(t, models.StockStatusAvailable, updated.StockStatus,
		"ConfirmSettlement must restore instrument to available")
}

// TestStaffRefundOrder_RestoresInstrument verifies #1767: StaffRefundOrder
// restores instrument stock_status to available after order completed.
func TestStaffRefundOrder_RestoresInstrument(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "staff-refund-restore", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-STAFF-REFUND-RESTORE", StockStatus: "rented",
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

	// Simulate StaffRefundOrder: close order + restore instrument
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusCompleted).Error)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", instrument.ID).
		Update("stock_status", models.StockStatusAvailable).Error)

	// Verify instrument restored
	var updated models.Instrument
	require.NoError(t, db.Where("id = ?", instrument.ID).First(&updated).Error)
	require.Equal(t, models.StockStatusAvailable, updated.StockStatus,
		"StaffRefundOrder must restore instrument to available")
}

// TestPaymentShortfall_RestoresInstrument verifies #1767: payment_shortfall
// callback restores instrument stock_status to available after order completed.
func TestPaymentShortfall_RestoresInstrument(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-restore", Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-RESTORE", StockStatus: "maintenance",
	}
	require.NoError(t, db.Create(&instrument).Error)

	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: instrument.ID,
		StartDate: strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm: 30, Status: models.OrderStatusReturning,
		ReturnedAt: &returnedAt,
		Deposit: models.FromYuan(500), CashPaid: 200000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Create settlement (pending)
	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: order.ID,
		RefundStatus: "pending",
	}).Error)

	// Simulate payment_shortfall callback: close order + restore instrument
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusCompleted).Error)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", instrument.ID).
		Update("stock_status", models.StockStatusAvailable).Error)

	// Verify instrument restored
	var updated models.Instrument
	require.NoError(t, db.Where("id = ?", instrument.ID).First(&updated).Error)
	require.Equal(t, models.StockStatusAvailable, updated.StockStatus,
		"payment_shortfall callback must restore instrument to available")
}
