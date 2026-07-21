package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"log"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

var pricingService = service.NewPricingService()

// CreateInstrumentRequest is used for POST (create). Fields are optional except CategoryID.
type CreateInstrumentRequest struct {
	LevelID        string                   `json:"level_id"`
	CategoryID     string                   `json:"category_id" binding:"required"`
	SN             string                   `json:"sn"`
	SiteID         string                   `json:"site_id"`
	Status         string                   `json:"status"`
	Pricing        map[string]interface{}   `json:"pricing"`
	BaseDailyRate  *float64                 `json:"base_daily_rate"`
	TotalPrice     *float64                 `json:"total_price"`
	Deposit        *float64                 `json:"deposit"`
	Description    string                   `json:"description"`
	Images         []string                 `json:"images"`
	Video          string                   `json:"video"`
	Poster         string                   `json:"poster"`
	Specifications []map[string]interface{} `json:"specifications"`
	Properties     map[string]interface{}   `json:"properties"`
	Level          string                   `json:"level"`
}

// UpdateInstrumentRequest is used for PUT (partial update). Pointer fields distinguish
// "not sent" (nil) from "explicitly cleared" (empty string).
type UpdateInstrumentRequest struct {
	CategoryID     *string                  `json:"category_id"`
	LevelID        *string                  `json:"level_id"`
	SiteID         *string                  `json:"site_id"`
	Status         *string                  `json:"status"`
	Description    *string                  `json:"description"`
	Video          *string                  `json:"video"`
	Poster         *string                  `json:"poster"`
	Images         []string                 `json:"images"`
	Pricing        map[string]interface{}   `json:"pricing"`
	BaseDailyRate  *float64                 `json:"base_daily_rate"`
	TotalPrice     *float64                 `json:"total_price"`
	Deposit        *float64                 `json:"deposit"`
	Specifications []map[string]interface{} `json:"specifications"`
	Properties     map[string]interface{}   `json:"properties"`
	Level          *string                  `json:"level"`
}

// processProperties handles the properties association logic for instruments
func processProperties(db *gorm.DB, instrumentID string, tenantID string, userID string, properties map[string]interface{}) error {
	return processPropertiesWithScope(db, instrumentID, tenantID, userID, properties, "", nil)
}

