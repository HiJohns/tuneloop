package handlers

// #2406：确认页（pricing-v2 读路径）与下单校验（resolveDepositTier 写路径）必须同源。
// 口径（Owner 2026-11-17 裁定）：
//   - instruments.deposit > 0   → 显式配置，优先；
//   - instruments.deposit = 0   → 未配置，押金取定价结果（可能是 ¥0.01 这种真实小额）；
//   - 只有定价押金 = 0 才是「零押金档」。
// 缺陷版（pricingResponseWithInstrument 把 0 也当权威）会让页面显示「零押金」而
// 下单按定价押金判非零 → 400「zero deposit tier ...」。
import (
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cents(v float64) *models.Cents {
	c := models.FromYuan(v)
	return &c
}

// R1：instrument.deposit=0（未配置）+ 定价押金 ¥0.01 → 响应必须是 1 分（不是 0）。
func TestPricingResponseDeposit_InstrumentZeroFallsBackToPricing_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(0)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 0.01}, inst)

	got, ok := data["deposit"].(float64)
	require.True(t, ok, "deposit must be numeric, got %T", data["deposit"])
	assert.Equal(t, 1.0, got, "pricing deposit 0.01 元 must surface as 1 cent, not 0")
	assert.NotZero(t, got, "0 would make the confirm page claim 零押金 and the order fail")
}

// R2：instrument.deposit 显式 >0 → 以乐器配置为准（覆盖定价）。
func TestPricingResponseDeposit_InstrumentOverride_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(500)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 0.01}, inst)

	got, ok := data["deposit"].(int64)
	require.True(t, ok, "explicit instrument deposit must be int64 cents, got %T", data["deposit"])
	assert.EqualValues(t, 50000, got, "500 元 must surface as 50000 cents")
}

// R3：定价押金 0 且乐器未配置 → 真零押金（zero 档唯一合法场景）。
func TestPricingResponseDeposit_TrueZero_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(0)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 0}, inst)

	assert.Equal(t, 0.0, data["deposit"], "zero pricing deposit stays zero (zero tier allowed)")
}

// R4：deposit_reduced 仅在 >0 时下发，0 表示未配置 → null（前端据此隐藏差额档）。
func TestPricingResponseDeposit_ReducedZeroIsNull_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(500), DepositReduced: cents(0)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 500}, inst)
	assert.Nil(t, data["deposit_reduced"], "deposit_reduced=0 means unconfigured -> null")

	inst.DepositReduced = cents(200)
	data = pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 500}, inst)
	assert.EqualValues(t, 20000, data["deposit_reduced"], "200 元 must surface as 20000 cents")
}
