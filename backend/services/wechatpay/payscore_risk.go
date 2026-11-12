package wechatpay

import "fmt"

// 押金档位取值与 backend/models 的 DepositTier* 保持一致（#2321）。
// 不直接 import models，避免服务层反向依赖数据层；一致性由 payscore_risk_test.go 断言。
const (
	TierFull    = "full"
	TierReduced = "reduced"
	TierZero    = "zero"
)

// RiskFundName 是支付分「免押租借」场景下风险金额的类型标识。
const RiskFundName = "ESTIMATE_ORDER_COST"

// ComputeRiskAmount 计算提交给微信支付分的订单风险金额（risk_fund.amount，单位：分）。
//
// 口径（#2327 计划 D2，联调期以微信实际口径为准）：
//   - full    全押金档：无需信用评估，风险金额 0
//   - reduced 差额押金档：减免额度 = 全额押金 − 差额押金
//   - zero    零押金档：减免额度 = 乐器原价
//
// 所有金额单位为分，禁止传负数；reduced 档必须满足 0 <= reduced <= full。
func ComputeRiskAmount(tier string, fullDepositCents, reducedDepositCents, totalPriceCents int64) (int64, error) {
	if fullDepositCents < 0 || reducedDepositCents < 0 || totalPriceCents < 0 {
		return 0, fmt.Errorf("payscore risk: negative amount (full=%d reduced=%d price=%d)",
			fullDepositCents, reducedDepositCents, totalPriceCents)
	}

	switch tier {
	case TierFull:
		return 0, nil
	case TierReduced:
		if reducedDepositCents > fullDepositCents {
			return 0, fmt.Errorf("payscore risk: reduced deposit %d exceeds full deposit %d",
				reducedDepositCents, fullDepositCents)
		}
		return fullDepositCents - reducedDepositCents, nil
	case TierZero:
		if totalPriceCents == 0 {
			return 0, fmt.Errorf("payscore risk: zero tier requires a positive instrument price")
		}
		return totalPriceCents, nil
	default:
		return 0, fmt.Errorf("payscore risk: unsupported deposit tier %q", tier)
	}
}
