package services

// #2423（#2417 S1）：spec v2.3 退款计算纯函数。
//
// 严格按 docs/cases/settlement-refund-spec-v2.md v2.3 §3（分账户）/§4（守恒）
// 实现，供 S2（#2424）切换 computeSettlement 时复用：
//   - 现金一律 int64 分、乐币 int64 点（1 点 = 100 分）、比例整型；
//   - 券在前（Q16）：cap 基数 = R_after_coupon_cents；
//   - 押金与租金完全分离（P1）；额外费用可扣押金、不足转补缴（P5）；
//   - 纯函数：不查库、不读时钟、不做资金动作。
//
// 不接线（Out of Scope）：本文件不被 computeSettlement 或任何 handler 引用。
//
// 关于父单 #2417 Decision Lock 的 `deposit_refund` 行：
// #2417 已勘误（comment 2026-11-20T05:xx，Architect）：`deposit_refund` 以
// docs/cases 用例文档为准，即 spec §3
// `deposit_held − min(deposit_held, undisputed_deductions)`；
// `deposit_cash_collected_net` 仅用于押金足额校验，不参与退款算式。
// 本实现与该口径一致（T23 押金退 50000、#2339 net cash 40）。

import (
	"fmt"
)

// RefundSpecInput 是退款计算的全部输入（单位：分；PUsedPoints 为点）。
type RefundSpecInput struct {
	// RAfterCouponCents = 实际天数标价租金 − 优惠券抵扣（>= 0）。
	RAfterCouponCents int64 `json:"R_after_coupon_cents"`
	// PUsedPoints 是下单时使用的乐币点数。
	PUsedPoints int64 `json:"p_used_points"`
	// RentCashPaidNetCents 是租金现金净实收，必须来自支付分配账本（§3.1）。
	// legacy 回退由调用方算好后传入（D2），并置 LegacyDerivedRentCash。
	RentCashPaidNetCents int64 `json:"rent_cash_paid_net_cents"`
	// DepositCashCollectedNetCents 是押金累计净收款（仅记账/审计，不参与退款公式）。
	DepositCashCollectedNetCents int64 `json:"deposit_cash_collected_net_cents"`
	// DepositHeldCents 是当前持有押金（未退未扣）。
	DepositHeldCents int64 `json:"deposit_held_cents"`
	// OtherFeeCents 是本期无争议额外费用合计（逾期/定损/物流）。
	OtherFeeCents int64 `json:"other_fee_cents"`
	// CashPaidNetCents 是结算口径现金净实收（含押金），守恒式 §4 必需。
	CashPaidNetCents int64 `json:"cash_paid_cents"`

	RatioNum int64 `json:"ratio_num"`
	RatioDen int64 `json:"ratio_den"`

	// LegacyDerivedRentCash 标记 RentCashPaidNetCents 由 §3.1 legacy 回退推导
	// （无押金扣用 + 历史退款分配完整），仅用于审计标记，不改变公式。
	LegacyDerivedRentCash bool `json:"legacy_derived_rent_cash,omitempty"`
}

// RefundSpec 是 §3/§4 的分账户结果（现金为分，乐币为点）。
type RefundSpec struct {
	// 乐币
	PUsedPoints     int64 `json:"p_used_points"`
	PCapPoints      int64 `json:"p_cap_points"`
	PConsumedPoints int64 `json:"p_consumed_points"`
	PRefundPoints   int64 `json:"p_refund_points"`

	// 租金账户
	RentCashDueCents   int64 `json:"rent_cash_due_cents"`
	RentBalanceCents   int64 `json:"rent_balance_cents"`
	RentRefundCents    int64 `json:"rent_refund_cents"`
	RentShortfallCents int64 `json:"rent_shortfall_cents"`

	// 押金账户
	DepositCashCollectedNetCents int64 `json:"deposit_cash_collected_net_cents"`
	DepositHeldCents             int64 `json:"deposit_held_cents"`
	DepositDeductionsCents       int64 `json:"deposit_deductions_cents"`
	DepositRefundCents           int64 `json:"deposit_refund_cents"`

	// 额外费用缺口（押金不足部分）
	OtherFeeCents          int64 `json:"other_fee_cents"`
	OtherFeeShortfallCents int64 `json:"other_fee_shortfall_cents"`
	CashRefundableCents    int64 `json:"cash_refundable_cents"`
	PayableShortfallCents  int64 `json:"payable_shortfall_cents"`
	NetCashCents           int64 `json:"net_cash_cents"`
	ConservationOK         bool  `json:"conservation_ok"`
	LegacyDerivedRentCash  bool  `json:"legacy_derived_rent_cash,omitempty"`
	ConservationLeftCents  int64 `json:"conservation_left_cents"`
	ConservationRightCents int64 `json:"conservation_right_cents"`
}

