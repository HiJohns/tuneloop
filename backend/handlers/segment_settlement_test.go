package handlers

import (
	"errors"
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
func TestComputeDiscountedSettlement(t *testing.T) {
	cents := func(yuan float64) int64 { return int64(yuan * 100) }

	enoSeg := func(kind string) PaidSegment {
		return PaidSegment{Kind: kind, Days: 1, OriginalTotal: cents(36), PaidTotal: cents(0.36)}
	}
	fullSeg := func(kind string) PaidSegment {
		return PaidSegment{Kind: kind, Days: 1, OriginalTotal: cents(36), PaidTotal: cents(36)}
	}
	shipping := cents(1)

	t.Run("场景1-三码实付1.08-实租1天-应补0.28", func(t *testing.T) {
		segs := []PaidSegment{enoSeg("contract"), enoSeg("renewal"), enoSeg("renewal")}
		res, err := computeDiscountedSettlement(1, segs, shipping)
		require.NoError(t, err)
		require.Equal(t, cents(0.36), res.DiscountedRent, "折后租金应收 0.36")
		require.Equal(t, cents(1.36), res.DiscountedDue, "折后应收 0.36+物流1.00")
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(0.28), res.NetAmount, "补缴 1.36−1.08=0.28")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("场景2A-续1无码实付36.72-实租1天-应退35.36", func(t *testing.T) {
		segs := []PaidSegment{enoSeg("contract"), fullSeg("renewal"), enoSeg("renewal")}
		res, err := computeDiscountedSettlement(1, segs, shipping)
		require.NoError(t, err)
		require.Equal(t, cents(0.36), res.DiscountedRent)
		require.Equal(t, cents(1.36), res.DiscountedDue)
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, cents(35.36), res.NetAmount, "应退 36.72−1.36=35.36")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("场景2B-续1无码实付36.72-实租2天-应补0.64", func(t *testing.T) {
		segs := []PaidSegment{enoSeg("contract"), fullSeg("renewal"), enoSeg("renewal")}
		res, err := computeDiscountedSettlement(2, segs, shipping)
		require.NoError(t, err)
		require.Equal(t, cents(36.36), res.DiscountedRent, "折后租金 0.36+36.00")
		require.Equal(t, cents(37.36), res.DiscountedDue, "折后应收 37.36")
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(0.64), res.NetAmount, "补缴 37.36−36.72=0.64")
		require.Equal(t, []int{1, 1, 0}, res.Usage)
	})

	t.Run("无码基线-三段原价36.72实付-实租1天-应退71", func(t *testing.T) {
		segs := []PaidSegment{fullSeg("contract"), fullSeg("renewal"), fullSeg("renewal")}
		res, err := computeDiscountedSettlement(1, segs, shipping)
		require.NoError(t, err)
		require.Equal(t, cents(36), res.DiscountedRent)
		require.Equal(t, cents(37), res.DiscountedDue)
		require.Equal(t, "refund", res.NetDirection)
		require.Equal(t, cents(71), res.NetAmount, "应退 108−37=71")
		require.Equal(t, []int{1, 0, 0}, res.Usage)
	})

	t.Run("租期跨三段-三码-实租3天-无差额", func(t *testing.T) {
		segs := []PaidSegment{enoSeg("contract"), enoSeg("renewal"), enoSeg("renewal")}
		res, err := computeDiscountedSettlement(3, segs, shipping)
		require.NoError(t, err)
		require.Equal(t, cents(1.08), res.DiscountedRent)
		require.Equal(t, cents(2.08), res.DiscountedDue)
		require.Equal(t, "shortfall", res.NetDirection)
		require.Equal(t, cents(1), res.NetAmount, "补缴 2.08−1.08=1.00（物流费）")
		require.Equal(t, []int{1, 1, 1}, res.Usage)
	})

	t.Run("实租天数超过覆盖段-应报错", func(t *testing.T) {
		segs := []PaidSegment{enoSeg("contract"), enoSeg("renewal"), enoSeg("renewal")}
		_, err := computeDiscountedSettlement(4, segs, shipping)
		require.Error(t, err)
	})

	t.Run("段实付超过原价-数据异常-应报错", func(t *testing.T) {
		segs := []PaidSegment{{Kind: "contract", Days: 1, OriginalTotal: cents(36), PaidTotal: cents(100)}}
		_, err := computeDiscountedSettlement(1, segs, shipping)
		require.Error(t, err)
	})

	t.Run("零段且无实租-空结果-应退0", func(t *testing.T) {
		res, err := computeDiscountedSettlement(0, nil, 0)
		require.NoError(t, err)
		require.Equal(t, "none", res.NetDirection)
		require.Equal(t, int64(0), res.NetAmount)
	})
}

var errSegmentModelNotImplemented = errors.New("segment model not implemented")
