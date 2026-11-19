package services

import (
	"errors"
	"math"
	"math/rand"
	"testing"
)

// #2423（#2417 S1）：ComputeRefundSpec 单测。
// 期望值来自 docs/cases/settlement-refund-spec-v2.md §3/§4/§5（T23）与
// #2339 fixture（R0=3000/券后 2800、乐币 1000 点、押金 500、现金实收 2000）。

// T23：券 30000 分 + 乐币 900 点，28 天（R_list=280000 / R_after_coupon=250000）。
func TestComputeRefundSpecT23_2423(t *testing.T) {
	in := RefundSpecInput{
		RAfterCouponCents:            250000,
		PUsedPoints:                  900,
		RentCashPaidNetCents:         160000,
		DepositCashCollectedNetCents: 50000,
		DepositHeldCents:             50000,
		OtherFeeCents:                0,
		CashPaidNetCents:             210000, // 租金 160000 + 押金 50000
		RatioNum:                     3,
		RatioDen:                     10,
	}

	got, err := ComputeRefundSpec(in)
	if err != nil {
		t.Fatalf("ComputeRefundSpec(T23) error: %v", err)
	}

	if got.PCapPoints != 750 {
		t.Fatalf("T23 cap = %d, want 750 (按券后租金)", got.PCapPoints)
	}
	if got.PConsumedPoints != 750 || got.PRefundPoints != 150 {
		t.Fatalf("T23 consumed/refund = %d/%d, want 750/150", got.PConsumedPoints, got.PRefundPoints)
	}
	if got.RentCashDueCents != 175000 {
		t.Fatalf("T23 rent due = %d, want 175000", got.RentCashDueCents)
	}
	if got.RentShortfallCents != 15000 || got.RentRefundCents != 0 {
		t.Fatalf("T23 rent shortfall/refund = %d/%d, want 15000/0", got.RentShortfallCents, got.RentRefundCents)
	}
	if got.DepositRefundCents != 50000 {
		t.Fatalf("T23 deposit refund = %d, want 50000", got.DepositRefundCents)
	}
	if got.NetCashCents != 35000 {
		t.Fatalf("T23 net cash = %d, want 35000", got.NetCashCents)
	}
	if got.CashRefundableCents != 50000 || got.PayableShortfallCents != 15000 {
		t.Fatalf("T23 cash refundable/payable = %d/%d, want 50000/15000",
			got.CashRefundableCents, got.PayableShortfallCents)
	}
	if !got.ConservationOK {
		t.Fatalf("T23 conservation must hold: left=%d right=%d", got.ConservationLeftCents, got.ConservationRightCents)
	}
	// 守恒：35000 + 15000 = 300000 − 250000（spec §5 T23 原文）。
	if got.ConservationLeftCents != 50000 || got.ConservationRightCents != 50000 {
		t.Fatalf("T23 conservation = %d/%d, want 50000/50000", got.ConservationLeftCents, got.ConservationRightCents)
	}
}

// #2339 fixture：R0=3000、券后 2800、乐币 1000 点、押金 500、现金实收 2000。
// 段模型旧口径为 TotalRefund 666.67 / CashRefundable 506.67；spec v2.3 为
// net cash 40（4000 分）且 p_refund=160。
func TestComputeRefundSpecFixture2339_2423(t *testing.T) {
	in := RefundSpecInput{
		RAfterCouponCents:            280000,
		PUsedPoints:                  1000,
		RentCashPaidNetCents:         150000, // 现金实收 200000 − 押金 50000
		DepositCashCollectedNetCents: 50000,
		DepositHeldCents:             50000,
		OtherFeeCents:                0,
		CashPaidNetCents:             200000,
		RatioNum:                     3,
		RatioDen:                     10,
	}

	got, err := ComputeRefundSpec(in)
	if err != nil {
		t.Fatalf("ComputeRefundSpec(#2339) error: %v", err)
	}

	if got.PCapPoints != 840 {
		t.Fatalf("cap = %d, want 840", got.PCapPoints)
	}
	if got.PConsumedPoints != 840 || got.PRefundPoints != 160 {
		t.Fatalf("consumed/refund = %d/%d, want 840/160", got.PConsumedPoints, got.PRefundPoints)
	}
	if got.RentCashDueCents != 196000 {
		t.Fatalf("rent due = %d, want 196000", got.RentCashDueCents)
	}
	if got.RentShortfallCents != 46000 || got.RentRefundCents != 0 {
		t.Fatalf("rent shortfall/refund = %d/%d, want 46000/0", got.RentShortfallCents, got.RentRefundCents)
	}
	if got.DepositRefundCents != 50000 {
		t.Fatalf("deposit refund = %d, want 50000", got.DepositRefundCents)
	}
	if got.NetCashCents != 4000 {
		t.Fatalf("net cash = %d, want 4000 分 (¥40); 666.67/506.67 是段模型旧口径", got.NetCashCents)
	}
	if !got.ConservationOK {
		t.Fatalf("conservation must hold: left=%d right=%d", got.ConservationLeftCents, got.ConservationRightCents)
	}
}

