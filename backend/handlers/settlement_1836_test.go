package handlers

import (
	"strconv"
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

	mkOrder := func(t *testing.T, cash float64, delivered, returned time.Time, renewals [][]interface{}) models.Order {
		t.Helper()
		// 真实世界：续费会把 pricing_breakdown.rent_days 累加为全部覆盖天数
		// （renewal.go）→ fixture 需模拟：rent_days = 初始 1 + Σ续费 days。
		renewalDays := 0
		for _, r := range renewals {
			renewalDays += r[0].(int)
		}
		totalDays := 1 + renewalDays
		pbJSON := `{"base_daily_rent":3600,"rent_days":` + strconv.Itoa(totalDays) + `,"deposit":0,"total_amount":3600,
			"tiers":[{"days_max":3,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}]}`
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-01"),
			LeaseTerm: 1, Status: models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(cash), ShippingFee: cents(1),
			PricingBreakdown: str1743Ptr(pbJSON),
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
		// #1837: discount_amount fields (plan §4)
		paidBlock := res.Breakdown["paid_block"].(map[string]interface{})
		contractRent := paidBlock["contract_rent"].(map[string]interface{})
		require.Equal(t, int64(3564), contractRent["discount_amount"])
		renewals := paidBlock["renewals"].([]map[string]interface{})
		require.Len(t, renewals, 2)
		for i, renewal := range renewals {
			require.Equal(t, int64(3564), renewal["discount_amount"], "renewal %d", i)
		}
		payableBlock := res.Breakdown["payable_block"].(map[string]interface{})
		require.Equal(t, int64(3564), payableBlock["discount_amount"])
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
		// #1837: discount_amount fields (plan §4)
		paidBlock := res.Breakdown["paid_block"].(map[string]interface{})
		contractRent := paidBlock["contract_rent"].(map[string]interface{})
		require.Equal(t, int64(3564), contractRent["discount_amount"])
		renewals := paidBlock["renewals"].([]map[string]interface{})
		require.Len(t, renewals, 2)
		// renewal1: no coupon (36.00) -> discount_amount absent or zero
		if d, exists := renewals[0]["discount_amount"]; exists {
			require.Equal(t, int64(0), d, "renewal1 discount_amount should be zero")
		}
		// renewal2: ENO -> discount_amount 3564
		require.Equal(t, int64(3564), renewals[1]["discount_amount"])
		payableBlock := res.Breakdown["payable_block"].(map[string]interface{})
		require.Equal(t, int64(3564), payableBlock["discount_amount"])
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

	t.Run("反推合同天数-lease_term=0(月语义)-三码实租1天-段模型补0.28", func(t *testing.T) {
		// #1838 二次修正：lease_term 为月语义（日租=0），段模型改为
		// rent_days(3) − Σ续费days(1+1) = 1 天反推合同初始天数 → 段模型启用。
		// 该单结构同场景1（三码 0.36×3、实租 1 天、物流 1.00）→ 补缴 0.28。
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
			LeaseTerm:   0, // 月语义：日租单 0（#1838 二次修正前误禁用段模型）
			Status:      models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(1.08), ShippingFee: cents(1),
			CouponDiscount: cents(35.64),
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
		require.True(t, res.SegmentModel, "合同天数反推（3−2=1）应启用段模型")
		require.InDelta(t, 0.36, res.DiscountedRent, 1e-9)
		require.InDelta(t, 0.28, res.PayableShortfall, 1e-9, "段模型口径：补缴 0.28")
		require.Zero(t, res.TotalRefund)
	})

	t.Run("1bf456df形状-两码两段实租1天-段模型补0.64", func(t *testing.T) {
		// 预生产 1bf456df 取证：rent_days=2（合同1天+续费1天）、lease_term=0
		// （月语义）、两笔 0.36 实付、实租 1 天、物流 1.00。
		// 修复前 lease_term=0 误回退 #1743 → 显示待补缴租金 ¥35.28（原价差）。
		// 反推合同 1 天 → 折后应收 0.36+1.00=1.36 → 补 1.36−0.72=0.64。
		tenantID := uuid.New().String()
		orgID := tenantID
		userID := uuid.New().String()
		instID := uuid.New().String()
		require.NoError(t, db.Create(&models.Instrument{
			ID: instID, TenantID: tenantID, OrgID: &orgID,
			SN: "SN-SEG-1BF", StockStatus: "rented",
		}).Error)
		delivered := time.Date(2026, 9, 8, 0, 53, 0, 0, time.UTC)
		returned := time.Date(2026, 9, 8, 0, 55, 0, 0, time.UTC)
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-09-08"), EndDate: str1743Ptr("2026-09-09"),
			LeaseTerm: 0, Status: models.OrderStatusReturning,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(0.72), ShippingFee: cents(1),
			CouponDiscount: cents(35.64),
			PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
				"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
				"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
		}
		require.NoError(t, db.Create(&o).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, UserID: userID, OrderID: &o.ID,
			OrderType: "renewal", Type: "payment", Status: "paid",
			Amount: cents(0.36), Days: intPtr(1),
		}).Error)
		res := computeSettlement(o, db)
		require.True(t, res.SegmentModel, "rent_days−renewal(2−1=1) 应启用段模型")
		require.InDelta(t, 0.36, res.DiscountedRent, 1e-9)
		require.InDelta(t, 1.36, res.DiscountedDue, 1e-9)
		require.InDelta(t, 0.64, res.PayableShortfall, 1e-9, "补缴 1.36−0.72=0.64")
		require.Zero(t, res.TotalRefund)
	})

	t.Run("续费缺days-合同天数不可反推-回退1743", func(t *testing.T) {
		tenantID := uuid.New().String()
		orgID := tenantID
		userID := uuid.New().String()
		instID := uuid.New().String()
		require.NoError(t, db.Create(&models.Instrument{
			ID: instID, TenantID: tenantID, OrgID: &orgID,
			SN: "SN-SEG-NODAYS", StockStatus: "rented",
		}).Error)
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-01"),
			LeaseTerm: 0, Status: models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0, CashPaid: cents(0.72), ShippingFee: cents(1),
			PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
				"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
				"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
		}
		require.NoError(t, db.Create(&o).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, UserID: userID, OrderID: &o.ID,
			OrderType: "renewal", Type: "payment", Status: "paid",
			Amount: cents(0.36), Days: nil, // legacy 续费缺 days → 不可反推
		}).Error)
		res := computeSettlement(o, db)
		require.False(t, res.SegmentModel, "续费缺 days 应回退 #1743")
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
		// #1837: no-coupon -> discount_amount = tier discount (base rate vs tier rate)
		paidBlock := res.Breakdown["paid_block"].(map[string]interface{})
		contractRent := paidBlock["contract_rent"].(map[string]interface{})
		require.Equal(t, int64(10000), contractRent["discount_amount"], "tier discount should be baseDailyRent*contractDays - contractRent")
		renewals := paidBlock["renewals"].([]map[string]interface{})
		require.Empty(t, renewals, "no renewals")
		payableBlock := res.Breakdown["payable_block"].(map[string]interface{})
		if discount, exists := payableBlock["discount_amount"]; exists {
			require.Equal(t, int64(0), discount, "no-coupon payable discount_amount should be zero")
		}
	})

	t.Run("跨阶-无码-续费跨30天边界-renewalOffset与contractDays续接", func(t *testing.T) {
		// #1837 Round2 REJECT「发现 2」裁决路径 A（先证后修）：
		// 日租单 lease_term=0（月语义）→ initialDays 回退 coverDays=rent_days=33
		// （合同 28 + 续费 5 的总覆盖）。合同段已按反推 contractDays=28 展开，
		// 续费真实发生在合同之后 → 展示层 renewalOffset 必须从 contractDays(28)
		// 续接（(28,33] = tier1 2 天 + tier2 3 天）；若仍从 initialDays(33) 起步
		// → (33,38] 整段落入 tier2（5% off），与合同段之间出现 5 天空洞，
		// tier 展开错误且 discount_amount 与真实支付脱节。
		// 判别断言：renewal tiers 必须为 2 段 {tier1:2 天全价, tier2:3 天 5% off}，
		// 无码 → discount_amount 必须 absent（原价 subtotal = 实付，差 0）。
		tenantID := uuid.New().String()
		orgID := tenantID
		userID := uuid.New().String()
		instID := uuid.New().String()
		require.NoError(t, db.Create(&models.Instrument{
			ID: instID, TenantID: tenantID, OrgID: &orgID,
			SN: "SN-SEG-XOFFSET", StockStatus: "rented",
		}).Error)
		delivered := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
		returned := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC) // 实租 33 天 = 全段覆盖 → 平账
		days5 := 5
		o := models.Order{
			TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
			StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-09-03"),
			LeaseTerm:   0, // 月语义：日租单 0 → fallback coverDays = rent_days = 33
			Status:      models.OrderStatusReturned,
			DeliveredAt: &delivered, ReturnedAt: &returned,
			Deposit: 0,
			// 无码全价：合同 28×3600=100800 + 续费 (28,33] 2×3600+3×3600×0.95=17460
			CashPaid: cents(1008.00 + 174.60), ShippingFee: 0,
			PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":33,"deposit":0,"total_amount":118260,
				"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0},{"days_max":180,"daily_rate":3600,"discount_percent":5}],
				"tier_segments":[{"tier":1,"days":30,"rate":3600,"discount":1,"subtotal":108000},
				                {"tier":2,"days":3,"rate":3600,"discount":0.95,"subtotal":10260}]}`),
		}
		require.NoError(t, db.Create(&o).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, UserID: userID, OrderID: &o.ID,
			OrderType: "renewal", Type: "payment", Status: "paid",
			Amount: cents(174.60), Days: &days5,
		}).Error)
		res := computeSettlement(o, db)
		require.True(t, res.SegmentModel, "rent_days(33)−续费(5)=28 应启用段模型")
		paidBlock := res.Breakdown["paid_block"].(map[string]interface{})
		contractRent := paidBlock["contract_rent"].(map[string]interface{})
		contractTiers := contractRent["tiers"].([]map[string]interface{})
		require.Len(t, contractTiers, 1, "合同 28 天全在 tier1（30 天边界内）")
		require.Equal(t, 28, contractTiers[0]["days"])
		renewals := paidBlock["renewals"].([]map[string]interface{})
		require.Len(t, renewals, 1)
		renewalTiers := renewals[0]["tiers"].([]map[string]interface{})
		// 判别断言：offset=28 续接 → 跨边界拆 2 段（tier1 2 天 + tier2 3 天）。
		// 错误实现（renewalOffset=initialDays=33）→ (33,38] 整段 tier2 5 天 1 段 → 红。
		require.Len(t, renewalTiers, 2, "续费跨 30 天边界必须拆为 tier1+tier2 两段")
		require.Equal(t, 1, renewalTiers[0]["tier"])
		require.Equal(t, 2, renewalTiers[0]["days"])
		require.Equal(t, int64(7200), renewalTiers[0]["subtotal"], "2 天全价 3600/天")
		require.Equal(t, 2, renewalTiers[1]["tier"])
		require.Equal(t, 3, renewalTiers[1]["days"])
		require.Equal(t, int64(10260), renewalTiers[1]["subtotal"], "3 天 × 3600 × 0.95")
		// 无码续费：原价 subtotal(17460) = 实付(17460) → discount_amount 必须 absent（无伪优惠抵扣）
		if d, exists := renewals[0]["discount_amount"]; exists {
			require.Equal(t, int64(0), d, "无码跨阶续费不得出现伪 discount_amount")
		}
		// 段模型应收 = 实付 → 无补退（无码平账，仅租金）
		require.InDelta(t, 0, res.PayableShortfall, 1e-9)
		require.InDelta(t, 0, res.TotalRefund, 1e-9)
	})
}

// TestPayableTiers_TruncatedByActualDays (#1850): 16fdfb81 形状 — returning、
// 实租 1 天（当天交付当天申请归还）、pb rent_days=2（合同 1 + 续费 1）。
// payable_block.actual_rent.tiers 必须按 actualDays 截断（1 天 ¥36.00），
// 与 amount 一致；修复前全租期展开 2 天 ¥72.00 与 amount ¥36.00 自相矛盾。
func TestPayableTiers_TruncatedByActualDays(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	cents := func(yuan float64) models.Cents { return models.Cents(int64(yuan*100 + 0.5)) }
	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-TIER-TRUNC", StockStatus: "rented",
	}).Error)
	delivered := time.Date(2026, 9, 8, 16, 56, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 8, 20, 34, 0, 0, time.UTC) // 同日 → 实租 1 天
	days1 := 1
	o := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
		StartDate: str1743Ptr("2026-09-08"), EndDate: str1743Ptr("2026-09-08"),
		LeaseTerm:   0, // 月语义：日租单 0
		Status:      models.OrderStatusReturning,
		DeliveredAt: &delivered, ReturnedAt: &returned,
		Deposit: 0, CashPaid: cents(0.72), ShippingFee: cents(0.01),
		CouponDiscount: cents(35.64),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
			"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
	}
	require.NoError(t, db.Create(&o).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		TenantID: tenantID, UserID: userID, OrderID: &o.ID,
		OrderType: "renewal", Type: "payment", Status: "paid",
		Amount: cents(0.36), Days: &days1,
	}).Error)

	res := computeSettlement(o, db)
	require.True(t, res.SegmentModel, "rent_days(2)−续费(1)=1 应启用段模型")

	payable := res.Breakdown["payable_block"].(map[string]interface{})
	actualRent := payable["actual_rent"].(map[string]interface{})
	require.Equal(t, int64(3600), actualRent["amount"], "1 天原价 36.00")
	require.Equal(t, 1, actualRent["days"])
	tiers := actualRent["tiers"].([]map[string]interface{})
	require.Len(t, tiers, 1, "全租期 2 天须截断为实租 1 天")
	require.Equal(t, 1, tiers[0]["tier"])
	require.Equal(t, 1, tiers[0]["days"])
	require.Equal(t, int64(3600), tiers[0]["subtotal"], "截断后 Σtiers == amount")
	// 折后口径顶层字段（fee_detail 透传的数据源）
	require.Equal(t, true, res.Breakdown["segment_model"])
	require.Equal(t, int64(37), res.Breakdown["discounted_due"], "折后 0.36 + 物流 0.01 = 0.37")
	require.InDelta(t, 0.35, res.TotalRefund, 1e-9, "0.72 − 0.37 = 0.35 应退")
}

// TestPayableTiers_FullTerm_NoOp (#1850 用例 C): actualDays=全租期时截断为
// no-op——tiers 输出与旧行为一致（全段完整展开）。
func TestPayableTiers_FullTerm_NoOp(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	cents := func(yuan float64) models.Cents { return models.Cents(int64(yuan*100 + 0.5)) }
	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-TIER-FULL", StockStatus: "rented",
	}).Error)
	delivered := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC) // 实租 2 天 = 全租期
	o := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
		StartDate: str1743Ptr("2026-09-01"), EndDate: str1743Ptr("2026-09-02"),
		LeaseTerm: 0, Status: models.OrderStatusReturned,
		DeliveredAt: &delivered, ReturnedAt: &returned,
		Deposit: 0, CashPaid: cents(36.00), ShippingFee: 0,
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1800,"rent_days":2,"deposit":0,"total_amount":3600,
			"pricing_tiers":[{"days_max":30,"daily_rate":1800,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":1800,"discount":1,"subtotal":3600}]}`),
	}
	require.NoError(t, db.Create(&o).Error)

	res := computeSettlement(o, db)
	payable := res.Breakdown["payable_block"].(map[string]interface{})
	actualRent := payable["actual_rent"].(map[string]interface{})
	tiers := actualRent["tiers"].([]map[string]interface{})
	require.Len(t, tiers, 1, "全租期展开不变")
	require.Equal(t, 2, tiers[0]["days"])
	require.Equal(t, int64(3600), tiers[0]["subtotal"])
	require.Equal(t, int64(3600), actualRent["amount"])
}

