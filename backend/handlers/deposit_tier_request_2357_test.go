package handlers

// #2357（Owner 2026-11-15 改判）：押金档位决定回到确认订单页。
// 下单接口允许请求 full / reduced；zero 仅当乐器全额押金 = 0。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func TestValidateRequestedTier_2357(t *testing.T) {
	// full 恒可
	assert.NoError(t, validateRequestedTier(models.DepositTierFull, 6000, 2000))
	assert.NoError(t, validateRequestedTier(models.DepositTierFull, 6000, 0))

	// reduced 需要配置第二档；0 为显式零差额档，允许但必须小于全额
	assert.NoError(t, validateRequestedTier(models.DepositTierReduced, 6000, 2000))
	assert.NoError(t, validateRequestedTier(models.DepositTierReduced, 6000, 0))
	assert.Error(t, validateRequestedTier(models.DepositTierReduced, 6000, 6000))

	// zero 仅当全额押金 = 0（不得凭信用免掉全额押金）
	assert.NoError(t, validateRequestedTier(models.DepositTierZero, 0, 0))
	assert.Error(t, validateRequestedTier(models.DepositTierZero, 6000, 2000))
}

// #2357 修正：deposit_reduced = 0 表示「未配置」，不得当成零差额免押；
// 请求差额但配置无效时必须由调用方显式拒绝（禁止静默回落成全额定档）。
func TestResolveDepositTierRejectsZeroReduced_2357(t *testing.T) {
	fullOnly := models.FromYuan(6000)
	zeroReduced := models.FromYuan(0)
	okReduced := models.FromYuan(2000)
	eqReduced := models.FromYuan(6000)
	pricing := &services.InstrumentPricing{Deposit: 6000}

	inst := &models.Instrument{Deposit: &fullOnly, DepositReduced: &zeroReduced}
	tier, _, reduced, valid := resolveDepositTier(inst, pricing, models.DepositTierReduced)
	assert.Equal(t, models.DepositTierReduced, tier)
	assert.True(t, valid, "reduced=0 is an explicit zero reduced tier")
	assert.Zero(t, reduced)

	inst.DepositReduced = &eqReduced
	_, _, _, valid = resolveDepositTier(inst, pricing, models.DepositTierReduced)
	assert.False(t, valid, "reduced == full must be invalid")

	inst.DepositReduced = &okReduced
	tier, _, reduced, valid = resolveDepositTier(inst, pricing, models.DepositTierReduced)
	assert.True(t, valid)
	assert.Equal(t, models.DepositTierReduced, tier)
	assert.Equal(t, 2000.0, reduced)
}
