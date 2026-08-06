package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// GetGiftRatios lists all membership gift ratio configs (#1536).
func GetGiftRatios(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var configs []models.MembershipGiftRatio
	if err := db.Order("level_id ASC").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": configs})
}

// UpdateGiftRatios updates per-level gift ratios (#1536).
// Body: {level_id, self_spend_ratio?, referral_reg_points?, referral_spend_ratio?, is_active?}
func UpdateGiftRatios(c *gin.Context) {
	var req struct {
		LevelID            int      `json:"level_id" binding:"required"`
		SelfSpendRatio     *float64 `json:"self_spend_ratio"`
		ReferralRegPoints  *float64 `json:"referral_reg_points"`
		ReferralSpendRatio *float64 `json:"referral_spend_ratio"`
		IsActive           *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// Ensure a row exists for this level
	var count int64
	db.Model(&models.MembershipGiftRatio{}).Where("level_id = ?", req.LevelID).Count(&count)
	if count == 0 {
		row := models.MembershipGiftRatio{LevelID: req.LevelID}
		if err := db.Create(&row).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}

	updates := map[string]interface{}{}
	if req.SelfSpendRatio != nil {
		updates["self_spend_ratio"] = *req.SelfSpendRatio
	}
	if req.ReferralRegPoints != nil {
		updates["referral_reg_points"] = *req.ReferralRegPoints
	}
	if req.ReferralSpendRatio != nil {
		updates["referral_spend_ratio"] = *req.ReferralSpendRatio
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if err := db.Model(&models.MembershipGiftRatio{}).Where("level_id = ?", req.LevelID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
