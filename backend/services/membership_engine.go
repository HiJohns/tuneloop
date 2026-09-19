package services

import (
	"log"

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
		// #1939 Sub-C（M-08）：晋升前违约校验——存在任一未结违约即阻止晋升
		//（口径 = 当前未结，结清/归还后解除可恢复，与 docs/cases/membership.md 一致）。
		if blocked, reason := hasBlockingDefault(&user, db); blocked {
			log.Printf("[membership] upgrade blocked for user %s: %s", user.ID, reason)
			return nil
		}
		if err := db.Model(&user).Update("membership_level_id", newLevelID).Error; err != nil {
			return err
		}
	}
	return nil
}

// hasBlockingDefault 晋升违约校验（M-08 四项，任一命中即阻止晋升）：
//
//  1. 逾期未归还       orders.status='expired'
//  2. 补缴未支付       order_payment_records.status='pending'
//     AND order_type IN ('damage','repair','loss')
//     （repair 含维修服务 dispatch 生成的补缴记录，user_id 为 IAM sub）
//  3. 损坏流程未结     orders.status IN ('pending_damage_response','damage_appealing')
//  4. 申诉未决         appeals.status IN ('pending','reviewing','forwarded')
//     （appellant_id 以 GetUserID 即 IAM sub 落库）
func hasBlockingDefault(user *models.User, db *gorm.DB) (bool, string) {
	var n int64

	db.Model(&models.Order{}).Where("user_id = ? AND status = ?", user.ID, "expired").Count(&n)
	if n > 0 {
		return true, "存在逾期未归还订单（orders.status=expired）"
	}

	db.Table("order_payment_records").
		Where("user_id = ? AND status = ? AND order_type IN ?", user.IAMSub, "pending",
			[]string{"damage", "repair", "loss"}).
		Count(&n)
	if n > 0 {
		return true, "存在未支付补缴记录（damage/repair/loss pending）"
	}

	db.Model(&models.Order{}).
		Where("user_id = ? AND status IN ?", user.ID,
			[]string{"pending_damage_response", "damage_appealing"}).
		Count(&n)
	if n > 0 {
		return true, "存在未结损坏流程（pending_damage_response/damage_appealing）"
	}

	db.Table("appeals").
		Where("appellant_id = ? AND status IN ?", user.IAMSub,
			[]string{"pending", "reviewing", "forwarded"}).
		Count(&n)
	if n > 0 {
		return true, "存在未决申诉（pending/reviewing/forwarded）"
	}

	return false, ""
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
// aggregateUserSpending 汇总用户累计消费（#1727 P2 起 DB 金额列为 BIGINT 分，
// SQL SUM 直接返回分——返回 Cents 与 membership_levels.min_amount 同单位比较）。
func aggregateUserSpending(userID string, db *gorm.DB) models.Cents {
	var purchaseTotal int64
	db.Raw(`SELECT COALESCE(SUM(p.amount),0) FROM order_payment_records p
		JOIN users u ON p.user_id::text = u.iam_sub
		WHERE u.id = ? AND p.type = 'payment' AND p.status = 'paid'
		AND p.order_type IN ('points','renewal','repair')`, userID).Scan(&purchaseTotal)

	var rentTotal int64
	db.Raw(`SELECT COALESCE(SUM(s.actual_rent_amount),0) FROM settlements s
		JOIN orders o ON o.id = s.order_id
		WHERE o.user_id = ?`, userID).Scan(&rentTotal)

	var refundTotal int64
	db.Raw(`SELECT COALESCE(SUM(rf.amount),0) FROM order_refund_records rf
		JOIN order_payment_records p ON p.id = rf.payment_record_id
		JOIN users u ON p.user_id::text = u.iam_sub
		WHERE u.id = ? AND rf.status = 'refunded'`, userID).Scan(&refundTotal)

	total := models.Cents(purchaseTotal + rentTotal - refundTotal)
	if total < 0 {
		total = 0
	}
	return total
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
