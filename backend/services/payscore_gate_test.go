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
		deposit int64
		enforce bool
		ok      bool
	}{
		{"enforce 关闭时 reduced 未评估也放行", "reduced", "payscore", "", 50000, false, true},
		{"enforce 关闭时 zero 未评估也放行", "zero", "payscore", "", 50000, false, true},
		{"enforce 开启 reduced 已通过", "reduced", "payscore", "approved", 50000, true, true},
		{"enforce 开启 reduced 未评估拒绝", "reduced", "payscore", "", 50000, true, false},
		{"enforce 开启 zero(payscore,押金>0) 未评估拒绝", "zero", "payscore", "", 50000, true, false},
		{"enforce 开启 zero(payscore,押金>0) 已通过", "zero", "payscore", "approved", 50000, true, true},
		// #2418 F2：乐器本身零押金（订单押金=0）与信用无关，不得被 enforce 误伤；判据不依赖路径。
		{"enforce 开启 乐器零押金(payscore) 放行", "zero", "payscore", "", 0, true, true},
		{"enforce 开启 乐器零押金(历史空路径) 放行", "zero", "", "", 0, true, true},
		{"enforce 开启 internal reduced 未评估拒绝", "reduced", "internal", "", 50000, true, false},
		// #2418 F1：full 档同样要求 approved（不再按路径豁免）。
		{"enforce 开启 full(payscore) 未评估拒绝", "full", "payscore", "", 50000, true, false},
		{"enforce 开启 full(internal) 未评估拒绝", "full", "internal", "", 50000, true, false},
		{"enforce 开启 full(payscore) 已通过", "full", "payscore", "approved", 50000, true, true},
	}
	for _, tc := range cases {
		err := AssertOrderPayableIfEnforced(tc.tier, tc.path, tc.state, tc.deposit, tc.enforce)
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

// #2418 R2：internal 全额档同样要求信用评测（Owner 三档一致口径）。
func TestAssertOrderPayableInternalFull_2418(t *testing.T) {
	if err := AssertOrderPayableIfEnforced("full", "internal", "", 50000, true); err == nil {
		t.Fatal("full + internal + 未评测 must be rejected")
	}
}

// #2418 R3：历史空路径形态（zero + path="" + 押金 0）与信用无关，必须放行（兼容历史订单）。
func TestAssertOrderPayableZeroDeposit_2418(t *testing.T) {
	if err := AssertOrderPayableIfEnforced("zero", "", "", 0, true); err != nil {
		t.Fatalf("legacy free zero-deposit instrument must be payable: %v", err)
	}
}

// #2418 R6（复审 F1）：full + payscore 未评测必须拒绝（Owner 三档一致）；已通过放行。
func TestAssertOrderPayableFullPayscore_2418(t *testing.T) {
	if err := AssertOrderPayableIfEnforced("full", "payscore", "", 50000, true); err == nil {
		t.Fatal("full + payscore + 未评测 must be rejected")
	}
	if err := AssertOrderPayableIfEnforced("full", "payscore", "approved", 50000, true); err != nil {
		t.Fatalf("full + payscore + approved must be payable: %v", err)
	}
}

// #2418 R7（复审 F2）：zero + payscore + 订单押金=0（乐器本身零押金）必须放行。
func TestAssertOrderPayableZeroDepositPayscore_2418(t *testing.T) {
	if err := AssertOrderPayableIfEnforced("zero", "payscore", "", 0, true); err != nil {
		t.Fatalf("production zero-deposit instrument (payscore path) must be payable: %v", err)
	}
	// 对照：信用免押（押金>0）仍必须评测。
	if err := AssertOrderPayableIfEnforced("zero", "payscore", "", 50000, true); err == nil {
		t.Fatal("zero + payscore + 押金>0 + 未评测 must be rejected")
	}
}
