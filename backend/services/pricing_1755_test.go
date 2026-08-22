package services

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// #1755 regression tests: FormatPricingResult outputs cents (P3 contract).
// Yuan inputs ×100 with math.Round (NO +0.5, #1763 lesson).

// TestFormatPricingResult_Cents (#1755): CV-73 日租 ¥45 → 4500 分,
// 押金 ¥30000 → 3000000 分; CV-08 押金 ¥0.01 → 1 分.
func TestFormatPricingResult_Cents(t *testing.T) {
	p := &InstrumentPricing{
		BaseDailyRate: 45.0,
		Tiers: []TierPrice{
			{DaysMax: 30, DailyRate: 45.0},
			{DaysMax: -1, DailyRate: 36.0},
		},
		Deposit:     30000.0,
		DepositMode: "base_daily_rate",
		ShippingFee: 20.0,
	}
	out := FormatPricingResult(p)

	require.Equal(t, 4500.0, out["base_daily_rate"], "¥45 → 4500 分")
	require.Equal(t, 3000000.0, out["deposit"], "¥30000 → 3000000 分")
	require.Equal(t, 2000.0, out["shipping_fee"], "¥20 → 2000 分")
	require.Equal(t, "base_daily_rate", out["deposit_mode"], "non-money field unchanged")

	tiers, ok := out["tiers"].([]map[string]interface{})
	require.True(t, ok, "tiers as array of maps")
	require.Len(t, tiers, 2)
	require.Equal(t, 4500.0, tiers[0]["daily_rate"], "tier 1 daily rate cents")
	require.Equal(t, 3600.0, tiers[1]["daily_rate"], "tier 2 daily rate cents")
	require.Equal(t, 30, tiers[0]["days_max"])
}

// TestFormatPricingResult_FractionalCents (#1755): ¥0.01 → exactly 1 分
// (no +0.5 drift, no float truncation).
func TestFormatPricingResult_FractionalCents(t *testing.T) {
	p := &InstrumentPricing{
		BaseDailyRate: 0.01,
		Deposit:       0.01,
		DepositMode:   "ratio",
	}
	out := FormatPricingResult(p)

	require.Equal(t, math.Round(0.01*100), out["base_daily_rate"], "¥0.01 → 1 分")
	require.Equal(t, 1.0, out["deposit"], "¥0.01 deposit → 1 分")
}
