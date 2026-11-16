package services

// #2359（#2268 子任务）：SettlementCalculator 纯计算模块。
//
// 按 docs/cases/settlement-refund-spec-v2.md v2.3 §2/§3/§8/§12 实现：
//   - 现金 int64 分、乐币 int64 点、比例 ratio_num/ratio_den 整型；禁止浮点。
//   - 券在前（Q16）：cap 基数 = R_after_coupon_cents。
//   - 押金与租金完全分离；额外费用独立核算；争议金额隔离。
//   - 纯函数：不写库、不做资金动作、不调用支付渠道。
//
// 唯一单位换算出口（业务代码禁止自行 ×100 / ÷100）：
//   PointsFromStoredCents / StoredCentsFromPoints / PointsToCashCents

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// AlgorithmVersionSettlement2359 是首个结算算法版本（历史单保留旧值，不重算）。
const AlgorithmVersionSettlement2359 = "settlement-v2.3"

// 结算比例来源（Q11）。
const (
	RatioSourceSnapshot   = "snapshot"
	RatioSourceHistory    = "history"
	RatioSourceUnresolved = "unresolved"
)

// 历史/不可解析标记（D4）。
const (
	LegacyMarkerPolicyUnresolved = "legacy_policy_unresolved"
	LegacyMarkerUnderpaid        = "legacy_underpaid"
)

var (
	ErrSettlementInvalidInput       = errors.New("settlement: invalid input")
	ErrSettlementOverflow           = errors.New("settlement: integer overflow")
	ErrSettlementInvariantViolation = errors.New("settlement: conservation invariant violated")
	ErrSettlementDuplicateActive    = errors.New("settlement: active settlement version already exists")
)

// PointsFromStoredCents 把以「分」存储的乐币换算为「点」（900 点 = 90000 分）。
// 存储分必须是 100 的整数倍（不足 1 点视为非法输入，避免静默丢弃余额）。
func PointsFromStoredCents(v int64) (int64, error) {
	if v < 0 {
		return 0, fmt.Errorf("%w: stored points cents must be >= 0 (got %d)", ErrSettlementInvalidInput, v)
	}
	if v%100 != 0 {
		return 0, fmt.Errorf("%w: stored points cents must be a multiple of 100 (got %d)", ErrSettlementInvalidInput, v)
	}
	return v / 100, nil
}

// StoredCentsFromPoints 把「点」换算为以「分」存储的乐币金额。
func StoredCentsFromPoints(p int64) int64 { return p * 100 }

// PointsToCashCents 把乐币按 1 点 = 100 分折算为现金（分）。
func PointsToCashCents(p int64) int64 { return p * 100 }

// PricingTier 是价格规则的阶梯（days_max <= 0 表示无上限）。
// discount_percent 为整型百分比（30 表示 30% 折扣），与定价模板一致。
type PricingTier struct {
	DaysMax         int
	DiscountPercent int64
}

// TermVersions 是合同期限与续费版本（T8 要求显式夹具）。
type TermVersions struct {
	OriginalTermDays       int
	OriginalRentCents      int64
	RenewalTermDays        int
	RenewalRentCents       int64
	TotalContractDays      int
	TotalContractRentCents int64
	DepositPolicy          string // carry_over（Q15）
}

// SettlementInput 是计算的完整输入（结算开始时冻结；重试必须复用同一份）。
type SettlementInput struct {
	SettlementID string
	OrderID      string
	// SettlementVersion：本次计算版本；ExistingActiveVersion > 0 表示订单已有
	// 未冲正的最终结算（confirmed/processing/succeeded/partially_succeeded/
	// failed_retryable），此时只允许重试同一版本（T25）。
	SettlementVersion     int64
	ExistingActiveVersion int64

	// 比例（Q11：快照优先，缺失走历史版本，不可追溯 → unresolved，不猜）。
	RatioNum    int64
	RatioDen    int64
	RatioSource string

	// 价格规则（阶梯按实际天数重算 R_actual_list）。
	PricingPolicyVersion string
	DailyRateCents       int64
	PricingTiers         []PricingTier
	ActualDays           int
	OverdueRentCents     int64 // Q14：overdue_rent 入租金基数
	CouponDiscountCents  int64

	// 乐币（以「分」存储；900 点 = 90000）。
	GiftPointsUsedStoredCents int64

	// 现金实收与账本（Q17/T26：租金实收只读分配账本）。
	CashPaidCents                int64
	RentCashPaidNetCents         int64
	DepositCashCollectedNetCents int64
	DepositHeldCents             int64

	// 额外费用分类（P5）与争议隔离（P6/§8）。
	OverduePenaltyCents int64
	DamageFeeCents      int64
	LogisticsFeeCents   int64
	DisputedAmountCents int64
	DisputeStatus       string
	DisputeReferenceID  string

	TermVersions TermVersions
}

