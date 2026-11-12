package services

import "tuneloop-backend/models"

// 结算时对支付分订单的动作。
const (
	PayscoreActionNone     = ""
	PayscoreActionComplete = "complete" // 履约完成 → 完结支付分订单
	PayscoreActionCancel   = "cancel"   // 未履约/取消 → 取消支付分订单，释放风险金额
)

// SettlementAction 描述一单结算时的资金动作。
type SettlementAction struct {
	// CashRefundCents 现金押金应退回的金额（走原路退款）。
	CashRefundCents int64
	// PayscoreAction 支付分订单收口动作；空表示该单未使用支付分。
	PayscoreAction string
	// PayscoreDeductCents 超出「现金押金 + 已扣押金」部分、应由支付分完结收取的金额（分）。
	// 仅作结算留痕/后续真实联调使用：当前 provider 的 CompleteOrder 不携带金额，
	// 微信侧按订单风险金额（credit_risk_amount_cents）作为上限。
	PayscoreDeductCents int64
}

// ResolveSettlementAction 计算结算动作（#2327 S3b / D14）。
//
// 关键约束：**现金押金与支付分担保是两条独立通道**
//   - 现金部分：押金 − 定损扣款，最低 0；只有现金部分走原路退款。
//   - 支付分部分：只做 complete/cancel，**绝不发起现金退款**（否则会重复赔付）。
//   - 定损扣款只能从现金押金扣除；不足以覆盖时不产生负退款，
//     缺口由调用方记为 PayscoreDeductCents（≤ risk amount），不向支付分追加现金退款。
func ResolveSettlementAction(tier, creditPath, creditState string, cashDepositCents, deductionCents int64, serviceCompleted bool) SettlementAction {
	action := SettlementAction{}

	// 现金押金部分：任何档位都可能有现金（reduced 档是差额）。
	refund := cashDepositCents - deductionCents
	if refund < 0 {
		// 超出部分只能由支付分担保覆盖（零押金档尤其如此），不得形成负退款。
		action.PayscoreDeductCents = -refund
		refund = 0
	}
	action.CashRefundCents = refund

	// 支付分通道：仅「确实走支付分」的订单（路径 payscore / 历史订单已有信用状态）才有动作。
	// internal 路径即使残留 approved 快照也不得走支付分收口。
	usesPayscore := creditState != "" && creditPath != models.DepositCreditPathInternal
	if RequiresCreditApprovalForOrder(tier, creditPath) && usesPayscore {
		if serviceCompleted {
			action.PayscoreAction = PayscoreActionComplete
		} else {
			action.PayscoreAction = PayscoreActionCancel
			// 未履约不收取任何扣款。
			action.PayscoreDeductCents = 0
		}
	}
	return action
}

// PayscoreRequiresCashRefund 恒为 false：支付分担保不得触发现金退款。
// 保留该函数用于结算代码显式表达意图，并作为负例断言锚点。
func PayscoreRequiresCashRefund() bool { return false }
