package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"tuneloop-backend/bindmsg"
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

	// #2251：商户配置按商户 org 锚定（共享 tenant 的商户不得共用配置）
	db := merchantConfigDB(ctx)
	anchor, ok := resolveMerchantConfigAnchor(ctx)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "merchant scope not found"})
		return
	}

	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", anchor).First(&config).Error; err != nil {
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
				"config":            json.RawMessage(defaultTemplate.ConfigSchema),
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
			"config":            json.RawMessage(config.Config),
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
			"message": bindmsg.Message(err),
		})
		return
	}

	// Verify template exists
	db := merchantConfigDB(ctx)
	anchor, ok := resolveMerchantConfigAnchor(ctx)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "merchant scope not found"})
		return
	}
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

	// #2251：读写同一锚（商户 org）
	var existing models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", anchor).First(&existing).Error; err == nil {
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
		TenantID:   anchor,
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
			ID            string                  `json:"id" binding:"required"`
			BaseDailyRate *float64                `json:"base_daily_rate"`
			Overrides     *map[string]interface{} `json:"overrides,omitempty"`
		} `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": bindmsg.Message(err),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	updatedCount := 0

	for _, item := range req.Items {
		updates := map[string]interface{}{}
		if item.BaseDailyRate != nil && *item.BaseDailyRate > 0 {
			updates["base_daily_rate"] = models.ToCentsPtr(item.BaseDailyRate)
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

// pricingResponseWithInstrument merges per-instrument deposit fields into the
// pricing-v2 response.
//
// #2437-urgent：instruments.deposit/deposit_reduced 只要非 nil 即为权威（0 也有效）。
// NULL 才回落到定价结果；0 表示显式零押金/零差额档。前端据此展示与下单同源。
func pricingResponseWithInstrument(result *services.InstrumentPricing, instrument *models.Instrument) map[string]interface{} {
	data := services.FormatPricingResult(result)
	if instrumentDepositExplicit(instrument) {
		data["deposit"] = int64(*instrument.Deposit)
	}
	if instrument.DepositReduced != nil {
		data["deposit_reduced"] = int64(*instrument.DepositReduced)
	} else {
		data["deposit_reduced"] = nil
	}
	return data
}

// defaultMerchantPricingConfigJSON is the built-in template used when a merchant
// has no pricing config row yet. Single source: pricing-v2 and the instrument
// list both fall back to it (#2528 D6).
const defaultMerchantPricingConfigJSON = `{"deposit_mode":"ratio","deposit_ratio":0.3,"deposit_multiplier":7,"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20},{"days_max":-1,"discount_percent":40}]}`

// loadMerchantPricingConfigJSON resolves the merchant pricing config for a tenant.
//
// #2528: extracted from pricing-v2 so the instrument list shares one source of
// truth for deposit/daily-rate derivation. Behaviour contract:
//   - row found      → its Config JSON
//   - ErrRecordNotFound → built-in default template, nil error
//   - other DB error    → ("", err); callers decide (pricing-v2: 500; list: log + default, D7)
func loadMerchantPricingConfigJSON(ctx context.Context, tenantID string) (string, error) {
	var config models.MerchantPricingConfig
	if err := merchantConfigDB(ctx).Where("tenant_id = ?", tenantID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultMerchantPricingConfigJSON, nil
		}
		return "", fmt.Errorf("failed to query pricing config: %w", err)
	}
	return config.Config, nil
}

// instrumentListPricing holds the additive price fields returned by the
// instrument list (#2528). All money values are **cents**; nil means "unknown /
// not configured" so the PC layer can render `-` instead of a fake ¥0.00.
type instrumentListPricing struct {
	ResolvedDailyRate int64  // pricing-v2 effective base daily rate; 0 = cannot resolve (pricing-v2 → 40004)
	Deposit           *int64 // nil = cannot resolve; 0 = explicit zero deposit
	DepositReduced    *int64 // nil = no reduced tier; 0 = explicit zero difference
	// #2537: overdue rent column; same chain as pricing-v2
	// (overrides → pricing JSONB → config default). nil = cannot resolve.
	OverdueDailyFee *int64
}