// TestSettlement_PostDamageStates (#1852 B1): 归还后未结算态（pending_damage_response/
// damage_appealing/deposit_refunding）租期已定格——ReturnedAt 生效（不再回退
// end_date），段模型必须启用。16fdfb81 形状：实租 1 天、优惠码两段 0.36。
// 修复前该态回退（历史错值）end_date → actualDays 虚增 → 段模型拒 → fallback。
func TestSettlement_PostDamageStates(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	cents := func(yuan float64) models.Cents { return models.Cents(int64(yuan*100 + 0.5)) }
	for _, status := range []string{
		models.OrderStatusPendingDamageResponse,
		models.OrderStatusDamageAppealing,
		models.OrderStatusDepositRefunding,
	} {
		t.Run(status, func(t *testing.T) {
			tenantID := uuid.New().String()
			orgID := tenantID
			userID := uuid.New().String()
			instID := uuid.New().String()
			require.NoError(t, db.Create(&models.Instrument{
				ID: instID, TenantID: tenantID, OrgID: &orgID,
				SN: "SN-DMG-" + uuid.New().String()[:6], StockStatus: "rented",
			}).Error)
			delivered := time.Date(2026, 9, 8, 16, 56, 0, 0, time.UTC)
			returned := time.Date(2026, 9, 8, 20, 34, 0, 0, time.UTC) // 同日 → 实租 1 天
			days1 := 1
			o := models.Order{
				TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
				StartDate: str1743Ptr("2026-09-08"), EndDate: str1743Ptr("2026-10-09"), // 历史错值 end_date 不应影响
				LeaseTerm: 0, Status: status,
				DeliveredAt: &delivered, ReturnedAt: &returned,
				Deposit: 0, CashPaid: cents(0.72), ShippingFee: cents(0.01),
				CouponDiscount: cents(35.64),
				PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
					"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
					"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
			}
			require.NoError(t, db.Create(&o).Error)
			require.NoError(t, db.Create(&models.OrderPaymentRecord{
				TenantID: tenantID, UserID: userID, OrderID: &o.ID,
				OrderType: "renewal", Type: "payment", Status: "paid",
				Amount: cents(0.36), Days: &days1,
			}).Error)
			damage := models.Cents(100) // 定损赔偿 ¥1
			require.NoError(t, db.Create(&models.DamageReport{
				TenantID: tenantID, OrgID: orgID, LeaseID: o.ID, InstrumentID: instID, UserID: userID,
				DamageAmount: &damage, DamageDescription: "刮痕", Status: "pending",
			}).Error)

			res := computeSettlement(o, db)
			require.True(t, res.SegmentModel, "归还后未结算态必须启用段模型（修复前 fallback）")
			payable := res.Breakdown["payable_block"].(map[string]interface{})
			actualRent := payable["actual_rent"].(map[string]interface{})
			require.Equal(t, 1, actualRent["days"], "租期定格：实租 1 天")
			require.Equal(t, int64(3600), actualRent["amount"])

			if status == models.OrderStatusDepositRefunding {
				// agreed 后：deposit_deducted=damage（appeal 裁决落库值）→ 扣赔偿
				require.NoError(t, db.Model(&models.DamageReport{}).
					Where("lease_id = ?", o.ID).Update("deposit_deducted", int64(100)).
					Update("status", "agreed").Error)
				res2 := computeSettlement(o, db)
				require.InDelta(t, 0.65, res2.PayableShortfall, 1e-9, "0.37+1.00−0.72 = 0.65 补缴（裁决值）")
			} else {
				// pending/appealing：预扣 damage_amount → 净补缴 0.65
				require.InDelta(t, 0.65, res.PayableShortfall, 1e-9, "预扣赔偿：0.37+1.00−0.72 = 0.65")
			}
		})
	}
}
