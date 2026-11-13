package services

import "fmt"

// 微信支付分信用状态（#2327 S2）。空字符串表示该订单未走支付分通道。
const (
	CreditStateNone     = ""
	CreditStatePending  = "pending"
	CreditStateApproved = "approved"
	CreditStateRejected = "rejected"
	CreditStateExpired  = "expired"
)

// 信用状态机事件。
const (
	CreditEventStart   = "start"
	CreditEventApprove = "approve"
	CreditEventReject  = "reject"
	CreditEventExpire  = "expire"
)

// creditTransitions 定义合法迁移；同态迁移用于幂等（重复回调/重复评估不报错）。
var creditTransitions = map[string]map[string]string{
	CreditStateNone: {
		CreditEventStart: CreditStatePending,
	},
	CreditStatePending: {
		CreditEventStart:   CreditStatePending,
		CreditEventApprove: CreditStateApproved,
		CreditEventReject:  CreditStateRejected,
		CreditEventExpire:  CreditStateExpired,
	},
	CreditStateApproved: {
		CreditEventApprove: CreditStateApproved,
	},
	CreditStateRejected: {
		CreditEventReject: CreditStateRejected,
	},
	CreditStateExpired: {
		CreditEventExpire: CreditStateExpired,
	},
}

// NextCreditState 计算下一个信用状态。
//
// 约束：
//   - 未开始（空）只能通过 start 进入 pending；
//   - pending 可 approve/reject/expire；
//   - approved/rejected/expired 为终态，只接受同义事件的幂等重复（重复回调不报错）；
//   - 其他组合一律返回错误（调用方不得静默放行）。
func NextCreditState(current, event string) (string, error) {
	if event == "" {
		return "", fmt.Errorf("credit state: empty event (current=%q)", current)
	}
	byEvent, ok := creditTransitions[current]
	if !ok {
		return "", fmt.Errorf("credit state: unknown current state %q", current)
	}
	next, ok := byEvent[event]
	if !ok {
		return "", fmt.Errorf("credit state: illegal transition %q + %q", current, event)
	}
	return next, nil
}

// IsCreditSettled 判断状态是否已终态（可用于"未 approved 不得支付 reduced/zero"的前置判断）。
func IsCreditSettled(state string) bool {
	switch state {
	case CreditStateApproved, CreditStateRejected, CreditStateExpired:
		return true
	default:
		return false
	}
}