// 守恒：固定种子生成 200 组内部一致的合法输入，逐组断言 §4 恒等式。
func TestComputeRefundSpecConservation_2423(t *testing.T) {
	rng := rand.New(rand.NewSource(2423))
	for i := 0; i < 200; i++ {
		ratioDen := int64(1 + rng.Intn(20))
		ratioNum := int64(rng.Intn(int(ratioDen) + 1))
		rAfterCoupon := int64(rng.Intn(5_000_000))
		pUsed := int64(rng.Intn(5_000))
		rentPaid := int64(rng.Intn(6_000_000))
		deposit := int64(rng.Intn(300_000))
		fees := int64(rng.Intn(200_000))

		in := RefundSpecInput{
			RAfterCouponCents:            rAfterCoupon,
			PUsedPoints:                  pUsed,
			RentCashPaidNetCents:         rentPaid,
			DepositCashCollectedNetCents: deposit,
			DepositHeldCents:             deposit,
			OtherFeeCents:                fees,
			CashPaidNetCents:             rentPaid + deposit,
			RatioNum:                     ratioNum,
			RatioDen:                     ratioDen,
		}

		got, err := ComputeRefundSpec(in)
		if err != nil {
			t.Fatalf("case %d: unexpected error: %v (input=%+v)", i, err, in)
		}
		if !got.ConservationOK {
			t.Fatalf("case %d: conservation_ok=false (left=%d right=%d)", i, got.ConservationLeftCents, got.ConservationRightCents)
		}
		left := got.NetCashCents + StoredCentsFromPoints(got.PRefundPoints)
		right := in.CashPaidNetCents + StoredCentsFromPoints(in.PUsedPoints) - in.RAfterCouponCents - in.OtherFeeCents
		if left != right {
			t.Fatalf("case %d: net + PointsToCash(p_refund) = %d, cash_paid + PointsToCash(p_used) - R - fees = %d",
				i, left, right)
		}
	}
}

