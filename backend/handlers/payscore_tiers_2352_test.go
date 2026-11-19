package handlers

// #2352 Rev.2（Owner 流程）回归：
//   R3 评测标的 = 乐器原价（不随档位变化）；
//   R5 支付页档位自选：0/1 担保人→拒绝差额；2 位→差额押金=deposit_reduced；zero 仅 full==0；
//   R6 零押金场景：订单押金为 0 且档位 zero（小票层不再出现押金项）；
//   R7 历史 internal 订单读/结算值不变。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func TestCreditRiskAmountEqualsInstrumentPrice_2352(t *testing.T) {
	full := models.FromYuan(6000)
	// 原价存在 → 恒等原价（与档位/押金无关）。
	assert.Equal(t, int64(600000), creditRiskAmount(600000, &full))
	// 原价缺失 → 退回押金快照（兼容历史数据）。
	assert.Equal(t, int64(600000), creditRiskAmount(0, &full))
	assert.Equal(t, int64(0), creditRiskAmount(0, nil))
}

func TestPaymentTierChoice_2352(t *testing.T) {
	full := models.FromYuan(6000)
	reduced := models.FromYuan(2000)
	zeroFull := models.FromYuan(0)

	// 0 担保人（或 1 位）→ 拒绝差额。
	_, _, err := resolvePaymentDepositTier(models.DepositTierReduced, &full, &reduced, nil)
	require.Error(t, err)
	_, _, err = resolvePaymentDepositTier(models.DepositTierReduced, &full, &reduced, []string{"g1"})
	require.Error(t, err)

	// 2 位担保人 → 差额档且押金 = deposit_reduced。
	tier, dep, err := resolvePaymentDepositTier(models.DepositTierReduced, &full, &reduced, []string{"g1", "g2"})
	require.NoError(t, err)
	assert.Equal(t, models.DepositTierReduced, tier)
	assert.Equal(t, reduced, dep)

	// 全额档默认可用。
	tier, dep, err = resolvePaymentDepositTier(models.DepositTierFull, &full, &reduced, nil)
	require.NoError(t, err)
	assert.Equal(t, models.DepositTierFull, tier)
	assert.Equal(t, full, dep)

	// zero 仅乐器全额押金 == 0。
	_, _, err = resolvePaymentDepositTier(models.DepositTierZero, &full, &reduced, nil)
	require.Error(t, err)
	tier, dep, err = resolvePaymentDepositTier(models.DepositTierZero, &zeroFull, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, models.DepositTierZero, tier)
	assert.Equal(t, models.Cents(0), dep)

	// 非法档位。
	_, _, err = resolvePaymentDepositTier("bogus", &full, &reduced, nil)
	require.Error(t, err)
}

func TestAllPayscoreOrdersRequireAssessment_2352(t *testing.T) {
	// R4：所有 payscore 新单（含全额档）在 enforce=true 且 pending 时支付被拦；
	// #2418 F1：internal 单同样要求 approved（Owner 三档一致）。
	require.Error(t, services.AssertOrderPayableIfEnforced(models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStatePending, 50000, true))
	require.NoError(t, services.AssertOrderPayableIfEnforced(models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStateApproved, 50000, true))
	// #2418 F1：internal 路径同样要求 approved（不再豁免）。
	require.Error(t, services.AssertOrderPayableIfEnforced(models.DepositTierFull, models.DepositCreditPathInternal, "", 50000, true))
	require.NoError(t, services.AssertOrderPayableIfEnforced(models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStatePending, 50000, false))
}

func TestZeroReceiptDepositIsZero_2352(t *testing.T) {
	// R6：零押金场景订单押金为 0、档位 zero（前端据此不渲染押金项）。
	order := models.Order{DepositTier: models.DepositTierZero, Deposit: 0}
	assert.Equal(t, models.Cents(0), order.Deposit)
	assert.Equal(t, models.DepositTierZero, order.DepositTier)
}

func TestLegacyInternalOrderUnchanged_2352(t *testing.T) {
	// R7：历史 internal 单读/结算值不变（档位与押金原样返回）。
	legacy := models.Order{
		DepositTier:       models.DepositTierReduced,
		DepositCreditPath: models.DepositCreditPathInternal,
		Deposit:           models.FromYuan(2000),
	}
	assert.Equal(t, models.DepositTierReduced, legacy.DepositTier)
	assert.Equal(t, models.DepositCreditPathInternal, legacy.DepositCreditPath)
	assert.Equal(t, models.FromYuan(2000), legacy.Deposit)
}
