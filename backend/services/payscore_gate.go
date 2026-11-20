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

// RequiresCreditApprovalForOrder 是「订单级」的信用要求判定（#2327 D4/D13 / #2418）。
//
// Owner 2026-11-15「三档一致：未 approved 一律不可支付」：
//   - full 档在 enforce 语义下同样要求 approved（#2418 F1，不再按路径豁免）；
//   - reduced 档必须 approved；
//   - zero 档仅当「订单押金为 0」（乐器本身零押金，D3）与信用无关；
//     信用免押（订单押金 > 0）必须 approved。
//
// #2418 F2：零押金判据是**订单押金金额**（depositCents），不是 `deposit_credit_path` 是否为空——
// 新单一律 `path=payscore`，路径判据会把「乐器本身零押金」误判为需要评测。
// creditPath 保留参数用于语义追溯，不参与判定。
func RequiresCreditApprovalForOrder(tier, creditPath string, depositCents int64) bool {
	switch tier {
	case models.DepositTierFull:
		return true
	case models.DepositTierZero:
		return depositCents > 0
	case models.DepositTierReduced:
		return true
	default:
		return false
	}
}

// AssertOrderPayable 订单级支付闸门：按档位 + 下单时的信用路径 + 订单押金判定。
func AssertOrderPayable(tier, creditPath, creditState string, depositCents int64) error {
	if !RequiresCreditApprovalForOrder(tier, creditPath, depositCents) {
		return nil
	}
	return assertApproved(creditState)
}

// AssertOrderPayableIfEnforced 是支付路径唯一使用的闸门组合（enforce + 订单级判定）。
func AssertOrderPayableIfEnforced(tier, creditPath, creditState string, depositCents int64, enforce bool) error {
	if !enforce {
		return nil
	}
	return AssertOrderPayable(tier, creditPath, creditState, depositCents)
}

func assertApproved(creditState string) error {
	switch creditState {
	case CreditStateApproved:
		return nil
	case CreditStatePending:
		return fmt.Errorf("credit_pending: 信用评估尚未完成，请先在支付页完成微信信用评估")
	case CreditStateRejected:
		// #2352 F2（Owner 裁定 2026-11-15）：全额档同样要求评估通过，
		// 因此不能再引导用户「改用全押金」。
		return fmt.Errorf("credit_rejected: 信用评估未通过，本订单无法继续支付，请联系客服协助处理")
	case CreditStateExpired:
		return fmt.Errorf("credit_expired: 信用评估已过期，请重新评估")
	default:
		return fmt.Errorf("credit_required: 该押金档位需要先完成微信信用评估")
	}
}
