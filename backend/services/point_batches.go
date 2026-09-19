package services

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"tuneloop-backend/models"
)

// 批次来源（#1947 Sub-D）。
const (
	PointBatchSourceSignup    = "signup"
	PointBatchSourceReferral  = "referral"
	PointBatchSourceFission   = "fission"
	PointBatchSourcePurchase  = "purchase"
	PointBatchSourceActivity  = "activity"
	PointBatchSourceMigration = "migration"
)

// PointBatchValidity 乐币批次有效期。默认 2 年，包级可配置（#1939 配置化准则）——
// Sub-B/后台配置接入时通过 SetPointBatchValidity 覆盖。
var PointBatchValidity = 2 * 365 * 24 * time.Hour

// PointBatchReminderLead 到期提醒提前量（默认 30 天）。
var PointBatchReminderLead = 30 * 24 * time.Hour

// SetPointBatchValidity 覆盖批次有效期（配置化入口；<=0 时回退默认 2 年）。
func SetPointBatchValidity(validity time.Duration) {
	if validity > 0 {
		PointBatchValidity = validity
	}
}

// SetPointBatchReminderLead 覆盖到期提醒提前量（配置化入口；<=0 时回退默认 30 天）。
func SetPointBatchReminderLead(lead time.Duration) {
	if lead > 0 {
		PointBatchReminderLead = lead
	}
}

// 全局「乐币规则」参数在 system_settings 中的键（#1945 Sub-B §四）。
const (
	SettingPayRatioMax              = "pay_ratio_max"                // 抵扣比例上限（默认 1.0）
	SettingPointBatchValidityMonths = "point_batch_validity_months"  // 乐币有效期（月，默认 24）
	SettingPointExpiryReminderDays  = "point_expiry_reminder_days"   // 到期提醒提前量（天，默认 30）
)

// GetGlobalFloatSetting 读取全局租户（nil UUID）下的 system_settings 数值，缺失/非法返回 def。
// 注意：调用方应传入已清除租户作用域的 DB（见 ApplyPointBatchSettingsFromDB）。
func GetGlobalFloatSetting(db *gorm.DB, key string, def float64) float64 {
	if db == nil {
		return def
	}
	var s models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", "00000000-0000-0000-0000-000000000000", key).
		First(&s).Error; err == nil {
		if v, perr := strconv.ParseFloat(s.SettingValue, 64); perr == nil {
			return v
		}
	}
	return def
}

// ApplyPointBatchSettings 将「乐币规则」全局参数应用到运行时（启动与配置更新时调用）。
// validityMonths<=0 回退默认；reminderDays<=0 回退默认。
func ApplyPointBatchSettings(validityMonths, reminderDays int) {
	if validityMonths > 0 {
		SetPointBatchValidity(time.Duration(validityMonths) * 30 * 24 * time.Hour)
	}
	if reminderDays > 0 {
		SetPointBatchReminderLead(time.Duration(reminderDays) * 24 * time.Hour)
	}
}

// ApplyPointBatchSettingsFromDB 从 system_settings 读取全局参数并应用（缺失取默认）。
func ApplyPointBatchSettingsFromDB(db *gorm.DB) {
	if db == nil {
		return
	}
	validityMonths := GetGlobalFloatSetting(db, SettingPointBatchValidityMonths, 24)
	reminderDays := GetGlobalFloatSetting(db, SettingPointExpiryReminderDays, 30)
	ApplyPointBatchSettings(int(validityMonths), int(reminderDays))
}

// pointBatchLocation 归一化/清扫使用的时区（北京时间）。
var pointBatchLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}()

// normalizeBatchExpiry 归一化到期时间：acquired_at + 有效期 之后的「次月首日 00:00（北京时间）」。
// 例：2026-09-10 获取、有效期 2 年 → 2028-09-10 → 2028-10-01 00:00 CST。
// 归一化对任意有效期长度均生效（便于凌晨统一清扫 + 合并通知）。
func normalizeBatchExpiry(acquired time.Time) time.Time {
	t := acquired.In(pointBatchLocation).Add(PointBatchValidity)
	y, m, _ := t.Date()
	return time.Date(y, m+1, 1, 0, 0, 0, 0, pointBatchLocation)
}

