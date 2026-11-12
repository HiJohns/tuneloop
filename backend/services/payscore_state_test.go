package services

import "testing"

// TestCreditStateMachine 覆盖合法迁移 / 非法迁移 / 幂等（#2327 矩阵 R5）。
func TestCreditStateMachine(t *testing.T) {
	legal := []struct {
		current string
		event   string
		want    string
	}{
		{CreditStateNone, CreditEventStart, CreditStatePending},
		{CreditStatePending, CreditEventApprove, CreditStateApproved},
		{CreditStatePending, CreditEventReject, CreditStateRejected},
		{CreditStatePending, CreditEventExpire, CreditStateExpired},
		// 幂等：重复回调/重复评估
		{CreditStatePending, CreditEventStart, CreditStatePending},
		{CreditStateApproved, CreditEventApprove, CreditStateApproved},
		{CreditStateRejected, CreditEventReject, CreditStateRejected},
		{CreditStateExpired, CreditEventExpire, CreditStateExpired},
	}
	for _, tc := range legal {
		got, err := NextCreditState(tc.current, tc.event)
		if err != nil {
			t.Fatalf("%q+%q unexpected error: %v", tc.current, tc.event, err)
		}
		if got != tc.want {
			t.Fatalf("%q+%q = %q, want %q", tc.current, tc.event, got, tc.want)
		}
	}

	illegal := []struct {
		current string
		event   string
	}{
		{CreditStateNone, CreditEventApprove},    // 未 start 不能直接通过
		{CreditStateNone, CreditEventReject},     // 同上
		{CreditStateApproved, CreditEventReject}, // 终态不可翻转
		{CreditStateRejected, CreditEventApprove},
		{CreditStateExpired, CreditEventApprove},
		{CreditStatePending, ""},    // 空事件
		{"weird", CreditEventStart}, // 未知状态
	}
	for _, tc := range illegal {
		if next, err := NextCreditState(tc.current, tc.event); err == nil {
			t.Fatalf("%q+%q must fail, got %q", tc.current, tc.event, next)
		}
	}
}

func TestIsCreditSettled(t *testing.T) {
	settled := []string{CreditStateApproved, CreditStateRejected, CreditStateExpired}
	for _, s := range settled {
		if !IsCreditSettled(s) {
			t.Fatalf("%q should be settled", s)
		}
	}
	for _, s := range []string{CreditStateNone, CreditStatePending} {
		if IsCreditSettled(s) {
			t.Fatalf("%q should not be settled", s)
		}
	}
}
