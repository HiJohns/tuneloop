package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// globalTenantID system_settings 中承载全局配置的租户（与 UpsertGlobalSetting 一致）。
const globalTenantID = "00000000-0000-0000-0000-000000000000"

// globalSettingsDB 返回已清除租户作用域的 DB，用于读写全局（nil UUID）配置。
// registerTenantCallbacks 的 addTenantScope 会注入真实 tenant_id，导致全局查询永假。
func globalSettingsDB(ctx context.Context) *gorm.DB {
	return database.GetDB().WithContext(database.SetTenantID(ctx, ""))
}

// getFloatSetting 读取全局 system_settings 数值，缺失/非法时返回 def。
func getFloatSetting(ctx context.Context, key string, def float64) float64 {
	return services.GetGlobalFloatSetting(globalSettingsDB(ctx), key, def)
}

// upsertGlobalSetting 幂等写入全局（nil UUID）配置。
func upsertGlobalSetting(ctx context.Context, key, value string) error {
	setting := models.SystemSetting{
		TenantID:     globalTenantID,
		SettingKey:   key,
		SettingValue: value,
		UpdatedBy:    middleware.GetUserID(ctx),
		UpdatedAt:    time.Now(),
	}
	return globalSettingsDB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "setting_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"setting_value", "updated_by", "updated_at"}),
	}).Create(&setting).Error
}

// ListGiftPolicies lists gift policies for all membership levels,
// including placeholder rows for levels without a policy (#1605, L-05).
// level_id=0 is the default fallback row.
// #1945: exposes referral_ratio / referral_reg_points; refund_ratio removed.
func ListGiftPolicies(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var policies []models.GiftPolicy
	if err := db.Order("level_id ASC").Find(&policies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}

	// Merge with membership levels to surface placeholder rows
	type levelRow struct {
		LevelID int    `json:"level_id"`
		Name    string `json:"name"`
	}
	var levels []levelRow
	if err := db.Table("membership_levels").Select("id AS level_id, name").Order("id ASC").Scan(&levels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}

	byLevel := make(map[int]models.GiftPolicy, len(policies))
	for _, p := range policies {
		byLevel[p.LevelID] = p
	}

	// #1900: levels without their own row run on the level-0 fallback at
	// runtime; surface the effective values (plus is_fallback).
	def, hasDef := byLevel[0]
	fallbackPay, fallbackReferral, fallbackRegPoints, fallbackActive := 0.0, 0.0, 0.0, false
	if hasDef {
		fallbackPay, fallbackReferral, fallbackRegPoints, fallbackActive = def.PayRatio, def.ReferralRatio, def.ReferralRegPoints, def.IsActive
	}

	result := make([]gin.H, 0, len(levels)+1)
	if hasDef {
		result = append(result, gin.H{
			"level_id":            0,
			"name":                "默认（未设置级别）",
			"pay_ratio":           def.PayRatio,
			"referral_ratio":      def.ReferralRatio,
			"referral_reg_points": def.ReferralRegPoints,
			"is_active":           def.IsActive,
			"is_fallback":         false,
		})
	}
	for _, lv := range levels {
		entry := gin.H{
			"level_id":            lv.LevelID,
			"name":                lv.Name,
			"pay_ratio":           fallbackPay,
			"referral_ratio":      fallbackReferral,
			"referral_reg_points": fallbackRegPoints,
			"is_active":           fallbackActive,
			"is_fallback":         true,
		}
		if p, ok := byLevel[lv.LevelID]; ok {
			entry["pay_ratio"] = p.PayRatio
			entry["referral_ratio"] = p.ReferralRatio
			entry["referral_reg_points"] = p.ReferralRegPoints
			entry["is_active"] = p.IsActive
			entry["is_fallback"] = false
		}
		result = append(result, entry)
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// UpdateGiftPolicy updates the gift policy for a membership level (#1605, L-05).
// Body: {level_id, pay_ratio?, referral_ratio?, referral_reg_points?, is_active?}
// #1945: pay_ratio 上限改为可配置（system_settings.pay_ratio_max，默认 1.0）；
// 新增 referral_ratio（0~1.0）与 referral_reg_points（>=0）；移除 refund_ratio。
func UpdateGiftPolicy(c *gin.Context) {
	var req struct {
		// #1944 Sub-A：level_id=0 是合法兜底行（binding:required 会把 0 当缺省拒绝）
		LevelID           int      `json:"level_id"`
		PayRatio          *float64 `json:"pay_ratio"`
		ReferralRatio     *float64 `json:"referral_ratio"`
		ReferralRegPoints *float64 `json:"referral_reg_points"`
		IsActive          *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	// #1944 Sub-A：负数 level_id 非法（0=兜底行，正数=会员等级）
	if req.LevelID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "level_id 不能为负数"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// #1945：抵扣上限可配置（默认 100%），并夹取到 [0,1]。
	payRatioMax := getFloatSetting(ctx, services.SettingPayRatioMax, 1.0)
	if payRatioMax < 0 {
		payRatioMax = 0
	}
	if payRatioMax > 1 {
		payRatioMax = 1
	}

	if req.PayRatio != nil && (*req.PayRatio < 0 || *req.PayRatio > payRatioMax) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "pay_ratio 必须在 0~" + strconv.FormatFloat(payRatioMax, 'f', -1, 64) + " 之间（赠点最多抵扣应付金额的 " + strconv.FormatFloat(payRatioMax*100, 'f', -1, 64) + "%）",
		})
		return
	}
	if req.ReferralRatio != nil && (*req.ReferralRatio < 0 || *req.ReferralRatio > 1.0) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "referral_ratio 必须在 0~1.0 之间"})
		return
	}
	if req.ReferralRegPoints != nil && (*req.ReferralRegPoints < 0 || *req.ReferralRegPoints > 100000) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "referral_reg_points 必须在 0~100000 之间"})
		return
	}

	// Ensure a row exists (level_id=0 allowed as default row)
	var count int64
	db.Model(&models.GiftPolicy{}).Where("level_id = ?", req.LevelID).Count(&count)
	if count == 0 {
		row := models.GiftPolicy{LevelID: req.LevelID}
		if err := db.Create(&row).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}

	updates := map[string]interface{}{}
	if req.PayRatio != nil {
		updates["pay_ratio"] = *req.PayRatio
	}
	if req.ReferralRatio != nil {
		updates["referral_ratio"] = *req.ReferralRatio
	}
	if req.ReferralRegPoints != nil {
		updates["referral_reg_points"] = *req.ReferralRegPoints
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if err := db.Model(&models.GiftPolicy{}).Where("level_id = ?", req.LevelID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

// GetPointSettings 读取「乐币规则」全局参数（#1945 Sub-B §四）。
// GET /api/admin/point-settings
func GetPointSettings(c *gin.Context) {
	ctx := c.Request.Context()
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"pay_ratio_max":               getFloatSetting(ctx, services.SettingPayRatioMax, 1.0),
		"point_batch_validity_months": getFloatSetting(ctx, services.SettingPointBatchValidityMonths, 24),
		"point_expiry_reminder_days":  getFloatSetting(ctx, services.SettingPointExpiryReminderDays, 30),
	}})
}

