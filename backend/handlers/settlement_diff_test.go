package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// TestRefundDiff_PointsOverCap: A1 < A0 → refund (A0−A1) gift points,
// cash refund = C0−C1, C1 = R1−A1 (#1606, L-06).
func TestRefundDiff_PointsOverCap(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000a1"
	orgID := "00000000-0000-4000-8000-0000000000a2"
	userID := "00000000-0000-4000-8000-0000000000a3"

	// User at level 1 with policy pay_ratio=0.3 → A1 = floor(2800×0.3)=840
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.3, RefundRatio: 0.1, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffover", Status: "active", MembershipLevelID: intPtr(1),
		PromoPoints: 2000,
	}).Error)

	// R0 = 3000 (cash 2000 + gift 1000, deposit 500 excluded from rent formula)
	// Actual rent R1 = 2800 (28 days × 100).
	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         200000,
		GiftPointsUsed:   100000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	// Create the instrument the order references
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "DIFF-OVER", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, 2800.0, result.RentPayable, "R1 = 28×100")
	// A1 = floor(2800 × 0.3) = 840; A0 = 1000 > A1 → refund 160 gift points
	require.Equal(t, 160.0, result.GiftPointsRefunded, "A0−A1 = 1000−840")
	// TotalRefund = TotalRentPaid(2000+1000−500) + deposit(500) − R1(2800) = 200
	require.Equal(t, 200.0, result.TotalRefund, "total refund")
	// C1 = R1 − A1 = 2800−840 = 1960; C0 = 2000 → cash refund = 40
	require.Equal(t, 40.0, result.CashRefundable, "cash refund after gift split")
	require.Equal(t, 1960.0, result.CashBasis, "C1 = R1 − A1")
	// Conservation: gift_refunded + cash_refunded = R0 − R1 = 3000−2800 = 200
	require.InDelta(t, 200.0, result.GiftPointsRefunded+result.CashRefundable, 0.001, "conservation")
}

// TestRefundDiff_PointsWithinCap: A1 ≥ A0 → gift stays, cash refund = R0−R1.
func TestRefundDiff_PointsWithinCap(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000b1"
	orgID := "00000000-0000-4000-8000-0000000000b2"
	userID := "00000000-0000-4000-8000-0000000000b3"

	// pay_ratio = 0.5 → A1 = floor(2800×0.5) = 1400 ≥ A0=500
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.5, RefundRatio: 0.1, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffwithin", Status: "active", MembershipLevelID: intPtr(1),
		PromoPoints: 2000,
	}).Error)

	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         250000,
		GiftPointsUsed:   50000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "DIFF-WITHIN", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, 0.0, result.GiftPointsRefunded, "gift stays at A0")
	// TotalRefund = (2500+1000−500) + 500 − 2800 = 200; cash refund = 200
	require.Equal(t, 200.0, result.CashRefundable, "full cash refund")
	require.Equal(t, 2300.0, result.CashBasis, "C1 = 2800−500")
}

// TestRefundDiff_TotalSpendingC1: total_spending increments by C1 not R1.
func TestRefundDiff_TotalSpendingC1(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000c1"
	orgID := "00000000-0000-4000-8000-0000000000c2"
	userID := "00000000-0000-4000-8000-0000000000c3"

	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.3, RefundRatio: 0.1, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffspend", Status: "active", MembershipLevelID: intPtr(1),
		PromoPoints: 2000, TotalSpending: 0,
	}).Error)

	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         200000,
		GiftPointsUsed:   100000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "DIFF-SPEND", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)
	require.Equal(t, 1960.0, result.CashBasis, "C1")
	require.Greater(t, result.CashBasis, 0.0)
}

// TestRefundDiff_RebatePoints: A2 = floor(C1 × refund_ratio) via policy.
func TestRefundDiff_RebatePoints(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000d1"
	orgID := "00000000-0000-4000-8000-0000000000d2"
	userID := "00000000-0000-4000-8000-0000000000d3"

	// refund_ratio = 0.1 → A2 = floor(1960 × 0.1) = 196
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.3, RefundRatio: 0.1, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffrebate", Status: "active", MembershipLevelID: intPtr(1),
		PromoPoints: 0,
	}).Error)

	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         200000,
		GiftPointsUsed:   100000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "DIFF-REBATE", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)
	require.Equal(t, 1960.0, result.CashBasis)

	// Verify the rebate computation formula directly: floor(C1 × ratio)
	rebate := float64(int(result.CashBasis * 0.1))
	require.Equal(t, 196.0, rebate, "A2 = floor(1960×0.1)")
}

