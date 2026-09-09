package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1855 regression tests (OREZ 优惠码 damage 补缴死循环):
// ① computeSettlement：paid/waived 的 damage/shortfall 支付记录
//   Amount+CouponDiscount 必须冲减应付——否则 waived(amount=0,coupon=65) 的
//   补缴被视为「未收」→ damage 应付被重复追缴（16fdfb81: shortfall 65 死循环）。
// ② applySideEffects(damage) 全链：waived 支付 → completed 不回退 returning、
//   不生成新 payment_shortfall 记录。

// settle1855Order 构造 16fdfb81 形状订单：实租 1 天（同日收还）、优惠码两段
// 0.36（合同 1 天 + 续费 1 天，pb rent_days=2）、物流 0.01、已付 0.72；
// 定损报告 agreed（DepositDeducted=¥1.00）。supplementCoupon>0 时追加
// paid/waived 的 damage 补缴记录（amount=0 + coupon_discount=supplementCoupon）。
func settle1855Order(t *testing.T, db *gorm.DB, supplementCoupon int64) models.Order {
	t.Helper()
	cents := func(yuan float64) models.Cents { return models.Cents(int64(yuan*100 + 0.5)) }
	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-1855-" + uuid.New().String()[:6], StockStatus: "rented",
	}).Error)
	delivered := time.Date(2026, 9, 8, 16, 56, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 8, 20, 34, 0, 0, time.UTC) // 同日 → 实租 1 天
	days1 := 1
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
		StartDate: str1743Ptr("2026-09-08"), EndDate: str1743Ptr("2026-09-08"),
		LeaseTerm:   0,                           // 月语义：日租单 0
		Status:      models.OrderStatusCompleted, // damage waived 后 completed 再结算（applySideEffects 路径）
		DeliveredAt: &delivered, ReturnedAt: &returned,
		Deposit: 0, CashPaid: cents(0.72), ShippingFee: cents(0.01),
		CouponDiscount: cents(35.64),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
			"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		TenantID: tenantID, UserID: userID, OrderID: &order.ID,
		OrderType: "renewal", Type: "payment", Status: "paid",
		Amount: cents(0.36), Days: &days1,
	}).Error)

	// 定损 agreed：裁决落库 DepositDeducted=¥1.00（completed 态结算读取该值）
	damageAmt := models.Cents(100)
	require.NoError(t, db.Create(&models.DamageReport{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instID, UserID: userID,
		DamageAmount: &damageAmt, DepositDeducted: cents(1.00),
		Status: "agreed",
	}).Error)

	if supplementCoupon > 0 {
		// OREZ 全免补缴：amount=0 + coupon_discount（#1855 关键输入）
		orez := "OREZ"
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			TenantID: tenantID, OrgID: &orgID, UserID: userID, OrderID: &order.ID,
			OrderType: "damage", Type: "payment", Status: "paid",
			Method: strPtr("waived"), CouponCode: &orez,
			Amount: 0, CouponDiscount: models.Cents(supplementCoupon),
		}).Error)
	}
	return order
}

// TestComputeSettlement_SupplementWaive_CancelsShortfall (#1855 §四.2):
// 16fdfb81 形状 + paid/waived damage 记录（amount=0, coupon=65）→
// 折后应收 0.37 + 定损 1.00 − 已付(0.72+0.65) = 0 → payable_shortfall=0。
// 对照：同形状但 coupon_discount=0（优惠未入账 = 修复前）→ 0.65 补缴。
func TestComputeSettlement_SupplementWaive_CancelsShortfall(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// 修复后：优惠码减扣冲减应付 → 无补缴
	orderWith := settle1855Order(t, db, 65)
	resWith := computeSettlement(orderWith, db)
	require.True(t, resWith.SegmentModel)
	require.InDelta(t, 0.0, resWith.PayableShortfall, 1e-9,
		"waived coupon 65 计入已付 → 0.37+1.00−(0.72+0.65)=0，不得再生成补缴")
	require.InDelta(t, 0.0, resWith.TotalRefund, 1e-9)

	// 修复前行为锁定：同形状但 coupon 未入账（coupon_discount=0）→ 0.65 补缴
	orderWithout := settle1855Order(t, db, 0)
	resWithout := computeSettlement(orderWithout, db)
	require.InDelta(t, 0.65, resWithout.PayableShortfall, 1e-9,
		"discriminator：coupon 不入账时补缴 0.65（修复前死循环值）")
}

// TestApplySideEffects_DamageWaive_NoRollback (#1855 §四.1 E2E):
// damage 补缴支付（两种记账形态：OREZ waived amount=0+coupon=65 /
// jsapi 实付 amount=65）经 applySideEffects damage case → 订单 completed
// 且 executeRefund 不产生 shortfall 回退（completed→returning）、不生成新
// payment_shortfall 记录、settlement 落库 payable_shortfall=0 且 refund_status
// completed——16fdfb81 死循环的完整服务端链条复现。
func TestApplySideEffects_DamageWaive_NoRollback(t *testing.T) {
	for _, tc := range []struct {
		name string
		// 补缴记录记账形态：waived（OREZ，amount=0+coupon）/ jsapi（实付）
		waived    bool
		paidAmt   int64
		couponAmt int64
	}{
		{name: "OREZ_waived", waived: true, paidAmt: 0, couponAmt: 65},
		{name: "jsapi_paid", waived: false, paidAmt: 65, couponAmt: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testfixtures.SetupTestDB(t)
			order := settle1855Order(t, db, 65)
			// 真实链条：AgreeDamage 后订单保持待回应定损，支付回调
			// applySideEffects 先置 completed 再 executeRefund——从 pending 态起跑。
			require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
				Update("status", models.OrderStatusPendingDamageResponse).Error)

			// damage 补缴支付记录（prepay 产物：OREZ waive 分支 / jsapi 实付）
			record := models.OrderPaymentRecord{
				ID: uuid.New().String(), OrderID: &order.ID, OrderType: "damage",
				Amount: models.Cents(tc.paidAmt), Status: "paid",
			}
			if tc.waived {
				record.Method = strPtr("waived")
				record.CouponDiscount = models.Cents(tc.couponAmt)
			} else {
				record.Method = strPtr("jsapi")
			}
			require.NoError(t, applySideEffects(db, &record, time.Now()))

			// ① 订单 completed，不回退 returning
			var after models.Order
			require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
			require.Equal(t, models.OrderStatusCompleted, after.Status,
				"补缴后订单保持 completed，不得回退 returning")

			// ② 无新 payment_shortfall 记录
			var shortfallCount int64
			require.NoError(t, db.Model(&models.OrderPaymentRecord{}).
				Where("order_id = ? AND order_type = ? AND status = ?", order.ID, "payment_shortfall", "pending").
				Count(&shortfallCount).Error)
			require.Zero(t, shortfallCount, "不得生成重复补缴记录（死循环根因）")

			// ③ settlement 落库：payable_shortfall=0 + refund_status completed
			var settlement models.Settlement
			require.NoError(t, db.Where("order_id = ?", order.ID).First(&settlement).Error)
			require.Equal(t, "completed", settlement.RefundStatus, "零退款结算直接闭环")
			var bd struct {
				PayableShortfall float64 `json:"payable_shortfall"`
			}
			require.NoError(t, json.Unmarshal([]byte(settlement.Breakdown), &bd))
			require.InDelta(t, 0.0, bd.PayableShortfall, 1e-9, "落库 breakdown 无补缴残留")
		})
	}
}
