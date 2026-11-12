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
