package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetRebateConfig(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var configs []models.RebateConfig
	if err := db.Order("level_id ASC").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": configs})
}

func UpdateRebateConfig(c *gin.Context) {
	var req struct {
		LevelID   int      `json:"level_id" binding:"required"`
		RentRatio *float64 `json:"rent_ratio"`
		IsActive  *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	updates := map[string]interface{}{}
	if req.RentRatio != nil {
		updates["rent_ratio"] = *req.RentRatio
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if err := db.Model(&models.RebateConfig{}).Where("level_id = ?", req.LevelID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
