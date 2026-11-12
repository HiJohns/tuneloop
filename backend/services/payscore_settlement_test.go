package services

import "testing"

func TestResolveSettlementAction(t *testing.T) {
	cases := []struct {
		name       string
		tier       string
		credit     string
		cash       int64
		deduct     int64
		completed  bool
		wantCash   int64
		wantAction string
	}{
		{"full 档无扣款", "full", "", 500000, 0, true, 500000, PayscoreActionNone},
		{"full 档扣款", "full", "", 500000, 120000, true, 380000, PayscoreActionNone},
		{"full 档扣款超额封顶 0", "full", "", 100000, 300000, true, 0, PayscoreActionNone},
		{"reduced 已通过且履约完成", "reduced", "approved", 200000, 50000, true, 150000, PayscoreActionComplete},
		{"reduced 已通过但取消", "reduced", "approved", 200000, 0, false, 200000, PayscoreActionCancel},
		{"zero 无现金押金", "zero", "approved", 0, 0, true, 0, PayscoreActionComplete},
		{"zero 定损扣款不得为负", "zero", "approved", 0, 50000, true, 0, PayscoreActionComplete},
		{"未走支付分则无动作", "reduced", "", 200000, 0, true, 200000, PayscoreActionNone},
	}
	for _, tc := range cases {
		got := ResolveSettlementAction(tc.tier, tc.credit, tc.cash, tc.deduct, tc.completed)
		if got.CashRefundCents != tc.wantCash {
			t.Fatalf("%s: cash=%d want %d", tc.name, got.CashRefundCents, tc.wantCash)
		}
		if got.PayscoreAction != tc.wantAction {
			t.Fatalf("%s: action=%q want %q", tc.name, got.PayscoreAction, tc.wantAction)
		}
	}
	// 负例：支付分担保绝不产生现金退款通道
	if PayscoreRequiresCashRefund() {
		t.Fatal("payscore guarantee must never trigger a cash refund")
	}
}
