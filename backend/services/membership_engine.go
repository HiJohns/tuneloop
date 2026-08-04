package services

import (
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

func CheckAndUpgradeLevel(userID string, db *gorm.DB) error {
	if db == nil {
		db = database.GetDB()
	}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	var levels []models.MembershipLevel
	if err := db.Order("id ASC").Find(&levels).Error; err != nil {
		return err
	}
	totalSpending := aggregateUserSpending(userID, db)
	newLevelID := 1
	for _, l := range levels {
		if totalSpending >= l.MinAmount {
			newLevelID = l.ID
		}
	}
	if user.MembershipLevelID == nil || *user.MembershipLevelID < newLevelID {
		if err := db.Model(&user).Update("membership_level_id", newLevelID).Error; err != nil {
			return err
		}
	}
	return nil
}

// aggregateUserSpending computes lifetime spending on demand from payment
// records, settled rent and refunds:
//
//	total = Σ(prepaid/renewal/repair payments)
//	      + Σ(settlements.actual_rent_amount)
//	      - Σ(refunded refund records)
//
// order_refund_records has no user_id column, so it joins through
// order_payment_records via payment_record_id.
func aggregateUserSpending(userID string, db *gorm.DB) float64 {
	var purchaseTotal float64
	db.Raw(`SELECT COALESCE(SUM(amount),0) FROM order_payment_records
		WHERE user_id = ? AND type = 'payment' AND status = 'paid'
		AND order_type IN ('points','renewal','repair')`, userID).Scan(&purchaseTotal)

	var rentTotal float64
	db.Raw(`SELECT COALESCE(SUM(s.actual_rent_amount),0) FROM settlements s
		JOIN orders o ON o.id = s.order_id
		WHERE o.user_id = ?`, userID).Scan(&rentTotal)

	var refundTotal float64
	db.Raw(`SELECT COALESCE(SUM(rf.amount),0) FROM order_refund_records rf
		JOIN order_payment_records p ON p.id = rf.payment_record_id
		WHERE p.user_id = ? AND rf.status = 'refunded'`, userID).Scan(&refundTotal)

	total := purchaseTotal + rentTotal - refundTotal
	if total < 0 {
		total = 0
	}
	return total
}