func processPropertiesWithScope(db *gorm.DB, instrumentID string, tenantID string, userID string,
	properties map[string]interface{}, categoryID string, rowPropValues map[string]string) error {

	// Resolve audit fields from user info (for pending property options)
	submitterID := userID
	merchantID := tenantID
	siteID := ""
	if userID != "" {
		var member struct{ SiteID string }
		if err := database.GetDB().Table("site_members").
			Joins("JOIN users ON users.id = site_members.user_id").
			Where("users.iam_sub = ?", userID).
			Select("site_members.site_id").First(&member).Error; err == nil {
			siteID = member.SiteID
		}
	}

	for propName, rawValues := range properties {

		// 1. Find property definition by name (platform-wide, unscoped)
		var prop models.Property
		if err := database.GetDB().Where("name = ?", propName).First(&prop).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("[processProperties] property '%s' not defined, skipping", propName)
				continue
			}
			return err
		}
		// Use the property's own tenant_id (properties are platform-level, FK requires matching tenant)
		effectiveTenantID := prop.TenantID

		// Determine scope for this property
		scopeCategoryID := ""
		scopeParentValue := ""
		if prop.ScopeType == "category" && prop.RelatedCategoryID != nil && categoryID != "" {
			scopeCategoryID = categoryID
		} else if prop.ScopeType == "property" && prop.RelatedPropertyID != nil && rowPropValues != nil {
			var parentProp models.Property
			if err := database.GetDB().Where("id = ?", *prop.RelatedPropertyID).First(&parentProp).Error; err == nil {
				if pv, ok := rowPropValues[parentProp.Name]; ok {
					scopeParentValue = pv
				}
			}
		}

		// Convert values to string slice
		values, ok := toStringSlice(rawValues)
		if !ok || len(values) == 0 {
			continue
		}

		for _, value := range values {
			if value == "" {
				continue
			}

			// 2. Check if property option exists (handle scope and alias)
			var propOption models.PropertyOption
			q := database.GetDB().Where("property_name = ? AND value = ?", prop.Name, value)
			if scopeCategoryID != "" {
				q = q.Where("scope_category_id = ?", scopeCategoryID)
			} else {
				q = q.Where("scope_category_id IS NULL")
			}
			if scopeParentValue != "" {
				q = q.Where("scope_parent_value = ?", scopeParentValue)
			} else {
				q = q.Where("scope_parent_value IS NULL")
			}
			err := q.First(&propOption).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 3. Create new property option with status=pending
				propOption = models.PropertyOption{
					ID:           uuid.New().String(),
					TenantID:     effectiveTenantID,
					PropertyName: prop.Name,
					Value:        value,
					Status:       "pending",
					SubmitterID:  submitterID,
					MerchantID:   merchantID,
					InstrumentID: instrumentID,
				}
				if siteID != "" {
					propOption.SiteID = &siteID
				}
				if scopeCategoryID != "" {
					propOption.ScopeCategoryID = &scopeCategoryID
				}
				if scopeParentValue != "" {
					propOption.ScopeParentValue = &scopeParentValue
				}
				if err := db.Create(&propOption).Error; err != nil {
					return fmt.Errorf("failed to create property_option for '%s=%s': %w", propName, value, err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to query property_option: %w", err)
			} else if propOption.Alias != nil {
				// Auto-resolve alias: if this option has an alias, use the target option's value
				var aliasOption models.PropertyOption
				if err := database.GetDB().Where("id = ?", *propOption.Alias).First(&aliasOption).Error; err == nil {
					value = aliasOption.Value
				}
			}

			// 5. Create instrument_property association
			instProp := models.InstrumentProperty{
				ID:           uuid.New().String(),
				TenantID:     tenantID,
				InstrumentID: instrumentID,
				PropertyName: prop.Name,
				Value:        value,
			}
			if err := db.Create(&instProp).Error; err != nil {
				return fmt.Errorf("failed to create instrument_property for '%s=%s': %w", propName, value, err)
			}
		}
	}
	return nil
}

// Helper function to convert interface{} to []string
func toStringSlice(raw interface{}) ([]string, bool) {
	switch v := raw.(type) {
	case string:
		return []string{v}, true
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, true
	case []string:
		return v, true
	default:
		return nil, false
	}
}

func GetInstrumentPricing(c *gin.Context) {
	instrumentID := c.Param("id")

	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	pricing := pricingService.GetInstrumentPricing(instrumentID)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": pricing,
	})
}