// instrumentListPriceFields derives the list price fields from the same helpers
// pricing-v2 uses (#2528 D6): resolveDailyRateCents → CalculatePricing →
// pricingResponseWithInstrument (which owns the explicit-deposit authority).
//
// configCache is a per-request cache keyed by tenant ID so a page costs at most
// one merchant-config query. It is never nil in callers; a nil map is tolerated.
//
// D7: a config read failure degrades to the built-in template (logged) instead of
// failing the whole list; pricing-v2 keeps its own 500 semantics.
func instrumentListPriceFields(ctx context.Context, instrument *models.Instrument, configCache map[string]string) instrumentListPricing {
	if instrument == nil {
		return instrumentListPricing{}
	}
	rate := resolveDailyRateCents(*instrument)
	if rate <= 0 {
		// No base rate → pricing-v2 answers 40004; the list reports "unknown".
		return instrumentListPricing{}
	}

	configJSON, ok := configCache[instrument.TenantID]
	if !ok {
		loaded, err := loadMerchantPricingConfigJSON(ctx, instrument.TenantID)
		if err != nil {
			log.Printf("[InstrumentList] WARN: pricing config unavailable for tenant=%s, using default template: %v",
				instrument.TenantID, err)
			loaded = defaultMerchantPricingConfigJSON
		}
		configJSON = loaded
		if configCache != nil {
			configCache[instrument.TenantID] = loaded
		}
	}

	// Mirror pricing-v2: the resolved rate becomes the model's base rate before
	// CalculatePricing, so pricing_overrides.daily_rent can still take effect.
	probe := *instrument
	rateCents := models.Cents(rate)
	probe.BaseDailyRate = &rateCents

	totalPrice := 0.0
	if probe.TotalPrice != nil {
		totalPrice = probe.TotalPrice.ToYuan()
	}
	result := services.CalculatePricing(probe.BaseDailyRate.ToYuan(), totalPrice, configJSON, probe.PricingOverrides, probe.Pricing)
	data := pricingResponseWithInstrument(result, &probe)

	out := instrumentListPricing{}
	switch v := data["base_daily_rate"].(type) {
	case float64:
		out.ResolvedDailyRate = int64(math.Round(v))
	case int64:
		out.ResolvedDailyRate = v
	}
	switch v := data["deposit"].(type) {
	case float64:
		deposit := int64(math.Round(v))
		out.Deposit = &deposit
	case int64:
		deposit := v
		out.Deposit = &deposit
	}
	switch v := data["deposit_reduced"].(type) {
	case int64:
		reduced := v
		out.DepositReduced = &reduced
	case *int64:
		if v != nil {
			reduced := *v
			out.DepositReduced = &reduced
		}
	}
	// #2537: overdue_daily_fee comes from services.FormatPricingResult (cents,
	// float64 in the map). Keep the list and pricing-v2 on one calculation chain.
	switch v := data["overdue_daily_fee"].(type) {
	case float64:
		overdue := int64(math.Round(v))
		out.OverdueDailyFee = &overdue
	case int64:
		overdue := v
		out.OverdueDailyFee = &overdue
	}
	return out
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

	// #2047: 统一用 resolveDailyRateCents 单点解析（>0 才认定）。
	// ≤0 时回退 pricing JSONB daily_rent（元语义），语义与列表端对齐。
	rate := resolveDailyRateCents(instrument)
	if rate <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "instrument has no base daily rate configured",
		})
		return
	}
	rateCents := models.Cents(rate)
	instrument.BaseDailyRate = &rateCents

	// Fetch merchant pricing config（#2251：按乐器归属商户 org，而非操作者 tenant）
	// #2528 D6：与列表共用 loadMerchantPricingConfigJSON（NotFound → 内置默认模板；
	// 其它错误保持本端点既有 500 语义）。
	configJSON, err := loadMerchantPricingConfigJSON(ctx, instrument.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query pricing config",
		})
		return
	}

	totalPrice := 0.0
	if instrument.TotalPrice != nil {
		totalPrice = (*instrument.TotalPrice).ToYuan()
	}
	result := services.CalculatePricing((*instrument.BaseDailyRate).ToYuan(), totalPrice, configJSON, instrument.PricingOverrides, instrument.Pricing)
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": pricingResponseWithInstrument(result, &instrument),
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
	TemplateID string                 `json:"template_id"`
	Config     map[string]interface{} `json:"config"`
	Configured bool                   `json:"configured"`
}