// ComputeRefundSpec 按 spec v2.3 §3/§4 计算退款分账户结果。
//
// 非法输入（负值、ratio_den<=0、ratio_num>ratio_den、溢出）返回
// ErrSettlementInvalidInput / ErrSettlementOverflow；守恒式不成立返回
// ErrSettlementInvariantViolation，同时返回 ConservationOK=false 的结果（D4：
// 不静默、不返回部分成功）。
func ComputeRefundSpec(in RefundSpecInput) (RefundSpec, error) {
	var out RefundSpec

	if err := validateRefundSpecInput(in); err != nil {
		return out, err
	}

	pCap, err := CalcPointsCap(in.RAfterCouponCents, in.RatioNum, in.RatioDen)
	if err != nil {
		return out, err
	}
	pConsumed := minInt64(in.PUsedPoints, pCap)
	pRefund := in.PUsedPoints - pConsumed

	pointsCash, err := pointsToCashChecked(pConsumed)
	if err != nil {
		return out, err
	}
	if pointsCash > in.RAfterCouponCents {
		return out, fmt.Errorf("%w: points cash %d exceeds rent after coupon %d",
			ErrSettlementInvalidInput, pointsCash, in.RAfterCouponCents)
	}
	due := in.RAfterCouponCents - pointsCash

	balance, err := subInt64(in.RentCashPaidNetCents, due)
	if err != nil {
		return out, err
	}

	depositDeductions := minInt64(in.DepositHeldCents, in.OtherFeeCents)
	depositRefund := in.DepositHeldCents - depositDeductions
	otherFeeShortfall := maxInt64(0, in.OtherFeeCents-in.DepositHeldCents)

	refundable, err := sumInt64(maxInt64(0, balance), depositRefund)
	if err != nil {
		return out, err
	}
	payable, err := sumInt64(maxInt64(0, -balance), otherFeeShortfall)
	if err != nil {
		return out, err
	}
	net, err := subInt64(refundable, payable)
	if err != nil {
		return out, err
	}

	out = RefundSpec{
		PUsedPoints:                  in.PUsedPoints,
		PCapPoints:                   pCap,
		PConsumedPoints:              pConsumed,
		PRefundPoints:                pRefund,
		RentCashDueCents:             due,
		RentBalanceCents:             balance,
		RentRefundCents:              maxInt64(0, balance),
		RentShortfallCents:           maxInt64(0, -balance),
		DepositCashCollectedNetCents: in.DepositCashCollectedNetCents,
		DepositHeldCents:             in.DepositHeldCents,
		DepositDeductionsCents:       depositDeductions,
		DepositRefundCents:           depositRefund,
		OtherFeeCents:                in.OtherFeeCents,
		OtherFeeShortfallCents:       otherFeeShortfall,
		CashRefundableCents:          refundable,
		PayableShortfallCents:        payable,
		NetCashCents:                 net,
		LegacyDerivedRentCash:        in.LegacyDerivedRentCash,
	}

	// §4 守恒：net + PointsToCash(p_refund) = cash_paid + PointsToCash(p_used)
	//                                                        − R_after_coupon − deductions。
	left, err := sumInt64(out.NetCashCents, StoredCentsFromPoints(out.PRefundPoints))
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	rightStart, err := sumInt64(in.CashPaidNetCents, StoredCentsFromPoints(in.PUsedPoints))
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	right, err := subInt64(rightStart, in.RAfterCouponCents)
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	right, err = subInt64(right, in.OtherFeeCents)
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	out.ConservationLeftCents = left
	out.ConservationRightCents = right

	if left != right {
		return out, fmt.Errorf("%w: left=%d right=%d (net=%d p_refund=%d cash_paid=%d p_used=%d R_after_coupon=%d deductions=%d)",
			ErrSettlementInvariantViolation, left, right, out.NetCashCents, out.PRefundPoints,
			in.CashPaidNetCents, in.PUsedPoints, in.RAfterCouponCents, in.OtherFeeCents)
	}
	out.ConservationOK = true
	return out, nil
}

func validateRefundSpecInput(in RefundSpecInput) error {
	if in.RatioDen <= 0 {
		return fmt.Errorf("%w: ratio_den must be > 0 (got %d)", ErrSettlementInvalidInput, in.RatioDen)
	}
	if in.RatioNum < 0 || in.RatioNum > in.RatioDen {
		return fmt.Errorf("%w: ratio must satisfy 0 <= ratio_num <= ratio_den (got %d/%d)",
			ErrSettlementInvalidInput, in.RatioNum, in.RatioDen)
	}
	for name, v := range map[string]int64{
		"R_after_coupon_cents":             in.RAfterCouponCents,
		"p_used_points":                    in.PUsedPoints,
		"rent_cash_paid_net_cents":         in.RentCashPaidNetCents,
		"deposit_cash_collected_net_cents": in.DepositCashCollectedNetCents,
		"deposit_held_cents":               in.DepositHeldCents,
		"other_fee_cents":                  in.OtherFeeCents,
		"cash_paid_cents":                  in.CashPaidNetCents,
	} {
		if v < 0 {
			return fmt.Errorf("%w: %s must be >= 0 (got %d)", ErrSettlementInvalidInput, name, v)
		}
	}
	return nil
}