func CreateInstrument(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	userID := middleware.GetUserID(ctx)

	log.Printf("[DEBUG CreateInstrument] tenantID=%s, orgID=%s, userID=%s", tenantID, orgID, userID)

	if tenantID == "" {
		log.Printf("[DEBUG CreateInstrument] WARNING: tenantID is empty, orgID=%s", orgID)
		if orgID != "" {
			tenantID = orgID
			log.Printf("[DEBUG CreateInstrument] Using orgID as tenantID: %s", tenantID)
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing tenant_id in context",
			})
			return
		}
	}

	var req CreateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] CreateInstrument - BindJSON error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	// Validate level_id is provided (level string is deprecated)
	if req.LevelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "level_id is required",
		})
		return
	}

	log.Printf("[DEBUG] CreateInstrument - Parsed: SN='%s', Level='%s', LevelID='%s', CategoryID='%s', SiteID='%s', Properties=%v",
		req.SN, req.Level, req.LevelID, req.CategoryID, req.SiteID, req.Properties)

	log.Printf("[DEBUG CreateInstrument] Creating instrument: sn=%s", req.SN)

	// Get category_name from database
	var categoryName string
	if req.CategoryID != "" {
		var cat struct {
			Name string `json:"name"`
		}
		if err := db.Table("categories").Where("id = ?", req.CategoryID).Select("name").First(&cat).Error; err == nil {
			categoryName = cat.Name
		}
	}

	// Map level to level_name and level_id
	levelName := ""
	var levelID *uuid.UUID

	// Use LevelID directly if provided
	if req.LevelID != "" {
		if parsedID, err := uuid.Parse(req.LevelID); err == nil {
			levelID = &parsedID
			// Get level_name from instrument_levels table
			var level models.InstrumentLevel
			if err := db.Where("id = ?", levelID).First(&level).Error; err == nil {
				levelName = level.Caption
			}
		}
	} else if req.Level != "" {
		// Legacy: map level string to level_id
		var level models.InstrumentLevel
		// Try to find by code or caption
		if err := db.Where("code = ? OR caption = ?", req.Level, req.Level).First(&level).Error; err == nil {
			levelID = &level.ID
			levelName = level.Caption
		} else {
			// Fallback to old mapping for backward compatibility
			switch req.Level {
			case "beginner":
				levelName = "入门级"
			case "intermediate":
				levelName = "中级"
			case "advanced":
				levelName = "高级"
			case "professional":
				levelName = "专业级"
			}
			log.Printf("[DEBUG] Level '%s' not found in instrument_levels, using legacy mapping: %s", req.Level, levelName)
		}
	}

	// Check SN uniqueness within tenant
	if req.SN != "" {
		var snCount int64
		if err := db.Model(&models.Instrument{}).Where("sn = ? AND tenant_id = ?", req.SN, tenantID).Count(&snCount).Error; err == nil && snCount > 0 {
			c.JSON(http.StatusConflict, gin.H{"code": 40901, "message": "识别码已存在"})
			return
		}
	}

	instrument := models.Instrument{
		TenantID:     tenantID,
		SN:           req.SN,
		LevelName:    levelName,
		LevelID:      levelID,
		CategoryName: categoryName,
		Description:  req.Description,
		StockStatus:  "available",
	}
	if req.CategoryID != "" {
		instrument.CategoryID = &req.CategoryID
	}

	// Handle SiteID
	if req.SiteID != "" {
		if siteUUID, err := uuid.Parse(req.SiteID); err == nil {
			instrument.SiteID = &siteUUID
		}
	}

	// Resolve OrgID from site's IAM org (matches ApplyOrgScope filtering in GetInstruments)
	if req.SiteID != "" {
		var site models.Site
		if err := db.Where("id = ?", req.SiteID).First(&site).Error; err == nil && site.OrgID != "" {
			instrument.OrgID = &site.OrgID
		}
	}
	if instrument.OrgID == nil && orgID != "" {
		instrument.OrgID = &orgID
	}

	// Handle Images field
	if req.Images != nil && len(req.Images) > 0 {
		imagesJSON, err := json.Marshal(req.Images)
		if err != nil {
			log.Printf("[ERROR] Failed to marshal images: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40003,
				"message": "invalid images format: " + err.Error(),
			})
			return
		}
		instrument.Images = string(imagesJSON)
	} else {
		instrument.Images = "[]"
	}

	// Handle Specifications field
	if req.Specifications != nil && len(req.Specifications) > 0 {
		specsJSON, err := json.Marshal(req.Specifications)
		if err != nil {
			log.Printf("[ERROR] Failed to marshal specifications: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40004,
				"message": "invalid specifications format: " + err.Error(),
			})
			return
		}
		instrument.Specifications = string(specsJSON)
	} else if req.Properties != nil && len(req.Properties) > 0 {
		// Also accept properties from frontend and convert to specifications JSON
		propsJSON, err := json.Marshal(req.Properties)
		if err != nil {
			log.Printf("[ERROR] Failed to marshal properties: %v", err)
		} else {
			instrument.Specifications = string(propsJSON)
		}
	} else {
		instrument.Specifications = "[]"
	}

	// Handle Pricing field
	if req.Pricing != nil {
		pricingJSON, err := json.Marshal(req.Pricing)
		if err != nil {
			log.Printf("[ERROR] Failed to marshal pricing: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40005,
				"message": "invalid pricing format: " + err.Error(),
			})
			return
		}
		instrument.Pricing = string(pricingJSON)
	} else {
		instrument.Pricing = "{}"
	}

	if req.BaseDailyRate != nil && *req.BaseDailyRate > 0 {
		instrument.BaseDailyRate = req.BaseDailyRate
	}
	if req.TotalPrice != nil && *req.TotalPrice > 0 {
		instrument.TotalPrice = req.TotalPrice
	}

	// Handle Video field
	if req.Deposit != nil && *req.Deposit > 0 {
		instrument.Deposit = req.Deposit
	}

	instrument.Video = req.Video
	instrument.Poster = req.Poster

	log.Printf("[DEBUG CreateInstrument] Before DB Create: tenantID=%s", instrument.TenantID)

	if err := db.Create(&instrument).Error; err != nil {
		log.Printf("[ERROR CreateInstrument] DB Error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create instrument: " + err.Error(),
		})
		return
	}

	log.Printf("[DEBUG CreateInstrument] Success: instrument.ID=%s", instrument.ID)

	// Process properties if provided
	if req.Properties != nil && len(req.Properties) > 0 {
		if err := processProperties(db, instrument.ID, tenantID, userID, req.Properties); err != nil {
			log.Printf("[ERROR] Failed to process properties: %v", err)
			// Don't fail the request if properties processing fails
			// The instrument was already created successfully
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id": instrument.ID,
			"sn": instrument.SN,
		},
	})
}