// CreatePointsBatch 建批次（仅建账，不同步 users.promo_points 快照）。
// expires_at 统一按 normalizeBatchExpiry 归一化（含 migration 批次）。
func CreatePointsBatch(tx *gorm.DB, userID, sourceType, sourceRef string, amountCents models.Cents) (*models.PointBatch, error) {
	if tx == nil || userID == "" || amountCents <= 0 {
		return nil, nil
	}
	now := time.Now()
	expiresAt := normalizeBatchExpiry(now)
	b := models.PointBatch{
		ID:             uuid.New().String(),
		UserID:         userID,
		SourceType:     sourceType,
		SourceRef:      sourceRef,
		AmountCents:    amountCents,
		RemainingCents: amountCents,
		AcquiredAt:     now,
		ExpiresAt:      &expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tx.Create(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// ConsumePointsFIFO 按 FIFO（先到期先扣，同到期按获取时间，NULL 视为最晚）逐批次扣减，
// 写 point_batch_consumptions 留痕，返回实际消费额（可能 < amountCents，无批次时为 0）。
func ConsumePointsFIFO(tx *gorm.DB, userID string, amountCents models.Cents, transactionID string) (models.Cents, error) {
	if tx == nil || userID == "" || amountCents <= 0 {
		return 0, nil
	}
	var batches []models.PointBatch
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND remaining_cents > 0", userID).
		Order("expires_at ASC NULLS LAST, acquired_at ASC, id ASC").
		Find(&batches).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	var consumed models.Cents
	for i := range batches {
		if consumed >= amountCents {
			break
		}
		need := amountCents - consumed
		take := batches[i].RemainingCents
		if take > need {
			take = need
		}
		if take <= 0 {
			continue
		}
		if err := tx.Model(&models.PointBatch{}).Where("id = ?", batches[i].ID).
			Update("remaining_cents", gorm.Expr("remaining_cents - ?", take)).Error; err != nil {
			return consumed, err
		}
		c := models.PointBatchConsumption{
			ID:            uuid.New().String(),
			TransactionID: transactionID,
			BatchID:       batches[i].ID,
			AmountCents:   take,
			ConsumedAt:    now,
		}
		if err := tx.Create(&c).Error; err != nil {
			return consumed, err
		}
		consumed += take
	}
	return consumed, nil
}

// RestorePointsByTransaction 按消费逆序恢复原批次 remaining（保留原到期日）。
// 恢复额 ≤ 该 transaction 的未恢复消费合计；已恢复部分从留痕中扣减（幂等）。
func RestorePointsByTransaction(tx *gorm.DB, userID, transactionID string, amountCents models.Cents) (models.Cents, error) {
	if tx == nil || userID == "" || transactionID == "" || amountCents <= 0 {
		return 0, nil
	}
	var cons []models.PointBatchConsumption
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("transaction_id = ? AND amount_cents > 0", transactionID).
		Order("consumed_at DESC, id DESC").Find(&cons).Error; err != nil {
		return 0, err
	}
	var restored models.Cents
	for _, cx := range cons {
		if restored >= amountCents {
			break
		}
		need := amountCents - restored
		give := cx.AmountCents
		if give > need {
			give = need
		}
		res := tx.Model(&models.PointBatch{}).
			Where("id = ? AND user_id = ?", cx.BatchID, userID).
			Update("remaining_cents", gorm.Expr("remaining_cents + ?", give))
		if res.Error != nil {
			return restored, res.Error
		}
		if res.RowsAffected == 0 {
			continue
		}
		if err := tx.Model(&models.PointBatchConsumption{}).Where("id = ?", cx.ID).
			Update("amount_cents", gorm.Expr("amount_cents - ?", give)).Error; err != nil {
			return restored, err
		}
		restored += give
	}
	return restored, nil
}

// ExpireDueBatches 过期清扫：remaining>0 且 expires_at<now → 置 expired_cents/expired_at、
// 写 points_transactions(type='expired')、同步扣减 users.promo_points 快照。
// 返回 (处理批次次数, 过期总面额)。
func ExpireDueBatches(db *gorm.DB) (int, models.Cents, error) {
	if db == nil {
		return 0, 0, nil
	}
	now := time.Now()
	var batches []models.PointBatch
	if err := db.Where("remaining_cents > 0 AND expires_at IS NOT NULL AND expires_at < ?", now).
		Find(&batches).Error; err != nil {
		return 0, 0, err
	}
	count := 0
	var total models.Cents
	expiredByUser := map[string]models.Cents{}
	tenantByUser := map[string]string{}
	orgByUser := map[string]string{}
	for _, b := range batches {
		var batchExp models.Cents
		var u models.User
		err := db.Transaction(func(tx *gorm.DB) error {
			batchExp = 0
			var fresh models.PointBatch
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", b.ID).First(&fresh).Error; err != nil {
				return err
			}
			if fresh.RemainingCents <= 0 {
				return nil
			}
			exp := fresh.RemainingCents
			if err := tx.Model(&models.PointBatch{}).Where("id = ?", fresh.ID).Updates(map[string]interface{}{
				"remaining_cents": 0,
				"expired_cents":   gorm.Expr("expired_cents + ?", exp),
				"expired_at":      now,
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
			// #1983 阶段 2：不再同步扣减 users.promo_points 快照；批次 remaining 已清零，
			// 余额 SUM(未过期 remaining) 自然减少。
			if err := tx.Where("id = ?", fresh.UserID).First(&u).Error; err != nil {
				return err
			}
			pt := models.PointsTransaction{
				ID:          uuid.New().String(),
				UserID:      fresh.UserID,
				TenantID:    u.TenantID,
				Type:        "expired",
				Amount:      models.Cents(-exp),
				Description: fmt.Sprintf("乐币到期失效: %.2f", float64(exp)/100),
				CreatedAt:   now,
			}
			if err := tx.Create(&pt).Error; err != nil {
				return err
			}
			batchExp = exp
			return nil
		})
		if err != nil {
			log.Printf("[PointBatches] expire batch %s failed: %v", b.ID, err)
			continue
		}
		if batchExp <= 0 {
			continue
		}
		count++
		total += batchExp
		expiredByUser[b.UserID] += batchExp
		tenantByUser[b.UserID] = u.TenantID
		orgByUser[b.UserID] = u.OrgID
	}

	// 按用户合并一条过期通知（B 条裁定：凌晨统一清扫后，同一用户只发一条汇总）
	for userID, exp := range expiredByUser {
		if exp <= 0 {
			continue
		}
		notif := models.Notification{
			ID:         uuid.New().String(),
			TenantID:   tenantByUser[userID],
			OrgID:      orgByUser[userID],
			UserID:     userID,
			Type:       "points_expired",
			Title:      "乐币已过期",
			Content:    fmt.Sprintf("您有 %.2f 乐币因到期未使用已失效", float64(exp)/100),
			RefID:      userID,
			RefType:    "user",
			ActionType: "points_expired",
			Status:     "unread",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[PointBatches] expired notification for user %s failed: %v", userID, err)
		}
	}
	return count, total, nil
}

// RemindExpiringBatches 到期前 PointBatchReminderLead 内提醒（幂等：以 batch id + 类型查重）。
func RemindExpiringBatches(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, nil
	}
	now := time.Now()
	deadline := now.Add(PointBatchReminderLead)
	var batches []models.PointBatch
	if err := db.Where("remaining_cents > 0 AND expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ?", now, deadline).
		Find(&batches).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, b := range batches {
		var exists int64
		db.Model(&models.Notification{}).
			Where("type = ? AND ref_type = ? AND ref_id = ?", "points_expiring", "point_batch", b.ID).
			Count(&exists)
		if exists > 0 {
			continue
		}
		var u models.User
		if err := db.Where("id = ?", b.UserID).First(&u).Error; err != nil {
			continue
		}
		notif := models.Notification{
			ID:         uuid.New().String(),
			TenantID:   u.TenantID,
			OrgID:      u.OrgID,
			UserID:     b.UserID,
			Type:       "points_expiring",
			Title:      "乐币即将到期",
			Content:    fmt.Sprintf("您有 %.2f 乐币将于 %s 到期，请及时使用", float64(b.RemainingCents)/100, b.ExpiresAt.Format("2006-01-02")),
			RefID:      b.ID,
			RefType:    "point_batch",
			ActionType: "points_expiring",
			Status:     "unread",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[PointBatches] reminder for batch %s failed: %v", b.ID, err)
			continue
		}
		count++
	}
	return count, nil
}

// GetUserPointsBalance 返回用户可用乐币余额 = 未过期批次 remaining 之和。
// #1983 阶段 2：批次 SUM 是余额唯一真源（users.promo_points 快照已废弃）。
func GetUserPointsBalance(db *gorm.DB, userID string) (models.Cents, error) {
	if db == nil || userID == "" {
		return 0, nil
	}
	var sum struct {
		Total int64
	}
	if err := db.Model(&models.PointBatch{}).
		Where("user_id = ? AND remaining_cents > 0 AND (expires_at IS NULL OR expires_at >= ?)", userID, time.Now()).
		Select("COALESCE(SUM(remaining_cents), 0) AS total").
		Scan(&sum).Error; err != nil {
		return 0, err
	}
	return models.Cents(sum.Total), nil
}
