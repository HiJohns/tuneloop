package handlers

import (
	"log"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
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

// GetPublicSetting returns a setting by key from the global tenant (nil UUID).
// No auth required — used by public-facing content pages.
func GetPublicSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "setting key is required"})
		return
	}

	nilUUID := "00000000-0000-0000-0000-000000000000"
	db := database.GetDB().WithContext(c.Request.Context())

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", nilUUID, key).First(&setting).Error; err != nil {
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

// UpsertGlobalSetting saves a setting for the global tenant (nil UUID).
// Requires auth with namespace_admin or higher.
func UpsertGlobalSetting(c *gin.Context) {
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

	nilUUID := "00000000-0000-0000-0000-000000000000"
	userID := middleware.GetUserID(c.Request.Context())
	// Global settings live under the nil UUID tenant. Clear the tenant from the
	// context so registerTenantCallbacks' addTenantScope does not inject
	// "tenant_id = <real tenant>" and turn the lookup into an always-false AND.
	db := database.GetDB().WithContext(database.SetTenantID(c.Request.Context(), ""))

	// Atomic upsert: ON CONFLICT (tenant_id, setting_key) DO UPDATE — eliminates
	// the check-then-create race that previously hit SQLSTATE 23505 on repeat save.
	setting := models.SystemSetting{
		TenantID:     nilUUID,
		SettingKey:   key,
		SettingValue: req.Value,
		UpdatedBy:    userID,
		UpdatedAt:    time.Now(),
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "setting_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"setting_value", "updated_by", "updated_at"}),
	}).Create(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save setting"})
		return
	}

	// Reconcile embedded /uploads/media/<key> references against media_assets
	// (#1692): mark present assets referenced, mark removed ones unreferenced.
	if err := services.NewMediaRegistry().ReconcileHTMLRefs(c.Request.Context(), key, req.Value); err != nil {
		log.Printf("[MediaRegistry] reconcile html refs for %s failed: %v", key, err)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
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
