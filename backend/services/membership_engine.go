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
	// Callers pass either local users.id or IAM subject (order_payment_records.user_id
	// stores the IAM subject) — resolve both.
	var user models.User
	if err := db.Where("id = ? OR iam_sub = ?", userID, userID).First(&user).Error; err != nil {
		return err
	}
	var levels []models.MembershipLevel
	if err := db.Order("id ASC").Find(&levels).Error; err != nil {
		return err
	}
	totalSpending := aggregateUserSpending(user.ID, db)
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
// order_payment_records.user_id stores the IAM subject, so payments are
// joined through users (u.iam_sub = p.user_id) to match the local user.
// order_refund_records has no user_id column, so it joins through
// order_payment_records via payment_record_id.
func aggregateUserSpending(userID string, db *gorm.DB) float64 {
	var purchaseTotal float64
	db.Raw(`SELECT COALESCE(SUM(p.amount),0) FROM order_payment_records p
		JOIN users u ON p.user_id::text = u.iam_sub
		WHERE u.id = ? AND p.type = 'payment' AND p.status = 'paid'
		AND p.order_type IN ('points','renewal','repair')`, userID).Scan(&purchaseTotal)

	var rentTotal float64
	db.Raw(`SELECT COALESCE(SUM(s.actual_rent_amount),0) FROM settlements s
		JOIN orders o ON o.id = s.order_id
		WHERE o.user_id = ?`, userID).Scan(&rentTotal)

	var refundTotal float64
	db.Raw(`SELECT COALESCE(SUM(rf.amount),0) FROM order_refund_records rf
		JOIN order_payment_records p ON p.id = rf.payment_record_id
		JOIN users u ON p.user_id::text = u.iam_sub
		WHERE u.id = ? AND rf.status = 'refunded'`, userID).Scan(&refundTotal)

	total := purchaseTotal + rentTotal - refundTotal
	if total < 0 {
		total = 0
	}
	return total
}

// GetGiftRatios returns the membership gift ratio config for a level,
// or nil if none is configured/active (#1536, #1542).
func GetGiftRatios(levelID int) *models.MembershipGiftRatio {
	if levelID <= 0 {
		return nil
	}
	db := database.GetDB()
	var r models.MembershipGiftRatio
	if err := db.Where("level_id = ? AND is_active = ?", levelID, true).First(&r).Error; err != nil {
		return nil
	}
	return &r
}

// GetGiftPolicyByLevel returns the gift policy for a membership level,
// falling back to the default row (level_id=0) when the level has no
// active policy (#1605, L-05). Returns nil only when even the default
// row is missing.
func GetGiftPolicyByLevel(db *gorm.DB, levelID int) *models.GiftPolicy {
	if db == nil {
		db = database.GetDB()
	}
	var p models.GiftPolicy
	if levelID > 0 {
		if err := db.Where("level_id = ? AND is_active = ?", levelID, true).First(&p).Error; err == nil {
			return &p
		}
	}
	if err := db.Where("level_id = 0 AND is_active = ?", true).First(&p).Error; err == nil {
		return &p
	}
	return nil
}

// FindReferrer looks up the referrer of a user via the referrals table.
// Returns the referrer's User record (with MembershipLevelID) or nil.
func FindReferrer(userID string) *models.User {
	if userID == "" {
		return nil
	}
	db := database.GetDB()
	var referral models.Referral
	if err := db.Where("referee_id = ?", userID).First(&referral).Error; err != nil {
		return nil
	}
	var referrer models.User
	if err := db.Where("id = ?", referral.ReferrerID).First(&referrer).Error; err != nil {
		return nil
	}
	return &referrer
}