// PUT /api/instruments/:id - Update instrument
func UpdateInstrument(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "instrument not found",
		})
		return
	}

	oldInstrument := instrument

	var req UpdateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	if req.CategoryID != nil {
		instrument.CategoryID = req.CategoryID
	}
	if req.Description != nil {
		instrument.Description = *req.Description
	}
	if req.Video != nil {
		instrument.Video = *req.Video
	}
	if req.Poster != nil {
		instrument.Poster = *req.Poster
	}
	if req.Deposit != nil && *req.Deposit > 0 {
		instrument.Deposit = req.Deposit
	}
	log.Printf("[DEBUG] req.CategoryID = '%v', req.Level = '%v'", req.CategoryID, req.Level)

	if req.SiteID != nil {
		if siteUUID, err := uuid.Parse(*req.SiteID); err == nil {
			instrument.SiteID = &siteUUID
		}
	}

	if req.Status != nil {
		instrument.StockStatus = *req.Status
	}

	if req.CategoryID != nil && *req.CategoryID != "" {
		if _, err := uuid.Parse(*req.CategoryID); err == nil {
			var cat struct {
				Name string `json:"name"`
			}
			if err := db.Table("categories").Where("id = ?", *req.CategoryID).Select("name").First(&cat).Error; err == nil {
				instrument.CategoryName = cat.Name
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "invalid category_id format: must be a valid UUID",
			})
			return
		}
	}

	if req.LevelID != nil && *req.LevelID != "" {
		if parsedID, err := uuid.Parse(*req.LevelID); err == nil {
			instrument.LevelID = &parsedID
			var level models.InstrumentLevel
			if err := db.Where("id = ?", instrument.LevelID).First(&level).Error; err == nil {
				instrument.LevelName = level.Caption
			}
		}
	} else if req.Level != nil && *req.Level != "" {
		var level models.InstrumentLevel
		if err := db.Where("code = ? OR caption = ?", *req.Level, *req.Level).First(&level).Error; err == nil {
			instrument.LevelID = &level.ID
			instrument.LevelName = level.Caption
		} else {
			// Fallback to old mapping for backward compatibility
			switch *req.Level {
			case "beginner":
				instrument.LevelName = "入门级"
			case "intermediate":
				instrument.LevelName = "中级"
			case "advanced":
				instrument.LevelName = "高级"
			case "professional":
				instrument.LevelName = "专业级"
			}
			log.Printf("[DEBUG] Level '%s' not found in instrument_levels, using legacy mapping: %s", *req.Level, instrument.LevelName)
		}
	}

	if req.Images != nil {
		imagesJSON, err := json.Marshal(req.Images)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40003,
				"message": "invalid images format: " + err.Error(),
			})
			return
		}
		instrument.Images = string(imagesJSON)
	}

	if req.Specifications != nil && len(req.Specifications) > 0 {
		specsJSON, err := json.Marshal(req.Specifications)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40004,
				"message": "invalid specifications format: " + err.Error(),
			})
			return
		}
		instrument.Specifications = string(specsJSON)
	}

	if req.Pricing != nil {
		pricingJSON, err := json.Marshal(req.Pricing)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40005,
				"message": "invalid pricing format: " + err.Error(),
			})
			return
		}
		instrument.Pricing = string(pricingJSON)
	}

	// Step 2: 构建 updates map
	updates := map[string]interface{}{}

	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if req.Video != nil {
		updates["video"] = *req.Video
	}

	if req.Poster != nil {
		updates["poster"] = *req.Poster
	}

	if req.Deposit != nil && *req.Deposit > 0 {
		updates["deposit"] = *req.Deposit
	}

	if req.CategoryID != nil && *req.CategoryID != "" {
		if _, err := uuid.Parse(*req.CategoryID); err == nil {
			updates["category_id"] = *req.CategoryID
			var cat struct {
				Name string `json:"name"`
			}
			if err := db.Table("categories").Where("id = ?", *req.CategoryID).Select("name").First(&cat).Error; err == nil {
				updates["category_name"] = cat.Name
			}
		}
	}

	if req.LevelID != nil && *req.LevelID != "" {
		if parsedID, err := uuid.Parse(*req.LevelID); err == nil {
			updates["level_id"] = parsedID
			var level models.InstrumentLevel
			if err := db.Where("id = ?", parsedID).First(&level).Error; err == nil {
				updates["level_name"] = level.Caption
			}
		}
	}

	if req.SiteID != nil && *req.SiteID != "" {
		if siteUUID, err := uuid.Parse(*req.SiteID); err == nil {
			updates["site_id"] = siteUUID
		}
	}

	if req.Status != nil {
		updates["stock_status"] = *req.Status
	}

	if req.Images != nil && len(req.Images) > 0 {
		imagesJSON, _ := json.Marshal(req.Images)
		updates["images"] = string(imagesJSON)
	}

	// Specifications - 非空数组才更新
	if req.Specifications != nil && len(req.Specifications) > 0 {
		specsJSON, _ := json.Marshal(req.Specifications)
		updates["specifications"] = string(specsJSON)
	}

	// Pricing - 非空才更新
	if req.Pricing != nil {
		pricingJSON, _ := json.Marshal(req.Pricing)
		updates["pricing"] = string(pricingJSON)
	}
	if req.BaseDailyRate != nil && *req.BaseDailyRate > 0 {
		updates["base_daily_rate"] = *req.BaseDailyRate
	}
	if req.TotalPrice != nil && *req.TotalPrice > 0 {
		updates["total_price"] = *req.TotalPrice
	}

	// Step 3: 确保 tenant_id 不为空
	if instrument.TenantID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "instrument is missing tenant_id",
		})
		return
	}

	// Build field-level diff for audit log
	diffFields := map[string]interface{}{}
	for key, newVal := range updates {
		oldVal := map[string]interface{}{}
		switch key {
		case "description":
			oldVal["old"] = oldInstrument.Description
		case "video":
			oldVal["old"] = oldInstrument.Video
		case "poster":
			oldVal["old"] = oldInstrument.Poster
		case "category_id":
			oldVal["old"] = oldInstrument.CategoryID
		case "level_id":
			oldVal["old"] = oldInstrument.LevelID
		case "base_daily_rate":
			oldVal["old"] = oldInstrument.BaseDailyRate
		default:
			continue
		}
		oldVal["new"] = newVal
		diffFields[key] = oldVal
	}
	if len(diffFields) > 0 {
		diffJSON, _ := json.Marshal(diffFields)
		log.Printf("[InstrumentUpdate] diff: %s", string(diffJSON))
	}

	// 只有当有字段需要更新时才执行更新
	if len(updates) > 0 {
		if err := db.Model(&instrument).Where("id = ? AND tenant_id = ?", instrument.ID, instrument.TenantID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to update instrument: " + err.Error(),
			})
			return
		}
	}

	// Process properties if provided (delete existing and recreate)
	if req.Properties != nil && len(req.Properties) > 0 {
		// Delete existing properties for this instrument
		db.Where("instrument_id = ?", instrument.ID).Delete(&models.InstrumentProperty{})

		// Create new property associations
		if err := processProperties(db, instrument.ID, tenantID, userID, req.Properties); err != nil {
			log.Printf("[ERROR] Failed to process properties: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save properties: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id": instrument.ID,
			"sn": instrument.SN,
		},
	})
}

