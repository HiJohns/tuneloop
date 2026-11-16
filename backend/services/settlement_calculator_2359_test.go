package services

// #2359（#2268 子任务）：SettlementCalculator 纯计算验收。
// R1 T1–T10（分账户逐项 + 守恒式）；R2 T23（券在前）；R3 T24/T25/T26；
// R4 非法输入/溢出负例。

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseSettlementInput2359(days int, cash, rentPaid, deposit, pointsStored int64) SettlementInput {
	return SettlementInput{
		SettlementID:                 "settle-2359",
		OrderID:                      "order-2359",
		SettlementVersion:            1,
		RatioNum:                     3,
		RatioDen:                     10,
		RatioSource:                  RatioSourceSnapshot,
		PricingPolicyVersion:         "policy-v1",
		DailyRateCents:               10000, // ¥100/天
		PricingTiers:                 []PricingTier{{DaysMax: 30, DiscountPercent: 0}, {DaysMax: -1, DiscountPercent: 0}},
		ActualDays:                   days,
		GiftPointsUsedStoredCents:    pointsStored,
		CashPaidCents:                cash,
		RentCashPaidNetCents:         rentPaid,
		DepositCashCollectedNetCents: deposit,
		DepositHeldCents:             deposit,
	}
}

func mustCalculate2359(t *testing.T, in SettlementInput) *SettlementResult {
	t.Helper()
	res, err := NewSettlementCalculator().Calculate(in)
	require.NoError(t, err, "calculate")
	require.NotNil(t, res)
	require.Equal(t, "calculated", res.Status)
	require.Equal(t, AlgorithmVersionSettlement2359, res.AlgorithmVersion)
	return res
}

// R1：T1–T10 分账户逐项断言（禁止只断净额）。
func TestSettlementCalculatorCore_2359(t *testing.T) {
	tests := []struct {
		name       string
		in         SettlementInput
		rList      int64
		capPoints  int64
		pRefund    int64
		rentDue    int64
		rentRefund int64
		rentShort  int64
		depRefund  int64
		otherShort int64
		netCash    int64
	}{
		{"T1", baseSettlementInput2359(30, 260000, 210000, 50000, 90000), 300000, 900, 0, 210000, 0, 0, 50000, 0, 50000},
		{"T2", baseSettlementInput2359(28, 260000, 210000, 50000, 90000), 280000, 840, 60, 196000, 14000, 0, 50000, 0, 64000},
		{"T3", baseSettlementInput2359(28, 300000, 250000, 50000, 50000), 280000, 840, 0, 230000, 20000, 0, 50000, 0, 70000},
		{"T4", baseSettlementInput2359(28, 200000, 150000, 50000, 100000), 280000, 840, 160, 196000, 0, 46000, 50000, 0, 4000},
		{"T7", baseSettlementInput2359(0, 260000, 210000, 50000, 90000), 0, 0, 900, 0, 210000, 0, 50000, 0, 260000},
		{"T9", baseSettlementInput2359(28, 210000, 210000, 0, 90000), 280000, 840, 60, 196000, 14000, 0, 0, 0, 14000},
		{"T10", baseSettlementInput2359(28, 213000, 210000, 3000, 90000), 280000, 840, 60, 196000, 14000, 0, 3000, 0, 17000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := mustCalculate2359(t, tc.in)
			assert.EqualValues(t, tc.rList, res.RActualListCents, "R_actual_list")
			assert.EqualValues(t, tc.capPoints, res.GiftPointsCap, "cap_points")
			assert.EqualValues(t, tc.pRefund, res.GiftPointsRefunded, "p_refund")
			assert.EqualValues(t, tc.rentDue, res.RentCashDueCents, "rent_cash_due")
			assert.EqualValues(t, tc.rentRefund, res.RentRefundCents, "rent_refund")
			assert.EqualValues(t, tc.rentShort, res.RentShortfallCents, "rent_shortfall")
			assert.EqualValues(t, tc.depRefund, res.DepositRefundCents, "deposit_refund")
			assert.EqualValues(t, tc.otherShort, res.OtherFeeShortfallCents, "other_fee_shortfall")
			assert.EqualValues(t, tc.netCash, res.NetCashCents, "net_cash")
			assert.EqualValues(t, tc.rentRefund+tc.depRefund, res.CashRefundableCents, "cash_refundable")
			assert.EqualValues(t, tc.rentShort+tc.otherShort, res.CashPayableCents, "cash_payable")
		})
	}

	// T5/T6：额外费用分别落库 + 押金不足转补缴（分账户逐项）。
	t.Run("T5", func(t *testing.T) {
		in := baseSettlementInput2359(28, 260000, 210000, 50000, 90000)
		in.DamageFeeCents = 30000
		res := mustCalculate2359(t, in)
		assert.EqualValues(t, 14000, res.RentRefundCents)
		assert.EqualValues(t, 30000, res.DamageFeeCents, "damage fee stays classified")
		assert.EqualValues(t, 30000, res.DeductionsTotalCents)
		assert.EqualValues(t, 30000, res.DepositDeductionsCents)
		assert.EqualValues(t, 20000, res.DepositRefundCents)
		assert.EqualValues(t, 34000, res.NetCashCents)
	})
	t.Run("T6", func(t *testing.T) {
		in := baseSettlementInput2359(28, 260000, 210000, 50000, 90000)
		in.DamageFeeCents = 80000
		res := mustCalculate2359(t, in)
		assert.EqualValues(t, 14000, res.RentRefundCents)
		assert.EqualValues(t, 50000, res.DepositDeductionsCents, "deposit capped at held amount")
		assert.Zero(t, res.DepositRefundCents)
		assert.EqualValues(t, 30000, res.OtherFeeShortfallCents)
		assert.EqualValues(t, -16000, res.NetCashCents)
		assert.EqualValues(t, 14000, res.CashRefundableCents)
		assert.EqualValues(t, 30000, res.CashPayableCents)
	})

	// T8：续费显式夹具（不得用默认值推导）。
	t.Run("T8", func(t *testing.T) {
		in := baseSettlementInput2359(45, 560000, 510000, 50000, 90000)
		in.TermVersions = TermVersions{
			OriginalTermDays: 30, OriginalRentCents: 300000,
			RenewalTermDays: 30, RenewalRentCents: 300000,
			TotalContractDays: 60, TotalContractRentCents: 600000,
			DepositPolicy: "carry_over",
		}
		res := mustCalculate2359(t, in)
		assert.EqualValues(t, 450000, res.RActualListCents)
		assert.EqualValues(t, 1350, res.GiftPointsCap)
		assert.EqualValues(t, 360000, res.RentCashDueCents)
		assert.EqualValues(t, 150000, res.RentRefundCents)
		assert.EqualValues(t, 50000, res.DepositRefundCents)
		assert.EqualValues(t, 200000, res.NetCashCents)
		assert.EqualValues(t, 30, res.OriginalTermDays)
		assert.EqualValues(t, 300000, res.OriginalRentCents)
		assert.EqualValues(t, 30, res.RenewalTermDays)
		assert.EqualValues(t, 300000, res.RenewalRentCents)
		assert.EqualValues(t, 60, res.TotalContractDays)
		assert.EqualValues(t, 600000, res.TotalContractRentCents)
		assert.EqualValues(t, "carry_over", res.DepositPolicy)
	})
}

