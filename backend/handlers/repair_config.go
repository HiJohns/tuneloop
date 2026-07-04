package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

const (
	systemTenantID             = "00000000-0000-0000-0000-000000000000"
	keyRepairInspectionFee     = "repair_inspection_fee"
	keyRepairShippingFee       = "repair_shipping_fee"
	keyRepairGiftPointsEnabled = "repair_gift_points_enabled"
	keyRepairCheckFee          = "repair_check_fee"
)

// GetRepairSetting returns a single repair config value by key.
func GetRepairSetting(c *gin.Context) {
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "key is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"value": ""}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"value": setting.SettingValue}})
}

// SetRepairSetting saves a repair config value.
func SetRepairSetting(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "key and value required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, req.Key).First(&setting).Error; err == nil {
		db.Model(&setting).Update("setting_value", req.Value)
	} else {
		db.Create(&models.SystemSetting{
			TenantID:     tenantID,
			SettingKey:   req.Key,
			SettingValue: req.Value,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
}

// GetRepairAllSettings returns all repair config values.
func GetRepairAllSettings(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	keys := []string{keyRepairInspectionFee, keyRepairShippingFee, keyRepairGiftPointsEnabled}
	settings := map[string]string{}

	for _, k := range keys {
		var s models.SystemSetting
		if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, k).First(&s).Error; err == nil {
			settings[k] = s.SettingValue
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": settings})
}

// GetSiteShippingFee returns a site's shipping fee override.
func GetSiteShippingFee(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "site id required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var setting models.SystemSetting
	tenantID := middleware.GetTenantID(ctx)
	key := "site_shipping_fee_" + siteID

	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"value": ""}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"value": setting.SettingValue}})
}

// SetSiteShippingFee sets a site's shipping fee override.
func SetSiteShippingFee(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "site id required"})
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "value required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	key := "site_shipping_fee_" + siteID

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting).Error; err == nil {
		db.Model(&setting).Update("setting_value", req.Value)
	} else {
		db.Create(&models.SystemSetting{TenantID: tenantID, SettingKey: key, SettingValue: req.Value})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
}

// GetCheckFee returns the system-level check fee (v3: namespace_admin setting, not merchant-level).
func GetCheckFee(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", systemTenantID, keyRepairCheckFee).First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"check_fee": 0}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"check_fee": setting.SettingValue}})
}

// SetCheckFee sets the system-level check fee (namespace_admin only).
func SetCheckFee(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "value required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", systemTenantID, keyRepairCheckFee).First(&setting).Error; err == nil {
		db.Model(&setting).Update("setting_value", req.Value)
	} else {
		db.Create(&models.SystemSetting{TenantID: systemTenantID, SettingKey: keyRepairCheckFee, SettingValue: req.Value})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "check fee saved"})
}
