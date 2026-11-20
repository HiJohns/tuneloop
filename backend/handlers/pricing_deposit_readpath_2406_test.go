package handlers

// #2406：确认页（pricing-v2 读路径）与下单校验（resolveDepositTier 写路径）必须同源。
// 口径（Owner 2026-11-17 裁定）：
//   紧急修改：instruments.deposit 非 nil 即权威（0=显式零押金）；NULL 才回落定价。
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

// R1：instrument.deposit=0 且 pricing 无 deposit（未配置）→ 回落定价 1 分。
func TestPricingResponseDeposit_InstrumentZeroFallsBackToPricing_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(0)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 0.01}, inst)

	got, ok := data["deposit"].(float64)
	require.True(t, ok, "deposit must be numeric, got %T", data["deposit"])
	assert.Equal(t, 1.0, got, "unconfigured 0 falls back to pricing")
}

// R1b：显式零押金（pricing.deposit=0）→ 响应 0。
func TestPricingResponseDeposit_ExplicitZero_2437urgent(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(0), Pricing: `{"deposit":0}`}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 0.01}, inst)
	assert.EqualValues(t, int64(0), data["deposit"], "explicit zero overrides pricing")
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

	assert.EqualValues(t, int64(0), data["deposit"], "explicit zero deposit stays zero")
}

// R4：deposit_reduced 非 nil 即下发（0=显式零差额档）。
func TestPricingResponseDeposit_ReducedZeroIsNull_2406(t *testing.T) {
	inst := &models.Instrument{Deposit: cents(500), DepositReduced: cents(0)}
	data := pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 500}, inst)
	assert.EqualValues(t, 0, data["deposit_reduced"], "explicit reduced 0 must surface")

	inst.DepositReduced = cents(200)
	data = pricingResponseWithInstrument(&services.InstrumentPricing{Deposit: 500}, inst)
	assert.EqualValues(t, 20000, data["deposit_reduced"], "200 元 must surface as 20000 cents")
}