// R2：T23 券在前（cap 基数为券后租金）。
func TestSettlementCalculatorCoupon_2359(t *testing.T) {
	in := baseSettlementInput2359(28, 210000, 160000, 50000, 90000)
	in.CouponDiscountCents = 30000
	res := mustCalculate2359(t, in)

	assert.EqualValues(t, int64(280000), res.RActualListCents)
	assert.EqualValues(t, int64(30000), res.CouponDiscountCents)
	assert.EqualValues(t, int64(250000), res.RAfterCouponCents, "R_after_coupon")
	assert.EqualValues(t, int64(750), res.GiftPointsCap, "cap = floor(250000*3/(100*10))")
	assert.EqualValues(t, int64(900), res.GiftPointsUsed)
	assert.EqualValues(t, int64(750), res.GiftPointsConsumed)
	assert.EqualValues(t, int64(150), res.GiftPointsRefunded)
	assert.EqualValues(t, int64(175000), res.RentCashDueCents, "due = 250000 - 750*100")
	assert.EqualValues(t, int64(15000), res.RentShortfallCents)
	assert.EqualValues(t, int64(50000), res.DepositRefundCents)
	assert.EqualValues(t, int64(35000), res.NetCashCents)
	assert.EqualValues(t, int64(50000), res.CashRefundableCents)
	assert.EqualValues(t, int64(15000), res.CashPayableCents)
	assert.EqualValues(t, int64(210000), res.InitialCashCollectedCents, "Q8: 250000 - 90000 + 50000")
}

