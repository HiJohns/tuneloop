package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// ListGiftPolicies lists gift policies for all membership levels,
// including placeholder rows for levels without a policy (#1605, L-05).
// level_id=0 is the default fallback row.
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

	result := make([]gin.H, 0, len(levels)+1)
	// Default row first (level_id=0)
	def, hasDef := byLevel[0]
	if hasDef {
		result = append(result, gin.H{
			"level_id":     0,
			"name":         "默认（未设置级别）",
			"pay_ratio":    def.PayRatio,
			"refund_ratio": def.RefundRatio,
			"is_active":    def.IsActive,
			"is_fallback":  false,
		})
	}
	// #1900: levels without their own row run on the level-0 fallback at
	// runtime; surface the effective values (plus is_fallback) so the admin
	// table matches actual behavior instead of showing 0/停用.
	fallbackPay, fallbackRefund, fallbackActive := 0.0, 0.0, false
	if hasDef {
		fallbackPay, fallbackRefund, fallbackActive = def.PayRatio, def.RefundRatio, def.IsActive
	}
	for _, lv := range levels {
		entry := gin.H{
			"level_id":     lv.LevelID,
			"name":         lv.Name,
			"pay_ratio":    fallbackPay,
			"refund_ratio": fallbackRefund,
			"is_active":    fallbackActive,
			"is_fallback":  true,
		}
		if p, ok := byLevel[lv.LevelID]; ok {
			entry["pay_ratio"] = p.PayRatio
			entry["refund_ratio"] = p.RefundRatio
			entry["is_active"] = p.IsActive
			entry["is_fallback"] = false
		}
		result = append(result, entry)
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// UpdateGiftPolicy updates the gift policy for a membership level (#1605, L-05).
// Body: {pay_ratio?, refund_ratio?, is_active?}
func UpdateGiftPolicy(c *gin.Context) {
	var req struct {
		LevelID     int      `json:"level_id" binding:"required"`
		PayRatio    *float64 `json:"pay_ratio"`
		RefundRatio *float64 `json:"refund_ratio"`
		IsActive    *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// Validate ratio bounds. #1900: business ceilings — 100% would allow
	// zero-pay rentals (pay_ratio) and unbounded point inflation (refund_ratio).
	if req.PayRatio != nil && (*req.PayRatio < 0 || *req.PayRatio > 0.5) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "pay_ratio 必须在 0~0.5 之间（赠点最多抵扣应付金额的 50%）"})
		return
	}
	if req.RefundRatio != nil && (*req.RefundRatio < 0 || *req.RefundRatio > 0.2) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "refund_ratio 必须在 0~0.2 之间（返点最多为实付现金的 20%）"})
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
	if req.RefundRatio != nil {
		updates["refund_ratio"] = *req.RefundRatio
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