// SettlementResult 是 §12 的全字段输出（纯计算，无执行状态）。
type SettlementResult struct {
	SettlementID      string    `json:"settlement_id"`
	OrderID           string    `json:"order_id"`
	SettlementVersion int64     `json:"settlement_version"`
	Status            string    `json:"status"`
	CalculatedAt      time.Time `json:"calculated_at"`
	AlgorithmVersion  string    `json:"algorithm_version"`

	RatioNum     int64  `json:"ratio_num"`
	RatioDen     int64  `json:"ratio_den"`
	RatioSource  string `json:"ratio_source"`
	LegacyMarker string `json:"legacy_marker,omitempty"`

	RActualListCents    int64 `json:"R_actual_list_cents"`
	CouponDiscountCents int64 `json:"coupon_discount_cents"`
	RAfterCouponCents   int64 `json:"R_after_coupon_cents"`
	ActualDays          int   `json:"actual_days"`

	RentCashPaidNetCents int64 `json:"rent_cash_paid_net_cents"`
	RentCashDueCents     int64 `json:"rent_cash_due_cents"`
	RentRefundCents      int64 `json:"rent_refund_cents"`
	RentShortfallCents   int64 `json:"rent_shortfall_cents"`

	DepositCashCollectedNetCents int64 `json:"deposit_cash_collected_net_cents"`
	DepositHeldCents             int64 `json:"deposit_held_cents"`
	DepositDeductionsCents       int64 `json:"deposit_deductions_cents"`
	DepositRefundCents           int64 `json:"deposit_refund_cents"`

	DeductionsTotalCents      int64 `json:"deductions_total_cents"`
	DisputedAmountCents       int64 `json:"disputed_amount_cents"`
	UndisputedDeductionsCents int64 `json:"undisputed_deductions_cents"`
	OtherFeeShortfallCents    int64 `json:"other_fee_shortfall_cents"`
	OverduePenaltyCents       int64 `json:"overdue_penalty_cents"`
	DamageFeeCents            int64 `json:"damage_fee_cents"`
	LogisticsFeeCents         int64 `json:"logistics_fee_cents"`

	GiftPointsUsed     int64 `json:"gift_points_used"`
	GiftPointsCap      int64 `json:"gift_points_cap"`
	GiftPointsConsumed int64 `json:"gift_points_consumed"`
	GiftPointsRefunded int64 `json:"gift_points_refunded"`

	CashRefundableCents int64 `json:"cash_refundable_cents"`
	CashPayableCents    int64 `json:"cash_payable_cents"`
	NetCashCents        int64 `json:"net_cash_cents"`

	InitialCashCollectedCents int64 `json:"initial_cash_collected_cents"`

	DisputeStatus      string `json:"dispute_status,omitempty"`
	DisputeReferenceID string `json:"dispute_reference_id,omitempty"`

	PricingPolicyVersion string `json:"pricing_policy_version,omitempty"`

	OriginalTermDays       int    `json:"original_term_days"`
	OriginalRentCents      int64  `json:"original_rent_cents"`
	RenewalTermDays        int    `json:"renewal_term_days"`
	RenewalRentCents       int64  `json:"renewal_rent_cents"`
	TotalContractDays      int    `json:"total_contract_days"`
	TotalContractRentCents int64  `json:"total_contract_rent_cents"`
	DepositPolicy          string `json:"deposit_policy,omitempty"`
}

// SettlementCalculator 是纯计算器；Now 可注入以便测试（默认 time.Now）。
type SettlementCalculator struct {
	AlgorithmVersion string
	Now              func() time.Time
}

func NewSettlementCalculator() *SettlementCalculator {
	return &SettlementCalculator{AlgorithmVersion: AlgorithmVersionSettlement2359, Now: time.Now}
}