// UpdatePointSettings 更新「乐币规则」全局参数（#1945 Sub-B §四）。
// PUT /api/admin/point-settings
// Body: {pay_ratio_max?, point_batch_validity_months?, point_expiry_reminder_days?}
func UpdatePointSettings(c *gin.Context) {
	var req struct {
		PayRatioMax              *float64 `json:"pay_ratio_max"`
		PointBatchValidityMonths *float64 `json:"point_batch_validity_months"`
		PointExpiryReminderDays  *float64 `json:"point_expiry_reminder_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if req.PayRatioMax != nil && (*req.PayRatioMax < 0 || *req.PayRatioMax > 1) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "pay_ratio_max 必须在 0~1 之间"})
		return
	}
	if req.PointBatchValidityMonths != nil && (*req.PointBatchValidityMonths < 1 || *req.PointBatchValidityMonths > 120) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "point_batch_validity_months 必须在 1~120 之间"})
		return
	}
	if req.PointExpiryReminderDays != nil && (*req.PointExpiryReminderDays < 0 || *req.PointExpiryReminderDays > 365) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "point_expiry_reminder_days 必须在 0~365 之间"})
		return
	}

	ctx := c.Request.Context()
	if req.PayRatioMax != nil {
		if err := upsertGlobalSetting(ctx, services.SettingPayRatioMax, strconv.FormatFloat(*req.PayRatioMax, 'f', -1, 64)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}
	if req.PointBatchValidityMonths != nil {
		if err := upsertGlobalSetting(ctx, services.SettingPointBatchValidityMonths, strconv.FormatFloat(*req.PointBatchValidityMonths, 'f', -1, 64)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}
	if req.PointExpiryReminderDays != nil {
		if err := upsertGlobalSetting(ctx, services.SettingPointExpiryReminderDays, strconv.FormatFloat(*req.PointExpiryReminderDays, 'f', -1, 64)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}

	// 立即生效：应用到运行时（无需重启）。
	services.ApplyPointBatchSettingsFromDB(globalSettingsDB(ctx))

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