// 边界：无押金 / 押金被扣用 / 券后租金为 0 / p_used > cap / 非法输入 / 溢出。
func TestComputeRefundSpecBounds_2423(t *testing.T) {
	t.Run("no_deposit", func(t *testing.T) {
		got, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents:    280000,
			PUsedPoints:          900,
			RentCashPaidNetCents: 160000,
			CashPaidNetCents:     160000,
			RatioNum:             3,
			RatioDen:             10,
		})
		if err != nil {
			t.Fatalf("no deposit: %v", err)
		}
		if got.DepositRefundCents != 0 || got.OtherFeeShortfallCents != 0 {
			t.Fatalf("no deposit: deposit refund=%d fee shortfall=%d, want 0/0",
				got.DepositRefundCents, got.OtherFeeShortfallCents)
		}
		if got.NetCashCents != got.RentRefundCents-got.RentShortfallCents {
			t.Fatalf("no deposit: net=%d rent=%d/%d", got.NetCashCents, got.RentRefundCents, got.RentShortfallCents)
		}
	})

	t.Run("deposit_consumed_by_fees", func(t *testing.T) {
		got, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents:            250000,
			PUsedPoints:                  900,
			RentCashPaidNetCents:         175000,
			DepositCashCollectedNetCents: 50000,
			DepositHeldCents:             30000,
			OtherFeeCents:                50000,
			CashPaidNetCents:             205000, // 租金 175000 + 押金实收 30000
			RatioNum:                     3,
			RatioDen:                     10,
		})
		if err != nil {
			t.Fatalf("deposit consumed: %v", err)
		}
		if got.DepositDeductionsCents != 30000 || got.DepositRefundCents != 0 {
			t.Fatalf("deposit consumed: deductions=%d refund=%d, want 30000/0",
				got.DepositDeductionsCents, got.DepositRefundCents)
		}
		if got.OtherFeeShortfallCents != 20000 {
			t.Fatalf("deposit consumed: fee shortfall=%d, want 20000", got.OtherFeeShortfallCents)
		}
		if got.PayableShortfallCents != 20000 || got.NetCashCents != -20000 {
			t.Fatalf("deposit consumed: payable=%d net=%d, want 20000/-20000",
				got.PayableShortfallCents, got.NetCashCents)
		}
	})

	t.Run("zero_rent_after_coupon", func(t *testing.T) {
		got, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents:            0,
			PUsedPoints:                  500,
			RentCashPaidNetCents:         0,
			DepositCashCollectedNetCents: 50000,
			DepositHeldCents:             50000,
			CashPaidNetCents:             50000,
			RatioNum:                     3,
			RatioDen:                     10,
		})
		if err != nil {
			t.Fatalf("zero rent: %v", err)
		}
		if got.PCapPoints != 0 || got.PConsumedPoints != 0 || got.PRefundPoints != 500 {
			t.Fatalf("zero rent: cap/consumed/refund = %d/%d/%d, want 0/0/500",
				got.PCapPoints, got.PConsumedPoints, got.PRefundPoints)
		}
		if got.RentCashDueCents != 0 || got.DepositRefundCents != 50000 || got.NetCashCents != 50000 {
			t.Fatalf("zero rent: due=%d deposit refund=%d net=%d, want 0/50000/50000",
				got.RentCashDueCents, got.DepositRefundCents, got.NetCashCents)
		}
	})

	t.Run("p_used_over_cap", func(t *testing.T) {
		got, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents:    28000,
			PUsedPoints:          1000,
			RentCashPaidNetCents: 28000,
			CashPaidNetCents:     28000,
			RatioNum:             3,
			RatioDen:             10,
		})
		if err != nil {
			t.Fatalf("over cap: %v", err)
		}
		if got.PCapPoints != 84 {
			t.Fatalf("over cap: cap=%d, want 84", got.PCapPoints)
		}
		if got.PRefundPoints != 916 {
			t.Fatalf("over cap: p_refund=%d, want 916", got.PRefundPoints)
		}
		if got.RentCashDueCents != 19600 {
			t.Fatalf("over cap: due=%d, want 19600", got.RentCashDueCents)
		}
	})

	invalid := []struct {
		name string
		in   RefundSpecInput
	}{
		{"negative_rent", RefundSpecInput{RAfterCouponCents: -1, RatioNum: 0, RatioDen: 1}},
		{"negative_points", RefundSpecInput{PUsedPoints: -1, RatioNum: 0, RatioDen: 1}},
		{"ratio_den_zero", RefundSpecInput{RAfterCouponCents: 100, RatioNum: 0, RatioDen: 0}},
		{"ratio_num_gt_den", RefundSpecInput{RAfterCouponCents: 100, RatioNum: 2, RatioDen: 1}},
		{"negative_fee", RefundSpecInput{RAfterCouponCents: 100, OtherFeeCents: -5, RatioNum: 0, RatioDen: 1}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ComputeRefundSpec(tc.in); !errors.Is(err, ErrSettlementInvalidInput) {
				t.Fatalf("%s: err=%v, want ErrSettlementInvalidInput", tc.name, err)
			}
		})
	}

	t.Run("overflow_cap", func(t *testing.T) {
		_, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents: math.MaxInt64,
			RatioNum:          3,
			RatioDen:          10,
		})
		if !errors.Is(err, ErrSettlementOverflow) {
			t.Fatalf("overflow: err=%v, want ErrSettlementOverflow", err)
		}
	})

	t.Run("conservation_mismatch_is_error", func(t *testing.T) {
		// cash_paid 与（租金实收 + 押金）不一致 → 守恒失败必须显式报错。
		got, err := ComputeRefundSpec(RefundSpecInput{
			RAfterCouponCents:            250000,
			PUsedPoints:                  900,
			RentCashPaidNetCents:         160000,
			DepositCashCollectedNetCents: 50000,
			DepositHeldCents:             50000,
			CashPaidNetCents:             160000, // 漏算押金
			RatioNum:                     3,
			RatioDen:                     10,
		})
		if !errors.Is(err, ErrSettlementInvariantViolation) {
			t.Fatalf("conservation mismatch: err=%v, want ErrSettlementInvariantViolation", err)
		}
		if got.ConservationOK {
			t.Fatal("conservation mismatch: ConservationOK must be false")
		}
		if got.ConservationLeftCents == got.ConservationRightCents {
			t.Fatalf("conservation mismatch: left/right unexpectedly equal (%d)", got.ConservationLeftCents)
		}
	})
}