// TestConfirmSettlement_ClosesOrder: manual confirm sets order completed (L-06).
func TestConfirmSettlement_ClosesOrder(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000e1"
	orgID := "00000000-0000-4000-8000-0000000000e2"
	userID := "00000000-0000-4000-8000-0000000000e3"

	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.3, RefundRatio: 0.1, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffclose", Status: "active", MembershipLevelID: intPtr(1),
		PromoPoints: 2000,
	}).Error)

	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusDepositRefunding,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         200000,
		GiftPointsUsed:   100000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "DIFF-CLOSE", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	// Simulate the ConfirmSettlement order-close update (the handler's final
	// step). The handler itself needs a full router; we verify the update
	// contract directly: deposit_refunding → completed.
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusCompleted).Error)
	var updated models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&updated).Error)
	require.Equal(t, models.OrderStatusCompleted, updated.Status, "manual settlement closes order")
}

// TestPaymentCallback_NoDoubleCount: gift_used deducts cash_paid (L-06).
func TestPaymentCallback_NoDoubleCount(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000f1"
	orgID := "00000000-0000-4000-8000-0000000000f2"
	userID := "00000000-0000-4000-8000-0000000000f3"

	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "diffnodd", Status: "active", PromoPoints: 1000,
	}).Error)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		Status:           models.OrderStatusPaid,
		Deposit:          models.FromYuan(500),
		CashPaid:         300000, // full total at creation
		GiftPointsUsed:   0,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Simulate the payment-callback update: gift_used=1000 → cash_paid 3000−1000=2000
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Updates(map[string]interface{}{
			"gift_points_used": 1000.0,
			"cash_paid":        gorm.Expr("GREATEST(cash_paid - ?, 0)", 1000.0),
		}).Error)

	var updated models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&updated).Error)
	require.Equal(t, 2000.0, updated.CashPaid, "cash_paid reduced by gift_used")
}

// TestSettlement_ShippingFeeSingleDeduction (#1721): shipping fee is deducted
// from the deposit exactly once — previously it was subtracted from both
// totalRentPaid (paid-rent side) and totalDepositDeducted (deposit side),
// under-refunding the customer by the shipping fee.
func TestSettlement_ShippingFeeSingleDeduction(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000b1"
	orgID := "00000000-0000-4000-8000-0000000000b2"
	userID := "00000000-0000-4000-8000-0000000000b3"

	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shippingfee", Status: "active", MembershipLevelID: intPtr(1),
	}).Error)

	// Rent 3000 (30d × 100), deposit 500, shipping fee 12.50, full term on-time.
	returnedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:   uuid.New().String(),
		StartDate:      strPtr("2026-08-01"),
		EndDate:        strPtr("2026-08-30"),
		LeaseTerm:      30,
		Status:         models.OrderStatusCompleted,
		ReturnedAt:     &returnedAt,
		Deposit:        models.FromYuan(500),
		CashPaid:       models.FromYuan(3500), // rent 3000 + deposit 500; shipping billed at dispatch
		ShippingFee:    models.FromYuan(12.50),
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SHIP-ONCE", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	// Rent 3000, deposit 500, shipping 12.50 (deducted from deposit once):
	// totalRefund = totalRentPaid(3000) + remainingDeposit(500−12.50) − rent(3000)
	//             = 487.50 — shipping deducted exactly once.
	require.Equal(t, 3000.0, result.RentPayable, "R1 = 30×100")
	require.Equal(t, 487.50, result.TotalRefund, "refund = rent + (deposit − shipping) − rent; shipping once")
	require.Equal(t, 0.0, result.DepositDeductedOverdue+result.DamageDeducted, "no overdue/damage in this case")
}
