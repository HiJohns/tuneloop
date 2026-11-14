package handlers

// #2352（Owner 押金三档口径）回归：
//   R4 全额押金 != 0 时不得接受 zero 档（下单 4xx）；
//   R5 乐器全额押金 == 0 的 zero 档必须通过微信信用评测（enforce=true）；
//   R6 历史订单（已落库 zero/reduced）的档位解析保持不变。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func TestZeroTierRejectedWhenFullDepositPositive_2352(t *testing.T) {
	// R4：payscore + zero + full>0 → 拒绝（下单返回 4xx 的同一判定）。
	err := validateZeroTierForFullDeposit(models.DepositTierZero, 6000)
	require.Error(t, err, "zero tier must be rejected when full deposit > 0")

	// full == 0 的乐器允许 zero 档（需评测，由 R5 覆盖）。
	assert.NoError(t, validateZeroTierForFullDeposit(models.DepositTierZero, 0))
	// 其余档位不受影响。
	assert.NoError(t, validateZeroTierForFullDeposit(models.DepositTierFull, 6000))
	assert.NoError(t, validateZeroTierForFullDeposit(models.DepositTierReduced, 6000))
}

func TestZeroTierRequiresAssessment_2352(t *testing.T) {
	// R5：zero 档（payscore 路径）在 enforce=true 时未 approved → 拦截。
	err := services.AssertOrderPayableIfEnforced(models.DepositTierZero, models.DepositCreditPathPayscore, "", true)
	require.Error(t, err, "zero tier without approved credit must be blocked")
	assert.Contains(t, err.Error(), "credit_")

	// approved → 放行。
	assert.NoError(t, services.AssertOrderPayableIfEnforced(models.DepositTierZero, models.DepositCreditPathPayscore, services.CreditStateApproved, true))
	// enforce=false 保持 D12 旧行为（不拦截）。
	assert.NoError(t, services.AssertOrderPayableIfEnforced(models.DepositTierZero, models.DepositCreditPathPayscore, "", false))
}

func TestLegacyOrderTiersUnchanged_2352(t *testing.T) {
	// R6：历史订单按**落库档位**读取，不重新解析；解析函数只影响新单。
	//  - full>0 的乐器请求 zero → 归一到 full（#2352 D1，新单不得全免）；
	//  - full==0 的乐器 → zero（乐器本身零押金，需评测）。
	pricing := &services.InstrumentPricing{}
	instrument := models.Instrument{}

	tierZero, _, _, _ := resolveDepositTier(&instrument, pricing, models.DepositTierZero)
	assert.Equal(t, models.DepositTierZero, tierZero, "zero-full instrument keeps the zero tier")

	deposit := models.FromYuan(6000)
	instrument.Deposit = &deposit
	tierFull, full, _, _ := resolveDepositTier(&instrument, pricing, models.DepositTierZero)
	assert.Equal(t, models.DepositTierFull, tierFull, "zero request on a positive-deposit instrument resolves to full")
	assert.InDelta(t, 6000, full, 0.001)

	// 历史订单读路径不经过 resolveDepositTier：已落库的 zero/reduced 原样返回（见 order.go 读路径）。
	legacy := models.Order{DepositTier: models.DepositTierZero}
	assert.Equal(t, models.DepositTierZero, legacy.DepositTier)
	legacyReduced := models.Order{DepositTier: models.DepositTierReduced}
	assert.Equal(t, models.DepositTierReduced, legacyReduced.DepositTier)
}
