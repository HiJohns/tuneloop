package handlers

// #2357（Owner 2026-11-15 改判）：押金档位决定回到确认订单页。
// 下单接口允许请求 full / reduced；zero 仅当乐器全额押金 = 0。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"tuneloop-backend/models"
)

func TestValidateRequestedTier_2357(t *testing.T) {
	// full 恒可
	assert.NoError(t, validateRequestedTier(models.DepositTierFull, 6000, 2000))
	assert.NoError(t, validateRequestedTier(models.DepositTierFull, 6000, 0))

	// reduced 需要配置第二档，且必须小于全额
	assert.NoError(t, validateRequestedTier(models.DepositTierReduced, 6000, 2000))
	assert.Error(t, validateRequestedTier(models.DepositTierReduced, 6000, 0))
	assert.Error(t, validateRequestedTier(models.DepositTierReduced, 6000, 6000))

	// zero 仅当全额押金 = 0（不得凭信用免掉全额押金）
	assert.NoError(t, validateRequestedTier(models.DepositTierZero, 0, 0))
	assert.Error(t, validateRequestedTier(models.DepositTierZero, 6000, 2000))
}
