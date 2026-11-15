package handlers

// #2352 F1/F2（2026-11-15 Owner 裁定）：
//   F1 评测页需要订单截止时间 → creditPayload 必须下发 payment_deadline / order_status；
//   F2 全额档同样要求评估通过 → 被拒文案不得再引导「改用全押金」。

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

func TestCreditPayloadExposesPaymentDeadline_2352(t *testing.T) {
	deadline := time.Date(2026, 11, 15, 12, 36, 30, 0, time.UTC)
	order := &models.Order{
		ID:                "b1894517-3706-4e96-b79b-731170b6b351",
		Status:            models.OrderStatusReserved,
		DepositTier:       models.DepositTierFull,
		DepositCreditPath: models.DepositCreditPathPayscore,
		CreditState:       services.CreditStatePending,
		PaymentDeadline:   &deadline,
	}
	h := &PayscoreHandler{provider: wechatpay.NewMockPayscoreProvider()}
	payload := h.creditPayload(order)

	assert.Equal(t, "2026-11-15T12:36:30Z", payload["payment_deadline"])
	assert.Equal(t, models.OrderStatusReserved, payload["order_status"])
	// 全额 payscore 新单也要求评估（F2 维持现状）
	assert.Equal(t, true, payload["requires_assessment"])

	// 无截止时间时字段存在且为 nil（前端据此隐藏倒计时，不误显示）
	payload2 := h.creditPayload(&models.Order{ID: "x"})
	assert.Nil(t, payload2["payment_deadline"])
}

func TestRejectedMessageDoesNotSuggestFullTier_2352(t *testing.T) {
	err := services.AssertOrderPayableIfEnforced(
		models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStateRejected, true)
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, "全押金")
	assert.NotContains(t, msg, "改用全额")
	assert.Contains(t, msg, "无法继续支付")

	// 全额 + pending：同样被拦（Owner 裁定：所有 payscore 单需 approved）
	require.Error(t, services.AssertOrderPayableIfEnforced(
		models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStatePending, true))
	// approved 放行；enforce=false 放行
	require.NoError(t, services.AssertOrderPayableIfEnforced(
		models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStateApproved, true))
	require.NoError(t, services.AssertOrderPayableIfEnforced(
		models.DepositTierFull, models.DepositCreditPathPayscore, services.CreditStateRejected, false))
}
