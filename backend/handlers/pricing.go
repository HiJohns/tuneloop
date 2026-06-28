package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GET /api/pricing/templates - List available pricing templates
func ListPricingTemplates(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var templates []models.PricingTemplate
	if err := db.Where("is_active = ?", true).Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch pricing templates: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": templates,
	})
}

// GET /api/merchant/pricing-config - Get current merchant's pricing config
func GetMerchantPricingConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", tenantID).First(&config).Error; err != nil {
		// Return default template config if merchant hasn't configured yet
		var defaultTemplate models.PricingTemplate
		if err := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "no pricing template found",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"template_id":       defaultTemplate.ID,
				"template_name":     defaultTemplate.Name,
				"template_code":     defaultTemplate.Code,
				"is_system_default": defaultTemplate.IsSystemDefault,
				"config":            defaultTemplate.ConfigSchema,
				"configured":        false,
			},
		})
		return
	}

	// Fetch template name
	var tmpl models.PricingTemplate
	tmplName := ""
	if err := db.Where("id = ?", config.TemplateID).First(&tmpl).Error; err == nil {
		tmplName = tmpl.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":                config.ID,
			"template_id":       config.TemplateID,
			"template_name":     tmplName,
			"is_system_default": tmpl.IsSystemDefault,
			"config":            config.Config,
			"configured":        true,
		},
	})
}

// PUT /api/merchant/pricing-config - Update merchant pricing config
func UpdateMerchantPricingConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	var req struct {
		TemplateID string                 `json:"template_id" binding:"required"`
		Config     map[string]interface{} `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	// Verify template exists
	db := database.GetDB().WithContext(ctx)
	var tmpl models.PricingTemplate
	if err := db.Where("id = ? AND is_active = ?", req.TemplateID, true).First(&tmpl).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "pricing template not found",
		})
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to marshal config: " + err.Error(),
		})
		return
	}

	userID := middleware.GetUserID(ctx)

	var existing models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", tenantID).First(&existing).Error; err == nil {
		// Update existing config
		if err := db.Model(&existing).Updates(map[string]interface{}{
			"template_id": req.TemplateID,
			"config":      string(configJSON),
			"updated_by":  userID,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to update pricing config: " + err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"message": "pricing config updated successfully",
		})
		return
	}

	// Create new config
	config := models.MerchantPricingConfig{
		TenantID:   tenantID,
		TemplateID: req.TemplateID,
		Config:     string(configJSON),
		UpdatedBy:  userID,
	}
	if err := db.Create(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create pricing config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20100,
		"message": "pricing config created successfully",
	})
}

// PUT /api/instruments/batch-pricing - Batch set instrument base daily rates
func BatchSetInstrumentPricing(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Items []struct {
			ID            string   `json:"id" binding:"required"`
			BaseDailyRate *float64 `json:"base_daily_rate"`
			Overrides     *map[string]interface{} `json:"overrides,omitempty"`
		} `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	updatedCount := 0

	for _, item := range req.Items {
		updates := map[string]interface{}{}
		if item.BaseDailyRate != nil {
			updates["base_daily_rate"] = *item.BaseDailyRate
		}
		if item.Overrides != nil {
			ovJSON, err := json.Marshal(item.Overrides)
			if err == nil {
				updates["pricing_overrides"] = string(ovJSON)
			}
		}
		if len(updates) == 0 {
			continue
		}

		result := db.Model(&models.Instrument{}).
			Where("id = ? AND tenant_id = ?", item.ID, tenantID).
			Updates(updates)
		if result.Error != nil {
			log.Printf("[BatchSetPricing] Failed to update instrument %s: %v", item.ID, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			updatedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"updated": updatedCount,
		},
	})
}

// GET /api/instruments/:id/pricing-v2 - Get instrument full pricing with tier calculation
func GetInstrumentPricingV2(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	instrumentID := c.Param("id")

	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(instrumentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "invalid instrument id format",
		})
		return
	}

	db := database.GetDB().WithContext(ctx)

	// Fetch instrument for base price
	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "instrument not found",
		})
		return
	}

	if instrument.BaseDailyRate == nil || *instrument.BaseDailyRate <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "instrument has no base daily rate configured",
		})
		return
	}

	// Fetch merchant pricing config
	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", tenantID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback to system default template
			var defaultTemplate models.PricingTemplate
			if err2 := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err2 != nil {
				c.JSON(http.StatusNotFound, gin.H{
					"code":    40400,
					"message": "no pricing template found",
				})
				return
			}
			config.Config = defaultTemplate.ConfigSchema
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to query pricing config",
			})
			return
		}
	}

	result := services.CalculatePricing(*instrument.BaseDailyRate, config.Config, instrument.PricingOverrides, instrument.Pricing)
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": services.FormatPricingResult(result),
	})
}

// Helper: calculate days between two dates for tier matching
func GetDailyRateForDuration(pricing *services.InstrumentPricing, days int) float64 {
	for _, tier := range pricing.Tiers {
		if tier.DaysMax == -1 || days <= tier.DaysMax {
			return tier.DailyRate
		}
	}
	if len(pricing.Tiers) > 0 {
		return pricing.Tiers[len(pricing.Tiers)-1].DailyRate
	}
	return pricing.BaseDailyRate
}

// Re-export for other handlers to use
var GetDailyRateForDurationFn = GetDailyRateForDuration

type PricingConfigResponse struct {
	TemplateID   string                    `json:"template_id"`
	Config       map[string]interface{}    `json:"config"`
	Configured   bool                      `json:"configured"`
}


