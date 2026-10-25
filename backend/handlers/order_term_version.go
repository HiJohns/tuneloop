package handlers

// #2282 P4a：订单创建时写入不可变期限版本（version=1）。

import (
	"encoding/json"
	"log"

	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"gorm.io/gorm"
)

// snapshotReferralLevel 下单时记录推荐人会员等级（#2282 P4c）。
func snapshotReferralLevel(order *models.Order) {
	if order == nil || order.UserID == "" {
		return
	}
	if ref := services.FindReferrer(order.UserID); ref != nil && ref.MembershipLevelID != nil {
		order.ReferralLevelID = ref.MembershipLevelID
	}
}

func insertInitialTermVersion(tx *gorm.DB, order *models.Order) {
	if order == nil || order.PricingBreakdown == nil || *order.PricingBreakdown == "" {
		return
	}
	var pb services.PricingBreakdown
	if err := json.Unmarshal([]byte(*order.PricingBreakdown), &pb); err != nil {
		log.Printf("[TermVersion] parse pricing_breakdown failed for order %s: %v", order.ID, err)
		return
	}
	days := order.LeaseTerm
	if pb.RentDays > 0 {
		days = pb.RentDays
	}
	seg, err := json.Marshal(pb.TierSegments)
	if err != nil {
		seg = []byte("[]")
	}
	v := models.OrderTermVersion{
		OrderID:      order.ID,
		Version:      1,
		StartDate:    order.StartDate,
		EndDate:      order.EndDate,
		Days:         days,
		TierSegments: string(seg),
		TotalAmount:  models.FromYuan(pb.TotalAmount),
	}
	if err := tx.Create(&v).Error; err != nil {
		log.Printf("[TermVersion] insert initial version failed for order %s: %v", order.ID, err)
	}
}