// Calculate 计算结算结果。任何非法输入/溢出都返回明确错误；守恒式不成立返回
// ErrSettlementInvariantViolation（不得返回部分结果）。
func (c *SettlementCalculator) Calculate(in SettlementInput) (*SettlementResult, error) {
	if c == nil {
		c = NewSettlementCalculator()
	}
	if err := validateSettlementInput(in); err != nil {
		return nil, err
	}
	// T25：存在未冲正的最终结算时，只允许重试同一版本。
	if in.ExistingActiveVersion > 0 && in.SettlementVersion != in.ExistingActiveVersion {
		return nil, fmt.Errorf("%w: order %s already has active settlement version %d; reverse it before creating version %d",
			ErrSettlementDuplicateActive, in.OrderID, in.ExistingActiveVersion, in.SettlementVersion)
	}

	pUsed, err := PointsFromStoredCents(in.GiftPointsUsedStoredCents)
	if err != nil {
		return nil, err
	}

	// 1) R_actual_list：标价 × 实际天数（阶梯折扣）+ 逾期租金（Q14）。
	rList, err := calcActualList(in.DailyRateCents, in.ActualDays, in.OverdueRentCents, in.PricingTiers)
	if err != nil {
		return nil, err
	}
	if in.CouponDiscountCents > rList {
		return nil, fmt.Errorf("%w: coupon discount %d exceeds rent %d", ErrSettlementInvalidInput, in.CouponDiscountCents, rList)
	}
	rAfterCoupon := rList - in.CouponDiscountCents

	res := &SettlementResult{
		SettlementID:                 in.SettlementID,
		OrderID:                      in.OrderID,
		SettlementVersion:            in.SettlementVersion,
		Status:                       "calculated",
		CalculatedAt:                 c.now(),
		AlgorithmVersion:             c.AlgorithmVersion,
		RatioNum:                     in.RatioNum,
		RatioDen:                     in.RatioDen,
		RatioSource:                  in.RatioSource,
		RActualListCents:             rList,
		CouponDiscountCents:          in.CouponDiscountCents,
		RAfterCouponCents:            rAfterCoupon,
		ActualDays:                   in.ActualDays,
		RentCashPaidNetCents:         in.RentCashPaidNetCents,
		DepositCashCollectedNetCents: in.DepositCashCollectedNetCents,
		DepositHeldCents:             in.DepositHeldCents,
		OverduePenaltyCents:          in.OverduePenaltyCents,
		DamageFeeCents:               in.DamageFeeCents,
		LogisticsFeeCents:            in.LogisticsFeeCents,
		DisputedAmountCents:          in.DisputedAmountCents,
		DisputeStatus:                in.DisputeStatus,
		DisputeReferenceID:           in.DisputeReferenceID,
		PricingPolicyVersion:         in.PricingPolicyVersion,
		GiftPointsUsed:               pUsed,
		OriginalTermDays:             in.TermVersions.OriginalTermDays,
		OriginalRentCents:            in.TermVersions.OriginalRentCents,
		RenewalTermDays:              in.TermVersions.RenewalTermDays,
		RenewalRentCents:             in.TermVersions.RenewalRentCents,
		TotalContractDays:            in.TermVersions.TotalContractDays,
		TotalContractRentCents:       in.TermVersions.TotalContractRentCents,
		DepositPolicy:                in.TermVersions.DepositPolicy,
	}

	// 2) 乐币：券后租金为 cap 基数（Q16）。
	if in.RatioSource == RatioSourceUnresolved {
		// D4：不可追溯 → 不猜比例、不产生乐币退款，标记后人工确认。
		// 「不退款」= 已用点全部视为消耗（consumed = p_used），否则守恒式会把
		// 未决的乐币权益算成多收现金。
		res.LegacyMarker = LegacyMarkerPolicyUnresolved
		res.GiftPointsCap = 0
		res.GiftPointsConsumed = pUsed
		res.GiftPointsRefunded = 0
	} else {
		capPoints, err := calcPointsCap(rAfterCoupon, in.RatioNum, in.RatioDen)
		if err != nil {
			return nil, err
		}
		res.GiftPointsCap = capPoints
		res.GiftPointsConsumed = minInt64(pUsed, capPoints)
		res.GiftPointsRefunded = pUsed - res.GiftPointsConsumed
	}

	// 3) 租金账户：应付 = 券后租金 - 乐币折算；实收来自账本（T26）。
	pointsCash, err := pointsToCashChecked(res.GiftPointsConsumed)
	if err != nil {
		return nil, err
	}
	if pointsCash > rAfterCoupon {
		return nil, fmt.Errorf("%w: points cash %d exceeds rent after coupon %d", ErrSettlementOverflow, pointsCash, rAfterCoupon)
	}
	res.RentCashDueCents = rAfterCoupon - pointsCash
	balance, err := subInt64(in.RentCashPaidNetCents, res.RentCashDueCents)
	if err != nil {
		return nil, err
	}
	if balance >= 0 {
		res.RentRefundCents = balance
	} else {
		res.RentShortfallCents = -balance
	}

	// 4) 额外费用 + 争议隔离（P5/P6）。
	deductions, err := sumInt64(in.OverduePenaltyCents, in.DamageFeeCents, in.LogisticsFeeCents)
	if err != nil {
		return nil, err
	}
	if in.DisputedAmountCents > deductions {
		return nil, fmt.Errorf("%w: disputed amount %d exceeds total deductions %d", ErrSettlementInvalidInput, in.DisputedAmountCents, deductions)
	}
	res.DeductionsTotalCents = deductions
	res.UndisputedDeductionsCents = deductions - in.DisputedAmountCents

	res.DepositDeductionsCents = minInt64(in.DepositHeldCents, res.UndisputedDeductionsCents)
	res.DepositRefundCents = in.DepositHeldCents - res.DepositDeductionsCents
	res.OtherFeeShortfallCents = maxInt64(0, res.UndisputedDeductionsCents-in.DepositHeldCents)

	// 5) 汇总。
	res.CashRefundableCents, err = sumInt64(res.RentRefundCents, res.DepositRefundCents)
	if err != nil {
		return nil, err
	}
	res.CashPayableCents, err = sumInt64(res.RentShortfallCents, res.OtherFeeShortfallCents)
	if err != nil {
		return nil, err
	}
	res.NetCashCents = res.CashRefundableCents - res.CashPayableCents

	// 6) Q8 足额收款口径（券在前）。
	initialPointsCash, err := pointsToCashChecked(pUsed)
	if err != nil {
		return nil, err
	}
	res.InitialCashCollectedCents, err = sumInt64(rAfterCoupon-initialPointsCash, in.DepositCashCollectedNetCents)
	if err != nil {
		return nil, err
	}

	// 7) 守恒式：net + p_refund*100 = cash_paid + p_used*100 - R_after_coupon - deductions_total。
	if err := checkSettlementInvariant(res, in.CashPaidCents); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *SettlementCalculator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func validateSettlementInput(in SettlementInput) error {
	if in.SettlementVersion <= 0 {
		return fmt.Errorf("%w: settlement_version must be >= 1", ErrSettlementInvalidInput)
	}
	if in.ExistingActiveVersion < 0 {
		return fmt.Errorf("%w: existing active version must be >= 0", ErrSettlementInvalidInput)
	}
	if in.RatioDen <= 0 {
		return fmt.Errorf("%w: ratio_den must be > 0", ErrSettlementInvalidInput)
	}
	if in.RatioNum < 0 || in.RatioNum > in.RatioDen {
		return fmt.Errorf("%w: ratio must satisfy 0 <= ratio_num <= ratio_den", ErrSettlementInvalidInput)
	}
	switch in.RatioSource {
	case RatioSourceSnapshot, RatioSourceHistory, RatioSourceUnresolved:
	default:
		return fmt.Errorf("%w: ratio_source must be snapshot|history|unresolved", ErrSettlementInvalidInput)
	}
	if in.ActualDays < 0 {
		return fmt.Errorf("%w: actual_days must be >= 0", ErrSettlementInvalidInput)
	}
	for name, v := range map[string]int64{
		"daily_rate_cents":         in.DailyRateCents,
		"overdue_rent_cents":       in.OverdueRentCents,
		"coupon_discount_cents":    in.CouponDiscountCents,
		"cash_paid_cents":          in.CashPaidCents,
		"rent_cash_paid_net_cents": in.RentCashPaidNetCents,
		"deposit_collected_cents":  in.DepositCashCollectedNetCents,
		"deposit_held_cents":       in.DepositHeldCents,
		"overdue_penalty_cents":    in.OverduePenaltyCents,
		"damage_fee_cents":         in.DamageFeeCents,
		"logistics_fee_cents":      in.LogisticsFeeCents,
		"disputed_amount_cents":    in.DisputedAmountCents,
		"gift_points_used_cents":   in.GiftPointsUsedStoredCents,
	} {
		if v < 0 {
			return fmt.Errorf("%w: %s must be >= 0 (got %d)", ErrSettlementInvalidInput, name, v)
		}
	}
	for _, t := range in.PricingTiers {
		if t.DiscountPercent < 0 || t.DiscountPercent > 100 {
			return fmt.Errorf("%w: tier discount_percent must be within 0..100 (got %d)", ErrSettlementInvalidInput, t.DiscountPercent)
		}
	}
	return nil
}

// calcActualList 以标价 × 实际天数计算 R_actual_list，并按命中阶梯打折。
func calcActualList(dailyRate int64, days int, overdueRent int64, tiers []PricingTier) (int64, error) {
	base, err := mulInt64(dailyRate, int64(days))
	if err != nil {
		return 0, err
	}
	discount := int64(0)
	for _, t := range tiers {
		if t.DaysMax <= 0 || days <= t.DaysMax {
			discount = t.DiscountPercent
			break
		}
	}
	if discount > 0 {
		net, err := mulInt64(base, 100-discount)
		if err != nil {
			return 0, err
		}
		base = net / 100
	}
	return addInt64(base, overdueRent)
}

// CalcPointsCap = floor(R_after_coupon * ratio_num / (100 * ratio_den))。
//
// #2359/#2362：这是「支付端 cap」与「结算端 cap」的唯一公式出口；任何调用方
// 不得自行实现第二套公式（审计按重复实现 REJECT）。
func CalcPointsCap(rAfterCouponCents, ratioNum, ratioDen int64) (int64, error) {
	if rAfterCouponCents < 0 {
		return 0, fmt.Errorf("%w: rent after coupon must be >= 0", ErrSettlementInvalidInput)
	}
	if ratioDen <= 0 {
		return 0, fmt.Errorf("%w: ratio_den must be > 0", ErrSettlementInvalidInput)
	}
	if ratioNum < 0 || ratioNum > ratioDen {
		return 0, fmt.Errorf("%w: ratio must satisfy 0 <= ratio_num <= ratio_den", ErrSettlementInvalidInput)
	}
	product, err := mulInt64(rAfterCouponCents, ratioNum)
	if err != nil {
		return 0, err
	}
	den, err := mulInt64(100, ratioDen)
	if err != nil {
		return 0, err
	}
	return product / den, nil
}

// calcPointsCap 保留旧名（包内调用），统一委托公开出口。
func calcPointsCap(rAfterCoupon, ratioNum, ratioDen int64) (int64, error) {
	return CalcPointsCap(rAfterCoupon, ratioNum, ratioDen)
}

func pointsToCashChecked(p int64) (int64, error) {
	if p < 0 {
		return 0, fmt.Errorf("%w: points must be >= 0", ErrSettlementInvalidInput)
	}
	return mulInt64(p, 100)
}

// checkSettlementInvariant 校验 §4 守恒式；不成立时返回错误而不是返回结果。
func checkSettlementInvariant(res *SettlementResult, cashPaid int64) error {
	// 左：net + p_refund*100
	left, err := sumInt64(res.NetCashCents, StoredCentsFromPoints(res.GiftPointsRefunded))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	// 右：cash_paid + p_used*100 - R_after_coupon - 本期实际扣款
	// 说明：§4 写作 deductions_total；§8/P6 要求争议金额挂起、不进入本期扣款，
	// 因此本期实际扣款 = undisputed_deductions。无争议时两者相等，公式与 §4 逐字一致；
	// 有争议时若仍用 total，会把「挂起未扣」的金额算作已扣，T24 的押金退 40000 将无法守恒。
	right, err := sumInt64(cashPaid, StoredCentsFromPoints(res.GiftPointsUsed))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	right, err = subInt64(right, res.RAfterCouponCents)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	right, err = subInt64(right, res.UndisputedDeductionsCents)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSettlementOverflow, err)
	}
	if left != right {
		return fmt.Errorf("%w: left=%d right=%d (net=%d p_refund=%d cash_paid=%d p_used=%d R_after_coupon=%d undisputed_deductions=%d)",
			ErrSettlementInvariantViolation, left, right, res.NetCashCents, res.GiftPointsRefunded,
			cashPaid, res.GiftPointsUsed, res.RAfterCouponCents, res.UndisputedDeductionsCents)
	}
	return nil
}

func addInt64(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, fmt.Errorf("%w: %d + %d", ErrSettlementOverflow, a, b)
	}
	return a + b, nil
}

func subInt64(a, b int64) (int64, error) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, fmt.Errorf("%w: %d - %d", ErrSettlementOverflow, a, b)
	}
	return a - b, nil
}

func mulInt64(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a == -1 && b == math.MinInt64 || b == -1 && a == math.MinInt64 {
		return 0, fmt.Errorf("%w: %d * %d", ErrSettlementOverflow, a, b)
	}
	if (a > 0 && b > 0 && a > math.MaxInt64/b) ||
		(a > 0 && b < 0 && b < math.MinInt64/a) ||
		(a < 0 && b > 0 && a < math.MinInt64/b) ||
		(a < 0 && b < 0 && a < math.MaxInt64/b) {
		return 0, fmt.Errorf("%w: %d * %d", ErrSettlementOverflow, a, b)
	}
	return a * b, nil
}

func sumInt64(vals ...int64) (int64, error) {
	var out int64
	for _, v := range vals {
		next, err := addInt64(out, v)
		if err != nil {
			return 0, err
		}
		out = next
	}
	return out, nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
