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

// #2282 P4a：按版本读取（无版本 → 旧单 fallback 由调用方处理）。
func LoadTermVersions(db *gorm.DB, orderID string) ([]models.OrderTermVersion, error) {
	var versions []models.OrderTermVersion
	err := db.Where("order_id = ?", orderID).Order("version ASC").Find(&versions).Error
	return versions, err
}

// termVersionTotals 返回版本覆盖天数与合计金额（分）；无版本 ok=false。
func termVersionTotals(db *gorm.DB, orderID string) (days int, total models.Cents, ok bool) {
	versions, err := LoadTermVersions(db, orderID)
	if err != nil || len(versions) == 0 {
		return 0, 0, false
	}
	for _, v := range versions {
		days += v.Days
		total += v.TotalAmount
	}
	return days, total, days > 0
}
