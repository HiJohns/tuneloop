package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestComputeDiscountedSettlement 固化 #1836 段级折后结算目标口径。
// 用例来自用户确认场景（09-07）：
//
//	场景 1：合同(ENO ¥0.36) + 续1(ENO ¥0.36) + 续2(ENO ¥0.36)，实付合计 ¥1.08
//	场景 2：合同(ENO ¥0.36) + 续1(无码 ¥36.00) + 续2(ENO ¥0.36)，实付合计 ¥36.72
//
// 物流费一律 ¥1.00、不参与优惠；租期段均为 1 天（原价 ¥36/天）。
// #1836 Round1 审计后：输入为 tier 精确子段（日租恒定）：
//
//	无码（Rate=1）结果必须与 rentPayable 的 tier 截断法逐分相等（跨阶用例 7）。
func TestComputeDiscountedSettlement(t *testing.T) {
	cents := func(yuan float64) int64 { return int64(yuan * 100) }

	daySeg := func(kind string, rate float64) PaidSegment {
		return PaidSegment{Kind: kind, Days: 1, OriginalDaily: cents(36), Rate: rate}
	}
	shipping := cents(1)

	t.Run("场景1-三码实付1.08-实租1天-应补0.28", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 0.01), daySeg("renewal", 0.01), daySeg("renewal", 0.01)}
		res, err := computeDiscountedSettlement(1, segs, shipping, cents(1.08))
		require.NoError(t, err)
		require.Equal(t, cents(0.36), res.DiscountedRent, "折后租金应收 0.36")
		require.Equal(t, cents(1.36), res.DiscountedDue, "折后应收 0.36+物流1.00")
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(0.28), res.NetAmount, "补缴 1.36−1.08=0.28")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("场景2A-续1无码实付36.72-实租1天-应退35.36", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 0.01), daySeg("renewal", 1.0), daySeg("renewal", 0.01)}
		res, err := computeDiscountedSettlement(1, segs, shipping, cents(36.72))
		require.NoError(t, err)
		require.Equal(t, cents(0.36), res.DiscountedRent)
		require.Equal(t, cents(1.36), res.DiscountedDue)
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, cents(35.36), res.NetAmount, "应退 36.72−1.36=35.36")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("场景2B-续1无码实付36.72-实租2天-应补0.64", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 0.01), daySeg("renewal", 1.0), daySeg("renewal", 0.01)}
		res, err := computeDiscountedSettlement(2, segs, shipping, cents(36.72))
		require.NoError(t, err)
		require.Equal(t, cents(36.36), res.DiscountedRent, "折后租金 0.36+36.00")
		require.Equal(t, cents(37.36), res.DiscountedDue, "折后应收 37.36")
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(0.64), res.NetAmount, "补缴 37.36−36.72=0.64")
		require.Equal(t, []int{1, 1, 0}, res.Usage)
	})

	t.Run("无码基线-三段原价108-实租1天-应退71", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 1.0), daySeg("renewal", 1.0), daySeg("renewal", 1.0)}
		res, err := computeDiscountedSettlement(1, segs, shipping, cents(108))
		require.NoError(t, err)
		require.Equal(t, cents(36), res.DiscountedRent)
		require.Equal(t, cents(37), res.DiscountedDue)
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, cents(71), res.NetAmount, "应退 108−37=71")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("租期跨三段-三码-实租3天-补物流1元", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 0.01), daySeg("renewal", 0.01), daySeg("renewal", 0.01)}
		res, err := computeDiscountedSettlement(3, segs, shipping, cents(1.08))
		require.NoError(t, err)
		require.Equal(t, cents(1.08), res.DiscountedRent)
		require.Equal(t, cents(2.08), res.DiscountedDue)
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(1), res.NetAmount, "补缴 2.08−1.08=1.00（物流费）")
		require.Equal(t, []int{1, 1, 1}, res.Usage)
	})

	t.Run("跨阶-无码-实租跨阶中途-与rentPayable tier截断逐分相等", func(t *testing.T) {
		// tier1: 10天@¥20 + tier2: 20天@¥15（原价 55000 分），实租 12 天
		segs := []PaidSegment{
			{Kind: "contract", Days: 10, OriginalDaily: cents(20), Rate: 1.0},
			{Kind: "contract", Days: 20, OriginalDaily: cents(15), Rate: 1.0},
		}
		res, err := computeDiscountedSettlement(12, segs, 0, cents(550))
		require.NoError(t, err)
		// 精确应收 = 10×2000 + 2×1500 = 23000（≠ 均匀折算 22000）
		require.Equal(t, int64(23000), res.DiscountedRent)
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, int64(55000-23000), res.NetAmount, "应退 55000−23000")
		require.Equal(t, []int{10, 2}, res.Usage)
	})

	t.Run("跨阶-有码1%-实租跨阶中途-按子段日租×段折扣率", func(t *testing.T) {
		// 同一跨阶合同，整单 ENO 1%（实付 55000×0.01=550 分），实租 12 天
		segs := []PaidSegment{
			{Kind: "contract", Days: 10, OriginalDaily: cents(20), Rate: 0.01},
			{Kind: "contract", Days: 20, OriginalDaily: cents(15), Rate: 0.01},
		}
		res, err := computeDiscountedSettlement(12, segs, cents(1), cents(5.50))
		require.NoError(t, err)
		// 折后 = 10×2000×0.01 + 2×1500×0.01 = 200 + 30 = 230 分
		require.Equal(t, int64(230), res.DiscountedRent)
		require.Equal(t, int64(330), res.DiscountedDue, "折后应收含物流 100 分")
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, int64(220), res.NetAmount, "应退 550−330=220")
	})

	t.Run("实租天数超过覆盖段-应报错", func(t *testing.T) {
		segs := []PaidSegment{daySeg("contract", 0.01), daySeg("renewal", 0.01), daySeg("renewal", 0.01)}
		_, err := computeDiscountedSettlement(4, segs, shipping, cents(1.08))
		require.Error(t, err)
	})

	t.Run("折扣率越界-应报错", func(t *testing.T) {
		segs := []PaidSegment{{Kind: "contract", Days: 1, OriginalDaily: cents(36), Rate: 1.5}}
		_, err := computeDiscountedSettlement(1, segs, shipping, cents(36))
		require.Error(t, err)
	})

	t.Run("零段且无实租-空结果-应退0", func(t *testing.T) {
		res, err := computeDiscountedSettlement(0, nil, 0, 0)
		require.NoError(t, err)
		require.Equal(t, "none", res.NetDirection)
		require.Equal(t, int64(0), res.NetAmount)
	})
}