// PUT /api/instruments/:id/status - Update instrument stock status
func UpdateInstrumentStatus(c *gin.Context) {
	instrumentID := c.Param("id")

	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	var req struct {
		StockStatus string `json:"stock_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Validate stock_status value
	validStatuses := []string{
		models.StockStatusAvailable,
		models.StockStatusRented,
		models.StockStatusMaintenance,
		models.StockStatusArchived,
		models.StockStatusLost,
	}
	isValid := false
	for _, status := range validStatuses {
		if req.StockStatus == status {
			isValid = true
			break
		}
	}

	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "invalid stock_status. Valid values: available, reserved, shipping, rented, returning, maintenance, archived",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Update the instrument
	result := db.Model(&models.Instrument{}).
		Where("id = ? AND tenant_id = ?", instrumentID, tenantID).
		Update("stock_status", req.StockStatus)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update instrument status: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "instrument not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":           instrumentID,
			"stock_status": req.StockStatus,
		},
	})
}

// GET /api/instruments/check - Check if SN exists
func CheckInstrumentSN(c *gin.Context) {
	sn := c.Query("sn")
	if sn == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "sn parameter is required",
		})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var instrument models.Instrument
	err := db.Where("sn = ? AND tenant_id = ?", sn, tenantID).First(&instrument).Error

	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"exists": false,
			},
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to check SN: " + err.Error(),
		})
		return
	}

	var siteName, categoryName string
	if instrument.SiteID != nil {
		var site models.Site
		if err := db.First(&site, "id = ?", instrument.SiteID).Error; err == nil {
			siteName = site.Name
		}
	}
	if instrument.CategoryName != "" {
		categoryName = instrument.CategoryName
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"exists": true,
			"info": gin.H{
				"id":            instrument.ID,
				"sn":            instrument.SN,
				"site":          siteName,
				"site_id":       instrument.SiteID,
				"category":      categoryName,
				"category_id":   instrument.CategoryID,
				"category_name": instrument.CategoryName,
				"stock_status":  instrument.StockStatus,
			},
		},
	})
}

// GET /api/instruments/levels - Get instrument levels list
func GetInstrumentLevels(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var levels []models.InstrumentLevel
	if err := db.Order("sort_order").Find(&levels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to get instrument levels",
		})
		return
	}

	// Auto-populate default levels if table is empty
	// This ensures the API always returns data even if migration hasn't run
	if len(levels) == 0 {
		fmt.Println("[DEBUG] Instrument levels table is empty, populating default levels")
		defaultLevels := []models.InstrumentLevel{
			{Caption: "入门", Code: "entry", SortOrder: 1},
			{Caption: "专业", Code: "professional", SortOrder: 2},
			{Caption: "大师", Code: "master", SortOrder: 3},
		}

		for _, level := range defaultLevels {
			// Check if already exists before creating
			var existing models.InstrumentLevel
			if err := db.Where("caption = ? OR code = ?", level.Caption, level.Code).First(&existing).Error; err == nil {
				fmt.Printf("[DEBUG] Level already exists: %s (%s), skipping\n", level.Caption, level.Code)
				levels = append(levels, existing)
				continue
			}
			if err := db.Create(&level).Error; err != nil {
				fmt.Printf("[WARN] Failed to create instrument level %s: %v\n", level.Caption, err)
				continue
			}
			levels = append(levels, level)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": levels,
	})
}

// DeleteInstrument handles DELETE /api/instruments/:id
func DeleteInstrument(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	instrumentID := c.Param("id")

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "乐器不存在"})
		return
	}

	if instrument.StockStatus == models.StockStatusRented {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "乐器正在使用中，无法删除"})
		return
	}

	// Check for linked orders before deletion
	var orderCount int64
	db.Model(&models.Order{}).Where("instrument_id = ?", instrumentID).Count(&orderCount)
	if orderCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40901, "message": "乐器有关联订单，无法删除"})
		return
	}

	if err := db.Delete(&instrument).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "删除乐器失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "乐器已删除"})
}
