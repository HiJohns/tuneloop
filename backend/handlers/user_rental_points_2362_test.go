package handlers

// #2362 Rev.2（A1/B1/D4）：支付端乐币额度与足额收款。
// R1 快照整数比；R2 cap 与 SettlementCalculator 同源；R3 越界/负数拒绝；
// R4 乐币覆盖押金（首付不足）拒绝。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/services"
)

func TestPointsRatioSnapshot_2362(t *testing.T) {
	cases := []struct {
		in       float64
		num, den int64
	}{
		{0.3, 3, 10},
		{0.25, 1, 4},
		{0.5, 1, 2},
		{1, 1, 1},
		{0.335, 67, 200},
		{0, 0, 1},
	}
	for _, tc := range cases {
		num, den, ok := pointsRatioFromDecimal(tc.in)
		require.True(t, ok, "ratio %v must parse", tc.in)
		assert.EqualValues(t, tc.num, num, "ratio %v num", tc.in)
		assert.EqualValues(t, tc.den, den, "ratio %v den", tc.in)
	}

	// R1 断言：券后租金 100000 分 + 3/10 的 cap（整数运算）
	capPoints, err := services.CalcPointsCap(100000, 3, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 300, capPoints, "floor(100000*3/(100*10))")
}

func TestPointsCapParity_2362(t *testing.T) {
	// R2：券 30000、券后租金 250000、900 点；支付端与 Calculator 必须完全一致。
	calc := services.NewSettlementCalculator()
	res, err := calc.Calculate(services.SettlementInput{
		SettlementID: "s-2362", OrderID: "o-2362", SettlementVersion: 1,
		RatioNum: 3, RatioDen: 10, RatioSource: services.RatioSourceSnapshot,
		DailyRateCents: 10000,
		PricingTiers:   []services.PricingTier{{DaysMax: 30, DiscountPercent: 0}, {DaysMax: -1, DiscountPercent: 0}},
		ActualDays:     28, CouponDiscountCents: 30000,
		GiftPointsUsedStoredCents:    90000,
		CashPaidCents:                210000,
		RentCashPaidNetCents:         160000,
		DepositCashCollectedNetCents: 50000,
		DepositHeldCents:             50000,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 750, res.GiftPointsCap)

	payCap, err := services.CalcPointsCap(res.RAfterCouponCents, 3, 10)
	require.NoError(t, err)
	assert.Equal(t, res.GiftPointsCap, payCap, "payment cap must equal calculator cap")

	// 基数不含押金：若把押金 50000 计入，cap 会变成 900（错误值）。
	withDeposit, err := services.CalcPointsCap(res.RAfterCouponCents+50000, 3, 10)
	require.NoError(t, err)
	assert.NotEqual(t, int64(750), withDeposit, "deposit must not enter the cap base")
}

func TestPointsUsageLimits_2362(t *testing.T) {
	// R3：cap=750（250000*3/1000）；用满 75000 分通过，超 1 点拒绝。
	if _, err := validatePointsUsage(250000, 75000, 3, 10); err != nil {
		t.Fatalf("exact cap must pass: %v", err.Message)
	}
	_, over := validatePointsUsage(250000, 75100, 3, 10)
	require.NotNil(t, over)
	assert.EqualValues(t, 40002, over.Code)

	_, negative := validatePointsUsage(250000, -100, 3, 10)
	require.NotNil(t, negative)
	assert.EqualValues(t, 40002, negative.Code)

	_, frac := validatePointsUsage(250000, 75050, 3, 10)
	require.NotNil(t, frac, "fractional points must be rejected")
}

func TestInsufficientPointsPayment_2362(t *testing.T) {
	// R4：全额比例 1/1、券后租金 100000（=1000 点）、押金 50000；
	// 用 1200 点会侵入押金 → 首付不足；支付端必须在建单前拒绝（400，不落 orders）。
	// 注：ratio<=1 时 cap<=券后租金，因此该场景由 cap 校验拦下（而不是靠后续现金流）。
	_, err := validatePointsUsage(100000, 120000, 1, 1)
	require.NotNil(t, err, "points covering deposit must be rejected")
	assert.EqualValues(t, 40002, err.Code)
}
