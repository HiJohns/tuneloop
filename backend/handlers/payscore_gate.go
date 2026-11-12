package handlers

import (
	"fmt"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// ResolveCreditRiskAmount 计算某订单需要提交给微信的风险金额（分）。
// 纯函数，不依赖 DB，便于单测；full 档返回 0（无需评估）。
func ResolveCreditRiskAmount(tier string, fullDeposit, reducedDeposit, instrumentPrice int64) (int64, error) {
	return wechatpay.ComputeRiskAmount(tier, fullDeposit, reducedDeposit, instrumentPrice)
}

// RequiresCreditAssessment 判断该档位是否需要微信信用评估。
func RequiresCreditAssessment(tier string) bool {
	return tier == models.DepositTierReduced || tier == models.DepositTierZero
}

// AssertTierPayable 支付闸门：reduced/zero 必须信用状态 approved，full 无需评估。
//
// 返回 nil 表示允许支付；否则返回可直接透传给前端的错误（含原因码）。
func AssertTierPayable(tier, creditState string) error {
	if !RequiresCreditAssessment(tier) {
		return nil
	}
	switch creditState {
	case services.CreditStateApproved:
		return nil
	case services.CreditStatePending:
		return fmt.Errorf("credit_pending: 信用评估尚未完成，请先在支付页完成微信信用评估")
	case services.CreditStateRejected:
		return fmt.Errorf("credit_rejected: 信用评估未通过，请改用全押金档位或更换乐器")
	case services.CreditStateExpired:
		return fmt.Errorf("credit_expired: 信用评估已过期，请重新评估")
	default:
		return fmt.Errorf("credit_required: 该押金档位需要先完成微信信用评估")
	}
}
