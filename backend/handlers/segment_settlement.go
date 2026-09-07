package handlers

import (
	"errors"
	"math"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// renewalRec is a paid renewal payment record (used by the segment model and
// the paid_block rendering in computeSettlement).
type renewalRec struct {
	Amount models.Cents
	Days   *int
	Status string
}

// segment_settlement.go — 段级折后结算模型（#1836）
//
// 目标口径（用户/业务确认，2026-09-07）：
//   - 优惠码可多次使用，每笔支付（合同/各次续费）按支付顺序覆盖租期段
//   - 实际租用天数按序吃掉各段；段内日均折后单价 = 段原价日租 × (段实付 / 段原价)
//   - 折后应收 = Σ(各段被使用天数 × 段折后日租) + 物流费（原价，不参与优惠）
//   - 补缴/退款 = 折后应收 − 实付合计（路径 A：未租段的已付优惠款整体抵用，不退）
//   - 物流费不可优惠、不随任何比例折算（禁止 #1743 r 模型把物流打折）
type PaidSegment struct {
	Kind             string // "contract" | "renewal"
	Days             int    // 该段覆盖租期天数
	OriginalTotal    int64  // 该段原价总额（分）
	PaidTotal        int64  // 该段实付金额（分，优惠后）
	OriginalDailyCtx string // 描述用，无计算语义
}

// segmentDiscountedSettlement 为段级折后结算的纯计算结果（分）。
type segmentDiscountedSettlement struct {
	// DiscountedRent 折后租金应收（分）＝ Σ 被使用天数 × 段折后日租
	DiscountedRent int64
	// DiscountedDue 折后应收总额（分）＝ DiscountedRent + ShippingFee(原价)
	DiscountedDue int64
	// NetDirection "none" | "shortfall" | "refund"
	NetDirection string
	// NetAmount 补缴（shortfall，正）或应退（refund，正）
	NetAmount int64
	// Usage 每段被实际使用的天数（按序，长度与 segments 一致）
	Usage []int
}

// computeDiscountedSettlement 计算段级折后应收与补退差。
// 实际租期 actualDays 按段顺序（contract → renewal1 → …）覆盖；
// 段实付 > 段原价等数据异常时返回 error（防御，勿静默产出错误金额）。
func computeDiscountedSettlement(actualDays int, segments []PaidSegment, shippingFeeCents int64) (segmentDiscountedSettlement, error) {
	res := segmentDiscountedSettlement{
		NetDirection: "none",
		Usage:        make([]int, len(segments)),
	}
	if actualDays <= 0 {
		return res, nil // 未发生租用：无应收、无差额
	}

	coverTotal := 0
	for i := range segments {
		if segments[i].Days <= 0 {
			return res, errors.New("segment days must be positive")
		}
		if segments[i].OriginalTotal < 0 || segments[i].PaidTotal < 0 {
			return res, errors.New("segment amounts must be non-negative")
		}
		if segments[i].PaidTotal > segments[i].OriginalTotal {
			return res, errors.New("segment paid exceeds original total")
		}
		coverTotal += segments[i].Days
	}
	if coverTotal < actualDays {
		return res, errors.New("actual lease days exceed covered segment days")
	}

	// 实付合计统计全部段（已付未租段的优惠款按路径 A 整体抵用，不退）
	paidSum := int64(0)
	for i := range segments {
		paidSum += segments[i].PaidTotal
	}

	remaining := actualDays
	discountedRent := int64(0)
	for i, seg := range segments {
		used := seg.Days
		if used > remaining {
			used = remaining
		}
		res.Usage[i] = used
		if used > 0 {
			// 段内均匀折后：使用 used 天 = round(段实付 × used/Days)。
			// Days=1 时精确；Days>1 且实付非整分/天时四舍五入（真实合同段
			// 粒度疑点见 #1836 前置核实项）。
			discountedRent += (seg.PaidTotal*int64(used) + int64(seg.Days)/2) / int64(seg.Days)
		}
		remaining -= used
		if remaining <= 0 {
			break
		}
	}

	res.DiscountedRent = discountedRent
	res.DiscountedDue = discountedRent + shippingFeeCents
	net := res.DiscountedDue - paidSum
	switch {
	case net > 0:
		res.NetDirection = "shortfall"
		res.NetAmount = net
	case net < 0:
		res.NetDirection = "refund"
		res.NetAmount = -net
	}
	return res, nil
}

// makePaidSegments 由订单支付结构构建段序（合同段 → 各续费段，分口径）。
// contractRent 为合同段实付合计（元）；renewalRecs 按 created_at ASC 传入。
// 段原价 = 定价展开 subtotal 合计（与 paidBlock 渲染同口径）。
func makePaidSegments(initialDays int, pricingTiers []services.PricingTierConfig,
	baseDailyRentCents float64, contractRent float64, renewalRecs []renewalRec) ([]PaidSegment, error) {

	roundCents := func(v float64) int64 { return int64(math.Round(v)) }
	segs := []PaidSegment{}
	contractOrig := 0.0
	for _, seg := range services.ComputeTierSegments(initialDays, pricingTiers) {
		contractOrig += baseDailyRentCents * seg.Discount * float64(seg.Days)
	}
	segs = append(segs, PaidSegment{
		Kind: "contract", Days: initialDays,
		OriginalTotal: roundCents(contractOrig),
		PaidTotal:     roundCents(contractRent * 100),
	})
	offset := initialDays
	for _, r := range renewalRecs {
		if r.Days == nil || *r.Days <= 0 {
			return nil, errors.New("renewal record missing days")
		}
		orig := 0.0
		for _, seg := range services.ComputeTierSegmentsFromOffset(offset, *r.Days, pricingTiers) {
			orig += baseDailyRentCents * seg.Discount * float64(seg.Days)
		}
		segs = append(segs, PaidSegment{
			Kind: "renewal", Days: *r.Days,
			OriginalTotal: roundCents(orig),
			PaidTotal:     int64(r.Amount),
		})
		offset += *r.Days
	}
	return segs, nil
}
