package handlers

import (
	"math"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// resolveBaseDailyRentCents 把订单快照 pricing_breakdown.base_daily_rent
// 解析为「分」语义（1 元 = 100 分）。
//
// 背景（#1734）：P2 cents 迁移（20260820001）的 moneyKeys 白名单遗漏
// base_daily_rent → 存量订单快照该字段为**元**语义（如 28 元/36 元，高价
// 乐器甚至 205 元），而同快照 total_amount/tier_segments.rate 已是分；
// P3 后新订单为分语义（如 3600）。读取方（renewal/damage_refund/
// user_settlement/order）对 bdr 的单位假定互相矛盾，导致金额少算/多算
// 100 倍。
//
// 判定策略（乐器现价权威最近邻，固定阈值对高价乐器失效——CB-111 205 元
// 残留被 <100 阈值漏判）：bdr×100 与 bdr 哪个更接近乐器当前日租（分），
// 更接近 ×100 者 → 元语义残留，归一为分；否则按分原样返回。乐器现价
// 不可用时按分返回（避免放大错误，存量由 T2 数据修复兜底）。
func resolveBaseDailyRentCents(db *gorm.DB, order *models.Order, bdr float64) float64 {
	if bdr <= 0 {
		return 0
	}
	if order != nil && order.InstrumentID != "" && db != nil {
		var inst models.Instrument
		if err := db.Select("base_daily_rate").Where("id = ?", order.InstrumentID).First(&inst).Error; err == nil && inst.BaseDailyRate != nil {
			cur := float64(*inst.BaseDailyRate)
			distCents := math.Abs(bdr - cur)
			distYuan := math.Abs(bdr*100 - cur)
			if distYuan < distCents {
				// bdr×100 更接近现价 → 快照为元语义残留，归一为分。
				return math.Round(bdr * 100)
			}
			// bdr 本身更接近现价 → 分语义。
			return bdr
		}
	}
	// 无乐器/现价不可用——按分原样返回，避免放大错误。
	return bdr
}
