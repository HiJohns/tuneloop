package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "setting key is required"})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{"key": key, "value": ""},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"key": setting.SettingKey, "value": setting.SettingValue},
	})
}

func UpsertSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "setting key is required"})
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request body"})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	userID := middleware.GetUserID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var setting models.SystemSetting
	result := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting)
	if result.Error != nil {
		setting = models.SystemSetting{
			TenantID:     tenantID,
			SettingKey:   key,
			SettingValue: req.Value,
			UpdatedBy:    userID,
		}
		if err := db.Create(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create setting"})
			return
		}
	} else {
		setting.SettingValue = req.Value
		setting.UpdatedBy = userID
		setting.UpdatedAt = time.Now()
		if err := db.Save(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update setting"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"key": key, "value": req.Value}})
}
