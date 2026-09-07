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
//   - 段展开为 tier 精确子段（日租在原价阶内恒定），实际租用天数按序吃掉子段
//   - 段折后 = Σ(子段被使用天数 × 子段原价日租 × 段折扣率)
//     段折扣率 = 该支付段实付 / 段原价（无码 = 1.0 → 与原 rentPayable tier
//     截断法逐分等价；有码时物流/逾期/定损不打折）
//   - 补缴/退款 = 折后应收 − 实付合计（路径 A：未租段的已付优惠款整体抵用，不退）
//
// 子段化而非整段均匀折算的原因（#1836 Round1 审计）：跨阶合同（tier>1 段）
// 整段均匀会把高阶日租摊到低阶，与 rentPayable 的 tier 精确截断产生偏差
// （例：tier1 10天@¥20 + tier2 20天@¥15，实租 12 天 → 均匀 ¥220 vs 精确 ¥230）。
type PaidSegment struct {
	Kind          string  // "contract" | "renewal"（该子段所属支付段）
	Days          int     // 子段覆盖天数（原价阶内）
	OriginalDaily int64   // 子段原价日租（分）
	Rate          float64 // 段折扣率（该支付段实付/原价，无码 1.0）
}

// segmentDiscountedSettlement 为段级折后结算的纯计算结果（分）。
type segmentDiscountedSettlement struct {
	// DiscountedRent 折后租金应收（分）＝ Σ 被使用子段天数 × 日租 × 段折扣率
	DiscountedRent int64
	// DiscountedDue 折后应收总额（分）＝ DiscountedRent + ShippingFee(原价)
	DiscountedDue int64
	// NetDirection "none" | "shortfall" | "refund"
	NetDirection string
	// NetAmount 补缴（shortfall，正）或应退（refund，正）
	NetAmount int64
	// Usage 每子段被实际使用的天数（按序，长度与 segments 一致）
	Usage []int
}

// computeDiscountedSettlement 计算段级折后应收与补退差。
// 实际租期 actualDays 按段顺序（合同 → renewal1 → …，段内 tier 子段序）覆盖；
// paidTotalCents 为该订单租金实付合计（路径 A 基数，含未租段已付优惠款）。
// 子段数据异常（天数/日租非正、折扣率越界）时返回 error（防御，勿静默产出错误金额）。
func computeDiscountedSettlement(actualDays int, segments []PaidSegment, shippingFeeCents, paidTotalCents int64) (segmentDiscountedSettlement, error) {
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
		if segments[i].OriginalDaily <= 0 {
			return res, errors.New("segment original daily must be positive")
		}
		if segments[i].Rate < 0 || segments[i].Rate > 1 {
			return res, errors.New("segment rate must be within [0,1]")
		}
		coverTotal += segments[i].Days
	}
	if coverTotal < actualDays {
		return res, errors.New("actual lease days exceed covered segment days")
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
			// 子段内日租恒定：used 天折后 = round(used × 日租 × 段折扣率)。
			// 无码（Rate=1）时为精确整数，与 rentPayable tier 截断逐分一致。
			discountedRent += int64(math.Round(float64(used) * float64(seg.OriginalDaily) * seg.Rate))
		}
		remaining -= used
		if remaining <= 0 {
			break
		}
	}

	res.DiscountedRent = discountedRent
	res.DiscountedDue = discountedRent + shippingFeeCents
	net := res.DiscountedDue - paidTotalCents
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

// makePaidSegments 由订单支付结构构建 tier 精确子段序列与租金实付合计。
// contractRent 为合同段实付合计（元）；renewalRecs 按 created_at ASC 传入。
// 每笔支付段（合同/续费）展开为其原价阶子段；子段原价日租 = base_daily_rent
// × 该子段政策折扣；段折扣率 = 段实付 / 段原价合计（展开与 paidBlock 同口径）。
func makePaidSegments(initialDays int, pricingTiers []services.PricingTierConfig,
	baseDailyRentCents float64, contractRent float64, renewalRecs []renewalRec) ([]PaidSegment, int64, error) {

	roundCents := func(v float64) int64 { return int64(math.Round(v)) }

	// tierDailyCents 返回第 tierIdx（1-based）档的原价日租（分）。
	// 权威口径：pricing_breakdown 组装时 TierSegment.Rate = PricingTiers[i].DailyRate
	// （rent_calculator.go CalculatePricingBreakdown），非 base_daily_rent
	// uniform 值——uniform 仅存在于 paid_block 展示层（预存展示缺陷，本模型不沿用）。
	tierDailyCents := func(tierIdx int) float64 {
		daily := baseDailyRentCents
		if tierIdx-1 < len(pricingTiers) && pricingTiers[tierIdx-1].DailyRate > 0 {
			daily = pricingTiers[tierIdx-1].DailyRate
		}
		return daily
	}

	// payment = 一个支付段聚合（用于折扣率与校验）
	type payment struct {
		kind         string
		days         int
		original     float64 // 分（原价合计）
		paid         int64   // 分（实付）
		tierSegments []services.TierSegment
	}
	payments := []payment{}

	// contract 支付段：初始租期（LeaseTerm）tier 展开
	contractTiers := services.ComputeTierSegments(initialDays, pricingTiers)
	contractOrig := 0.0
	for _, seg := range contractTiers {
		contractOrig += tierDailyCents(seg.Tier) * seg.Discount * float64(seg.Days)
	}
	payments = append(payments, payment{
		kind: "contract", days: initialDays,
		original:     contractOrig,
		paid:         roundCents(contractRent * 100),
		tierSegments: contractTiers,
	})

	offset := initialDays
	for _, r := range renewalRecs {
		if r.Days == nil || *r.Days <= 0 {
			return nil, 0, errors.New("renewal record missing days")
		}
		tiers := services.ComputeTierSegmentsFromOffset(offset, *r.Days, pricingTiers)
		orig := 0.0
		for _, seg := range tiers {
			orig += tierDailyCents(seg.Tier) * seg.Discount * float64(seg.Days)
		}
		payments = append(payments, payment{
			kind: "renewal", days: *r.Days,
			original:     orig,
			paid:         int64(r.Amount),
			tierSegments: tiers,
		})
		offset += *r.Days
	}

	segs := []PaidSegment{}
	paidTotal := int64(0)
	for _, p := range payments {
		if p.paid > roundCents(p.original) {
			return nil, 0, errors.New("segment paid exceeds original total")
		}
		paidTotal += p.paid
		rate := 1.0
		if p.original > 0 {
			rate = float64(p.paid) / p.original
		}
		for _, ts := range p.tierSegments {
			daily := tierDailyCents(ts.Tier) * ts.Discount
			segs = append(segs, PaidSegment{
				Kind:          p.kind,
				Days:          ts.Days,
				OriginalDaily: roundCents(daily),
				Rate:          rate,
			})
		}
	}
	return segs, paidTotal, nil
}
