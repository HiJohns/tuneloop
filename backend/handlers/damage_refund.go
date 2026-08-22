package handlers

import (
	"encoding/json"
	"math"

	"gorm.io/gorm"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// computeDamageRefund 计算定损场景的应退/应补金额（#1724 单点公式）：
//
//	refund = max(0, paidTotal − damage − actualRent − shippingFee)
//
// damage > refund → 需补缴 (damage − refund)；damage < refund → 应退还 (refund − damage)。
// GetOrder（定损面板预览）与 AgreeDamage（接受定损）共用，防止公式漂移。
//
// actualRent 推导：settlement → pricing_breakdown（actual_rent_*，P3 起为分）→
// delivered_at→returned_at 天数 × base_daily_rent（JSONB 分）。
func computeDamageRefund(db *gorm.DB, order models.Order, damageYuan float64) (refund float64, actualRent float64, paidTotal float64) {
	// paidTotal = 全部 paid 支付记录（元）
	var paidRecords []models.OrderPaymentRecord
	db.Where("order_id = ? AND status = ? AND type = ?", order.ID, "paid", "payment").
		Find(&paidRecords)
	for _, pr := range paidRecords {
		paidTotal += pr.Amount.ToYuan()
	}

	// actualRent：settlement 优先
	actualRentDays := 0
	var settlement models.Settlement
	if err := db.Where("order_id = ?", order.ID).Order("created_at DESC").First(&settlement).Error; err == nil {
		actualRentDays = settlement.ActualRentDays
		actualRent = settlement.ActualRentAmount.ToYuan()
	}
	var pb map[string]interface{}
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		json.Unmarshal([]byte(*order.PricingBreakdown), &pb)
	}
	if actualRentDays == 0 && actualRent == 0 && pb != nil {
		if v, ok := pb["actual_rent_days"].(float64); ok {
			actualRentDays = int(v)
		}
		if v, ok := pb["actual_rent_amount"].(float64); ok {
			actualRent = v / 100 // JSONB 为分（P3）
		}
	}
	if actualRentDays == 0 && actualRent == 0 {
		if order.DeliveredAt != nil && order.ReturnedAt != nil {
			// #1738 P3: same lease-day rule as settlement money math.
			days := services.CalculateLeaseDays(*order.DeliveredAt, *order.ReturnedAt)
			if v, ok := pb["base_daily_rent"].(float64); ok && v > 0 {
				// #1734: 统一分语义（存量元残留归一），再 /100 得元。
				bdr := resolveBaseDailyRentCents(db, &order, v)
				actualRent = math.Round(bdr/100*float64(days)*100) / 100
			}
		}
	}

	refund = math.Round((paidTotal-damageYuan-actualRent-order.ShippingFee.ToYuan())*100) / 100
	if refund < 0 {
		refund = 0
	}
	return refund, actualRent, paidTotal
}
