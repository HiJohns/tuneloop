package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// TestComputeSettlement_SegmentScenarios 固化 #1836 用户确认场景在
// computeSettlement 全链路的净额结果（合同 1 天 ¥36/天 + 续费 1+1 天）：
//
//	场景 1：合同/续1/续2 均 ENO（实付 0.36/0.36/0.36）→ 实租 1 天 → 补缴 0.28
//	场景 2A：续1 无码（36.00）其余 ENO → 实租 1 天 → 应退 35.36
//	场景 2B：同上 → 实租 2 天 → 应补 0.64
//
// 物流 ¥1.00 不打折；押金 0（免押）。
func TestComputeSettlement_SegmentScenarios(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	cents := func(yuan float64) models.Cents { return models.Cents(int64(yuan*100 + 0.5)) }
	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SEG1836", StockStatus: "rented",
	}).Error)

	// pricing: 第1阶梯 3 天内 ¥36/天；合同租 1 天。
	pb := `{"base_daily_rent":3600,"rent_days":1,"deposit":0,"total_amount":3600,
		"tiers":[{"days_max":3,"daily_rate":3600,"discount_percent":0}],
		"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}]}`

	mkOrder := func(t *testing.T, cash float64, delivered, returned time.Time, renewals [][]interface{}) models.Order {
		t.Helper()
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-01"),
			LeaseTerm: 1, Status: models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(cash), ShippingFee: cents(1),
			PricingBreakdown: str1743Ptr(pb),
		}
		require.NoError(t, db.Create(&o).Error)
		for i, r := range renewals {
			days := r[0].(int)
			amount := r[1].(float64)
			require.NoError(t, db.Create(&models.OrderPaymentRecord{
				TenantID: tenantID, UserID: userID, OrderID: &o.ID,
				OrderType: "renewal", Type: "payment", Status: "paid",
				Amount: cents(amount), Days: &days,
				CreatedAt: time.Date(2026, 8, 1, 10+i, 0, 0, 0, time.UTC),
			}).Error)
		}
		return o
	}

	t.Run("场景1-三码-实租1天-补缴0.28", func(t *testing.T) {
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)
		o := mkOrder(t, 1.08, delivered, returned, [][]interface{}{{1, 0.36}, {1, 0.36}})
		res := computeSettlement(o, db)
		require.True(t, res.SegmentModel)
		require.InDelta(t, 0.36, res.DiscountedRent, 1e-9)
		require.InDelta(t, 1.36, res.DiscountedDue, 1e-9)
		require.InDelta(t, 0, res.TotalRefund, 1e-9)
		require.InDelta(t, 0.28, res.PayableShortfall, 1e-9)
		require.Equal(t, int64(28), res.Breakdown["payable_shortfall"])
	})

	t.Run("场景2A-续1无码-实租1天-应退35.36", func(t *testing.T) {
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)
		o := mkOrder(t, 36.72, delivered, returned, [][]interface{}{{1, 36.00}, {1, 0.36}})
		res := computeSettlement(o, db)
		require.True(t, res.SegmentModel)
		require.InDelta(t, 0.36, res.DiscountedRent, 1e-9)
		require.InDelta(t, 1.36, res.DiscountedDue, 1e-9)
		require.InDelta(t, 35.36, res.TotalRefund, 1e-9)
		require.InDelta(t, 0, res.PayableShortfall, 1e-9)
	})

	t.Run("场景2B-续1无码-实租2天-应补0.64", func(t *testing.T) {
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
		o := mkOrder(t, 36.72, delivered, returned, [][]interface{}{{1, 36.00}, {1, 0.36}})
		res := computeSettlement(o, db)
		require.True(t, res.SegmentModel)
		require.InDelta(t, 36.36, res.DiscountedRent, 1e-9)
		require.InDelta(t, 37.36, res.DiscountedDue, 1e-9)
		require.Zero(t, res.TotalRefund)
		require.InDelta(t, 0.64, res.PayableShortfall, 1e-9)
	})

	t.Run("历史单-lease_term=0-回退1743-场景1金额0.28", func(t *testing.T) {
		// #1838 回归：历史订单 lease_term=0（legacy），段模型不得启用
		// （rent_days 被续费重写为覆盖天数，合同段无法拆分）→ #1743 兜底。
		// 该单结构同场景1（三码 0.36×3、实租 1 天、物流 1.00、CouponDiscount
		// 单笔面值 35.64）→ 旧口径补缴 0.28（与用户确认数值一致）。
		tenantID := uuid.New().String()
		orgID := tenantID
		userID := uuid.New().String()
		instID := uuid.New().String()
		require.NoError(t, db.Create(&models.Instrument{
			ID: instID, TenantID: tenantID, OrgID: &orgID,
			SN: "SN-SEG-LT0", StockStatus: "rented",
		}).Error)
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-01"),
			LeaseTerm:   0, // legacy 单：无初始合同天数
			Status:      models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(1.08), ShippingFee: cents(1),
			CouponDiscount: cents(35.64), // #1743 单笔面值口径
			PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":3,"deposit":0,"total_amount":10800,
				"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
				"tier_segments":[{"tier":1,"days":3,"rate":3600,"discount":1,"subtotal":10800}]}`),
		}
		require.NoError(t, db.Create(&o).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, UserID: userID, OrderID: &o.ID,
			OrderType: "renewal", Type: "payment", Status: "paid",
			Amount: cents(0.36), Days: intPtr(1),
		}).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, UserID: userID, OrderID: &o.ID,
			OrderType: "renewal", Type: "payment", Status: "paid",
			Amount: cents(0.36), Days: intPtr(1),
		}).Error)
		res := computeSettlement(o, db)
		require.False(t, res.SegmentModel, "lease_term=0 不得启用段模型")
		require.InDelta(t, 0.28, res.PayableShortfall, 1e-9, "#1743 兜底口径：补缴 0.28")
		require.Zero(t, res.TotalRefund)
	})

	t.Run("跨阶-无码-实租12天-段级应收与rentPayable逐分相等", func(t *testing.T) {
		// 合同 30 天跨两阶：tier1 10天@¥20 + tier2 20天@¥15 = 原价 ¥500 全额支付；
		// 实租 12 天 → rentPayable（tier 截断）= 10×20 + 2×15 = ¥230。
		// 回归点：#1836 Round1 审计——整段均匀折算曾得 ¥220，子段化后必须相等。
		tenantID := uuid.New().String()
		orgID := tenantID
		userID := uuid.New().String()
		instID := uuid.New().String()
		require.NoError(t, db.Create(&models.Instrument{
			ID: instID, TenantID: tenantID, OrgID: &orgID,
			SN: "SN-SEG-XTIER", StockStatus: "rented",
		}).Error)
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC) // 12 天
		xt := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-31"),
			LeaseTerm: 30, Status: models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(500), ShippingFee: 0,
			PricingBreakdown: str1743Ptr(`{"base_daily_rent":2000,"rent_days":30,"deposit":0,"total_amount":50000,
				"pricing_tiers":[{"days_max":10,"daily_rate":2000,"discount_percent":0},{"days_max":30,"daily_rate":1500,"discount_percent":0}],
				"tier_segments":[{"tier":1,"days":10,"rate":2000,"discount":1,"subtotal":20000},
				                {"tier":2,"days":20,"rate":1500,"discount":1,"subtotal":30000}]}`),
		}
		require.NoError(t, db.Create(&xt).Error)
		res := computeSettlement(xt, db)
		require.True(t, res.SegmentModel)
		require.InDelta(t, 230.0, res.RentPayable, 1e-9)
		require.InDelta(t, 230.0, res.DiscountedRent, 1e-9, "无码段级应收必须与 rentPayable 逐分相等")
		require.InDelta(t, 270.0, res.TotalRefund, 1e-9, "应退 500−230")
		require.Equal(t, []int64{10, 2}, res.Breakdown["segment_usage"])
	})
}
