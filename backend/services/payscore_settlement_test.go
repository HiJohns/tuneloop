package services

import (
	"testing"

	"tuneloop-backend/models"
)

func TestResolveSettlementAction(t *testing.T) {
	cases := []struct {
		name       string
		tier       string
		path       string
		credit     string
		cash       int64
		deduct     int64
		completed  bool
		wantCash   int64
		wantAction string
		wantDeduct int64
	}{
		{"full 档无扣款", "full", "", "", 500000, 0, true, 500000, PayscoreActionNone, 0},
		{"full 档扣款", "full", "", "", 500000, 120000, true, 380000, PayscoreActionNone, 0},
		{"full 档扣款超额封顶 0", "full", "", "", 100000, 300000, true, 0, PayscoreActionNone, 200000},
		{"reduced 已通过且履约完成", "reduced", models.DepositCreditPathPayscore, "approved", 200000, 50000, true, 150000, PayscoreActionComplete, 0},
		{"reduced 已通过但取消", "reduced", models.DepositCreditPathPayscore, "approved", 200000, 0, false, 200000, PayscoreActionCancel, 0},
		{"zero 无现金押金", "zero", models.DepositCreditPathPayscore, "approved", 0, 0, true, 0, PayscoreActionComplete, 0},
		{"zero 定损扣款不得为负退款", "zero", models.DepositCreditPathPayscore, "approved", 0, 50000, true, 0, PayscoreActionComplete, 50000},
		{"未走支付分则无动作", "reduced", models.DepositCreditPathPayscore, "", 200000, 0, true, 200000, PayscoreActionNone, 0},
		// 乐器本身零押金（internal/free）：不得被当成支付分订单收口
		{"free 零押金无支付分动作", "zero", "", "", 0, 0, true, 0, PayscoreActionNone, 0},
		// 负例：internal 路径的 reduced 即使有信用状态也不应走支付分收口（未走支付分通道）
		{"internal reduced 不走支付分", "reduced", models.DepositCreditPathInternal, "approved", 200000, 0, true, 200000, PayscoreActionNone, 0},
	}
	for _, tc := range cases {
		got := ResolveSettlementAction(tc.tier, tc.path, tc.credit, tc.cash, tc.deduct, tc.completed)
		if got.CashRefundCents != tc.wantCash {
			t.Fatalf("%s: cash=%d want %d", tc.name, got.CashRefundCents, tc.wantCash)
		}
		if got.PayscoreAction != tc.wantAction {
			t.Fatalf("%s: action=%q want %q", tc.name, got.PayscoreAction, tc.wantAction)
		}
		if got.PayscoreDeductCents != tc.wantDeduct {
			t.Fatalf("%s: deduct=%d want %d", tc.name, got.PayscoreDeductCents, tc.wantDeduct)
		}
	}
	// 负例：支付分担保绝不产生现金退款通道
	if PayscoreRequiresCashRefund() {
		t.Fatal("payscore guarantee must never trigger a cash refund")
	}
}
