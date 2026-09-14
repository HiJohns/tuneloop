package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1918: 未归还订单的实际租金天数必须与下单定价口径一致（含尾自然日）。
// 下单用 CalculateDays（end 23:59:59，含尾）= 3 天；此前结算 actualDays 用
// CalculateLeaseDays(start, end)（ceil-hours/24）= 2 天 → 详情显示与下单冲突。
// 修复：未归还时 actualLeaseEnd = end_date + 1 天；已归还订单维持
// CalculateLeaseDays(delivered, returned) 不变。

func seed1918Order(t *testing.T, status string, deliveredAt, returnedAt *time.Time) models.Order {
	t.Helper()
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageReport{}, &models.OverdueCharge{}, &models.Settlement{}))

	tenantID := "00000000-0000-0000-0000-000000001918"
	orgID := "00000000-0000-0000-0000-000000001919"
	userID := "00000000-0000-0000-0000-000000001920"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "1918-" + time.Now().Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	// 3-day lease (start 09-15 .. end 09-17, 含尾) at ¥100/day
	pricingBreakdown := `{"base_daily_rent":10000,"rent_days":3,"tiers":[{"days_max":3,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":3,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`
	startDate := "2026-09-15"
	endDate := "2026-09-17"

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        3,
		Status:           status,
		DeliveredAt:      deliveredAt,
		ReturnedAt:       returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         models.FromYuan(3500),
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)
	return order
}

// TestSettlement_NotEnded_InclusiveDays: shipped (未归还) order shows the
// same 3 days the customer paid for — not 2.
func TestSettlement_NotEnded_InclusiveDays(t *testing.T) {
	delivered := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	order := seed1918Order(t, "shipped", &delivered, nil)

	result := computeSettlement(order, database.GetDB())
	require.Equal(t, 3, result.ActualDays, "未归还订单 actualDays 必须与下单 rent_days(含尾) 一致")
	require.Equal(t, 300.0, result.RentPayable)
}

// TestSettlement_Returned_LeaseDaysUnchanged: returned orders keep the
// #1738/#1665 CalculateLeaseDays rule (delivered → returned, ceil, min 1)
// — the #1918 +1 adjustment must NOT leak into ended leases.
func TestSettlement_Returned_LeaseDaysUnchanged(t *testing.T) {
	// delivered 09-15 00:00 → returned 09-17 00:00 = exactly 48h → 2 days
	delivered := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	order := seed1918Order(t, "returned", &delivered, &returned)

	result := computeSettlement(order, database.GetDB())
	require.Equal(t, 2, result.ActualDays, "已归还订单按 delivered→returned 差值法（48h=2天），不受 #1918 影响")
}
