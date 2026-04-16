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

type CreateInstrumentRequest struct {
	LevelID        string                   `json:"level_id"` // UUID reference to instrument_levels
	CategoryID     string                   `json:"category_id" binding:"required"`
	SN             string                   `json:"sn"`
	SiteID         string                   `json:"site_id"`
	Status         string                   `json:"status"`
	Pricing        map[string]interface{}   `json:"pricing"`
	Description    string                   `json:"description"`
	Images         []string                 `json:"images"`
	Video          string                   `json:"video"`
	Specifications []map[string]interface{} `json:"specifications"`
	Properties     map[string]interface{}   `json:"properties"` // Accept frontend's properties field

	// Deprecated fields - kept for backward compatibility, use Properties instead
	Brand string `json:"brand"`
	Model string `json:"model"`
	Level string `json:"level"`
}

// processProperties handles the properties association logic for instruments
func processProperties(db *gorm.DB, instrumentID string, tenantID string, properties map[string]interface{}) error {
	for propName, rawValues := range properties {
		// 1. Find property definition by name
		var prop models.Property
		if err := db.Where("name = ? AND tenant_id = ?", propName, tenantID).First(&prop).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("property '%s' not defined in properties table", propName)
			}
			return err
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

			// 2. Check if property option exists
			var propOption models.PropertyOption
			err := db.Where("property_id = ? AND value = ? AND tenant_id = ?",
				prop.ID, value, tenantID).First(&propOption).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 3. Create new property option with status=pending
				propOption = models.PropertyOption{
					ID:         uuid.New().String(),
					TenantID:   tenantID,
					PropertyID: prop.ID,
					Value:      value,
					Status:     "pending",
				}
				if err := db.Create(&propOption).Error; err != nil {
					return fmt.Errorf("failed to create property_option for '%s=%s': %w", propName, value, err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to query property_option: %w", err)
			}

			// 4. Create instrument_property association
			instProp := models.InstrumentProperty{
				ID:           uuid.New().String(),
				TenantID:     tenantID,
				InstrumentID: instrumentID,
				PropertyID:   prop.ID,
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
	db := database.GetDB()
	ctx := c.Request.Context()
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

	instrument := models.Instrument{
		TenantID:     tenantID,
		OrgID:        tenantID,
		SN:           req.SN,
		LevelName:    levelName,
		LevelID:      levelID,
		CategoryID:   req.CategoryID,
		CategoryName: categoryName,
		Description:  req.Description,
		StockStatus:  "available",
	}

	// Handle SiteID
	if req.SiteID != "" {
		if siteUUID, err := uuid.Parse(req.SiteID); err == nil {
			instrument.SiteID = &siteUUID
		}
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
		if err := processProperties(db, instrument.ID, tenantID, req.Properties); err != nil {
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
	db := database.GetDB()
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

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "instrument not found",
		})
		return
	}

	var req CreateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	instrument.CategoryID = req.CategoryID
	instrument.Description = req.Description
	instrument.Video = req.Video
	log.Printf("[DEBUG] req.SN = '%s', req.Level = '%s', req.CategoryID = '%s'", req.SN, req.Level, req.CategoryID)
	instrument.SN = req.SN

	if req.SiteID != "" {
		if siteUUID, err := uuid.Parse(req.SiteID); err == nil {
			instrument.SiteID = &siteUUID
		}
	}

	if req.Status != "" {
		instrument.StockStatus = req.Status
	}

	// Step 1: 添加空值校验 - 只有当 category_id 不为空且是有效 UUID 时才查询
	if req.CategoryID != "" {
		// 校验 UUID 格式
		if _, err := uuid.Parse(req.CategoryID); err == nil {
			var cat struct {
				Name string `json:"name"`
			}
			if err := db.Table("categories").Where("id = ?", req.CategoryID).Select("name").First(&cat).Error; err == nil {
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

	// Handle level_id and level_name (similar to CreateInstrument)
	if req.LevelID != "" {
		if parsedID, err := uuid.Parse(req.LevelID); err == nil {
			instrument.LevelID = &parsedID
			// Get level_name from instrument_levels table
			var level models.InstrumentLevel
			if err := db.Where("id = ?", instrument.LevelID).First(&level).Error; err == nil {
				instrument.LevelName = level.Caption
			}
		}
	} else if req.Level != "" {
		// Legacy: map level string to level_id
		var level models.InstrumentLevel
		// Try to find by code or caption
		if err := db.Where("code = ? OR caption = ?", req.Level, req.Level).First(&level).Error; err == nil {
			instrument.LevelID = &level.ID
			instrument.LevelName = level.Caption
		} else {
			// Fallback to old mapping for backward compatibility
			switch req.Level {
			case "beginner":
				instrument.LevelName = "入门级"
			case "intermediate":
				instrument.LevelName = "中级"
			case "advanced":
				instrument.LevelName = "高级"
			case "professional":
				instrument.LevelName = "专业级"
			}
			log.Printf("[DEBUG] Level '%s' not found in instrument_levels, using legacy mapping: %s", req.Level, instrument.LevelName)
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

	// Step 2: 确保 tenant_id 和 org_id 不为空
	// 确保 tenant_id 和 org_id 存在
	if instrument.TenantID == "" || instrument.OrgID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "instrument is missing tenant_id or org_id",
		})
		return
	}

	if err := db.Save(&instrument).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update instrument: " + err.Error(),
		})
		return
	}

	// Process properties if provided (delete existing and recreate)
	if req.Properties != nil && len(req.Properties) > 0 {
		// Delete existing properties for this instrument
		if err := db.Where("instrument_id = ?", instrument.ID).Delete(&models.InstrumentProperty{}).Error; err != nil {
			log.Printf("[ERROR] Failed to delete existing instrument_properties: %v", err)
			// Continue, don't fail the update
		}

		// Create new property associations
		if err := processProperties(db, instrument.ID, tenantID, req.Properties); err != nil {
			log.Printf("[ERROR] Failed to process properties: %v", err)
			// Don't fail the request if properties processing fails
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
	validStatuses := []string{"available", "unavailable", "rented", "maintenance"}
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
			"message": "invalid stock_status. Valid values: available, unavailable, rented, maintenance",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Update the instrument
	result := db.Model(&models.Instrument{}).
		Where("id = ?", instrumentID).
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
				"id":       instrument.ID,
				"site":     siteName,
				"category": categoryName,
				"brand":    instrument.Brand,
				"model":    instrument.Model,
			},
		},
	})
}

// GET /api/instruments/levels - Get instrument levels list
func GetInstrumentLevels(c *gin.Context) {
	db := database.GetDB()

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
