package services

import "testing"

func TestAssertTierPayable(t *testing.T) {
	cases := []struct {
		name  string
		tier  string
		state string
		ok    bool
	}{
		{"full 档无需评估", "full", "", true},
		{"full 档即使拒绝也可支付", "full", "rejected", true},
		{"reduced 已通过", "reduced", "approved", true},
		{"reduced 评估中拒绝", "reduced", "pending", false},
		{"reduced 未评估拒绝", "reduced", "", false},
		{"reduced 拒绝", "reduced", "rejected", false},
		{"reduced 过期", "reduced", "expired", false},
		{"zero 已通过", "zero", "approved", true},
		{"zero 未评估拒绝", "zero", "", false},
	}
	for _, tc := range cases {
		err := AssertTierPayable(tc.tier, tc.state)
		if tc.ok && err != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		}
	}
}

// TestAssertOrderPayableIfEnforced 覆盖 #2327 D4/D12/D13：
//   - enforce=false 时一律放行（线上行为不变）；
//   - zero 档要区分「乐器本身零押金」（internal/空路径）与「payscore 免押」。
func TestAssertOrderPayableIfEnforced(t *testing.T) {
	cases := []struct {
		name    string
		tier    string
		path    string
		state   string
		enforce bool
		ok      bool
	}{
		{"enforce 关闭时 reduced 未评估也放行", "reduced", "payscore", "", false, true},
		{"enforce 关闭时 zero 未评估也放行", "zero", "payscore", "", false, true},
		{"enforce 开启 reduced 已通过", "reduced", "payscore", "approved", true, true},
		{"enforce 开启 reduced 未评估拒绝", "reduced", "payscore", "", true, false},
		{"enforce 开启 zero(payscore) 未评估拒绝", "zero", "payscore", "", true, false},
		{"enforce 开启 zero(payscore) 已通过", "zero", "payscore", "approved", true, true},
		// 乐器本身零押金（full<=0）与信用无关，不得被 enforce 误伤。
		{"enforce 开启 free 零押金放行", "zero", "", "", true, true},
		{"enforce 开启 internal reduced 未评估拒绝", "reduced", "internal", "", true, false},
		{"full 档始终放行", "full", "payscore", "", true, true},
	}
	for _, tc := range cases {
		err := AssertOrderPayableIfEnforced(tc.tier, tc.path, tc.state, tc.enforce)
		if tc.ok && err != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		}
	}
}

func TestResolveCreditRiskAmount(t *testing.T) {
	// full=5000, reduced=2000, price=12000（分）
	cases := []struct {
		tier string
		want int64
	}{
		{"full", 0},
		{"reduced", 3000},
		{"zero", 12000},
	}
	for _, tc := range cases {
		got, err := ResolveCreditRiskAmount(tc.tier, 5000, 2000, 12000)
		if err != nil {
			t.Fatalf("%s: %v", tc.tier, err)
		}
		if got != tc.want {
			t.Fatalf("%s = %d, want %d", tc.tier, got, tc.want)
		}
	}
	// 负例：reduced > full
	if _, err := ResolveCreditRiskAmount("reduced", 2000, 5000, 12000); err == nil {
		t.Fatal("reduced>full must fail")
	}
}