// R3：T24 争议隔离 / T25 版本唯一 / T26 租金现金来源。
func TestSettlementCalculatorDispute_2359(t *testing.T) {
	// T24：定损 30000 争议 + 逾期 10000 未争议 → 只扣 10000，押金退 40000。
	in := baseSettlementInput2359(30, 260000, 210000, 50000, 90000)
	in.DamageFeeCents = 30000
	in.OverduePenaltyCents = 10000
	in.DisputedAmountCents = 30000
	in.DisputeStatus = "appealing"
	in.DisputeReferenceID = "appeal-2359"
	res := mustCalculate2359(t, in)
	assert.EqualValues(t, int64(40000), res.DeductionsTotalCents)
	assert.EqualValues(t, int64(30000), res.DisputedAmountCents, "disputed stays isolated")
	assert.EqualValues(t, int64(10000), res.UndisputedDeductionsCents)
	assert.EqualValues(t, int64(10000), res.DepositDeductionsCents, "only undisputed hits deposit")
	assert.EqualValues(t, int64(40000), res.DepositRefundCents)
	assert.Zero(t, res.OtherFeeShortfallCents)
	assert.EqualValues(t, int64(40000), res.NetCashCents)
	assert.EqualValues(t, "appealing", res.DisputeStatus)
	assert.EqualValues(t, "appeal-2359", res.DisputeReferenceID)
	assert.EqualValues(t, int64(10000), res.OverduePenaltyCents)
	assert.EqualValues(t, int64(30000), res.DamageFeeCents)

	// T25：存在未冲正的最终结算时，新版本被拒；同版本重试允许。
	duplicate := baseSettlementInput2359(30, 260000, 210000, 50000, 90000)
	duplicate.ExistingActiveVersion = 1
	duplicate.SettlementVersion = 2
	_, err := NewSettlementCalculator().Calculate(duplicate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSettlementDuplicateActive), "must be ErrSettlementDuplicateActive: %v", err)

	retry := baseSettlementInput2359(30, 260000, 210000, 50000, 90000)
	retry.ExistingActiveVersion = 1
	retry.SettlementVersion = 1
	_, err = NewSettlementCalculator().Calculate(retry)
	require.NoError(t, err, "retrying the same version is allowed")

	// T26：押金被扣 30000 后，租金实收仍来自账本，不随 deposit_held 变化。
	before := mustCalculate2359(t, baseSettlementInput2359(28, 260000, 210000, 50000, 90000))
	// 已扣 30000 后：净现金实收 = 租金 210000 + 剩余押金 20000（守恒式口径）。
	afterIn := baseSettlementInput2359(28, 230000, 210000, 20000, 90000)
	afterIn.DepositHeldCents = 20000
	after := mustCalculate2359(t, afterIn)
	assert.EqualValues(t, before.RentCashPaidNetCents, after.RentCashPaidNetCents, "rent cash comes from ledger")
	assert.EqualValues(t, before.RentCashDueCents, after.RentCashDueCents, "rent due unaffected by deposit")
	assert.EqualValues(t, before.RentRefundCents, after.RentRefundCents, "rent refund unaffected by deposit")
	assert.EqualValues(t, int64(20000), after.DepositHeldCents)
	assert.EqualValues(t, int64(20000), after.DepositRefundCents)
}

// R4：非法输入/溢出返回明确错误，且不 panic。
func TestSettlementCalculatorErrors_2359(t *testing.T) {
	base := func() SettlementInput { return baseSettlementInput2359(28, 260000, 210000, 50000, 90000) }

	t.Run("ratio_den_zero", func(t *testing.T) {
		in := base()
		in.RatioDen = 0
		_, err := NewSettlementCalculator().Calculate(in)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("ratio_num_gt_den", func(t *testing.T) {
		in := base()
		in.RatioNum = 11
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("negative_days", func(t *testing.T) {
		in := base()
		in.ActualDays = -1
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("negative_deposit", func(t *testing.T) {
		in := base()
		in.DepositHeldCents = -1
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("coupon_exceeds_rent", func(t *testing.T) {
		in := base()
		in.CouponDiscountCents = 999999
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("disputed_exceeds_deductions", func(t *testing.T) {
		in := base()
		in.DamageFeeCents = 10000
		in.DisputedAmountCents = 20000
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvalidInput))
	})
	t.Run("daily_rate_overflow", func(t *testing.T) {
		in := base()
		in.DailyRateCents = math.MaxInt64 / 2
		in.ActualDays = 30
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementOverflow), "got: %v", err)
	})
	t.Run("ratio_den_overflow", func(t *testing.T) {
		in := base()
		in.RatioDen = math.MaxInt64 / 10
		in.RatioNum = in.RatioDen
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementOverflow), "got: %v", err)
	})
	t.Run("invariant_violation", func(t *testing.T) {
		// 人为提供不一致的 cash_paid（守恒式不成立）必须报错，而不是返回结果。
		in := base()
		in.CashPaidCents = 1
		_, err := NewSettlementCalculator().Calculate(in)
		assert.True(t, errors.Is(err, ErrSettlementInvariantViolation), "got: %v", err)
	})
	t.Run("unresolved_ratio_marker", func(t *testing.T) {
		in := base()
		in.RatioSource = RatioSourceUnresolved
		in.CashPaidCents = 260000 // 无点退款：p_refund=0，守恒式仍成立
		res := mustCalculate2359(t, in)
		assert.EqualValues(t, LegacyMarkerPolicyUnresolved, res.LegacyMarker)
		assert.Zero(t, res.GiftPointsCap)
		assert.EqualValues(t, 900, res.GiftPointsConsumed, "unresolved: used points are not refunded")
		assert.Zero(t, res.GiftPointsRefunded)
	})
}
