package services

import (
	"fmt"

	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
)

// ResolveCreditRiskAmount 计算某订单需要提交给微信的风险金额（分）。
// 纯函数，不依赖 DB；full 档返回 0（无需评估）。
func ResolveCreditRiskAmount(tier string, fullDeposit, reducedDeposit, instrumentPrice int64) (int64, error) {
	return wechatpay.ComputeRiskAmount(tier, fullDeposit, reducedDeposit, instrumentPrice)
}

// RequiresCreditAssessment 判断该档位是否需要微信信用评估。
func RequiresCreditAssessment(tier string) bool {
	return tier == models.DepositTierReduced || tier == models.DepositTierZero
}

// RequiresCreditApprovalForOrder 是「订单级」的信用要求判定（#2327 D4/D13）。
//
// zero 档存在两种来源，必须区分：
//   - 乐器本身零押金（full<=0）：与信用无关，不得因 enforce 而阻断支付；
//   - payscore 免押金路径（deposit_credit_path=payscore）：押金由支付分担保，
//     必须 approved。
//
// reduced 档恒需信用（差额押金靠支付分补足）；full 档无需评估。
func RequiresCreditApprovalForOrder(tier, creditPath string) bool {
	switch tier {
	case models.DepositTierReduced:
		return true
	case models.DepositTierZero:
		return creditPath == models.DepositCreditPathPayscore
	default:
		return false
	}
}

// AssertOrderPayable 订单级支付闸门：按档位 + 下单时的信用路径判定。
func AssertOrderPayable(tier, creditPath, creditState string) error {
	if !RequiresCreditApprovalForOrder(tier, creditPath) {
		return nil
	}
	return assertApproved(creditState)
}

// AssertTierPayable 支付闸门：reduced/zero 必须信用状态 approved，full 无需评估。
//
// 返回 nil 表示允许支付；否则返回可直接透传给前端的错误（含原因码）。
func AssertTierPayable(tier, creditState string) error {
	if !RequiresCreditAssessment(tier) {
		return nil
	}
	return assertApproved(creditState)
}

// AssertTierPayableIfEnforced 在 PAYSCORE_ENFORCE=false 时保持现有行为（不拦截）。
func AssertTierPayableIfEnforced(tier, creditState string, enforce bool) error {
	if !enforce {
		return nil
	}
	return AssertTierPayable(tier, creditState)
}

// AssertOrderPayableIfEnforced 是支付路径唯一使用的闸门组合（enforce + 订单级判定）。
func AssertOrderPayableIfEnforced(tier, creditPath, creditState string, enforce bool) error {
	if !enforce {
		return nil
	}
	return AssertOrderPayable(tier, creditPath, creditState)
}

func assertApproved(creditState string) error {
	switch creditState {
	case CreditStateApproved:
		return nil
	case CreditStatePending:
		return fmt.Errorf("credit_pending: 信用评估尚未完成，请先在支付页完成微信信用评估")
	case CreditStateRejected:
		return fmt.Errorf("credit_rejected: 信用评估未通过，请改用全押金档位或更换乐器")
	case CreditStateExpired:
		return fmt.Errorf("credit_expired: 信用评估已过期，请重新评估")
	default:
		return fmt.Errorf("credit_required: 该押金档位需要先完成微信信用评估")
	}
}
