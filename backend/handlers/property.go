package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PropertyHandler struct{}

func NewPropertyHandler() *PropertyHandler {
	return &PropertyHandler{}
}

// GET /api/properties - List all properties
func (h *PropertyHandler) ListProperties(c *gin.Context) {
	db := database.GetDB()

	var properties []models.Property
	if err := db.Find(&properties).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query properties: " + err.Error(),
		})
		return
	}

	type PropertyOptionResponse struct {
		models.PropertyOption
		DisplayValue string `json:"display_value"`
	}

	type PropertyWithOptions struct {
		models.Property
		Options []PropertyOptionResponse `json:"options"`
	}

	var result []PropertyWithOptions
	for _, prop := range properties {
		var rawOptions []models.PropertyOption
		db.Where("property_name = ? AND status != ?", prop.Name, "obsolete").Find(&rawOptions)

		options := make([]PropertyOptionResponse, 0, len(rawOptions))
		for _, opt := range rawOptions {
			displayValue := opt.Value
			if opt.Status == "pending" {
				displayValue = opt.Value + " (待审核)"
			}
			options = append(options, PropertyOptionResponse{
				PropertyOption: opt,
				DisplayValue:   displayValue,
			})
		}

		result = append(result, PropertyWithOptions{
			Property: prop,
			Options:  options,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}

// POST /api/property - Create property
func (h *PropertyHandler) CreateProperty(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())
	unscopedDB := database.GetDB()

	var req struct {
		Name               string   `json:"name" binding:"required"`
		PropertyType       string   `json:"property_type" binding:"required"`
		Unit               string   `json:"unit"`
		Description        string   `json:"description"`
		Options            []string `json:"options"`
		ScopeType          string   `json:"scope_type"`
		RelatedCategoryID  string   `json:"related_category_id"`
		RelatedPropertyID  string   `json:"related_property_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Check for duplicate name (platform-wide)
	var existingProperty models.Property
	if err := unscopedDB.Where("name = ?", req.Name).First(&existingProperty).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "属性名称已存在",
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to check duplicate name: " + err.Error(),
		})
		return
	}

	property := models.Property{
		Name:         req.Name,
		PropertyType: req.PropertyType,
		Unit:         req.Unit,
		Description:  req.Description,
		TenantID:     tenantID,
		ScopeType:    req.ScopeType,
	}
	if property.ScopeType == "" {
		property.ScopeType = "global"
	}
	if req.RelatedCategoryID != "" {
		property.RelatedCategoryID = &req.RelatedCategoryID
	}
	if req.RelatedPropertyID != "" {
		property.RelatedPropertyID = &req.RelatedPropertyID
	}

	if err := db.Create(&property).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create property: " + err.Error(),
		})
		return
	}

	// Create property options if provided
	if len(req.Options) > 0 {
		for _, optionValue := range req.Options {
			option := models.PropertyOption{
				PropertyName: property.Name,
				Value:        optionValue,
				Status:       "confirmed", // Confirmed by default as requested
				TenantID:     tenantID,
			}
			if err := db.Create(&option).Error; err != nil {
				// Log error but don't fail the whole operation
				fmt.Printf("Warning: failed to create option %s for property %s: %v\n", optionValue, property.ID, err)
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": property,
	})
}

// PUT /api/property/:id - Update property
func (h *PropertyHandler) UpdateProperty(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	unscopedDB := database.GetDB()

	propertyID := c.Param("id")

	var req struct {
		Name         string   `json:"name" binding:"required"`
		PropertyType string   `json:"property_type" binding:"required"`
		Unit         string   `json:"unit"`
		Description  string   `json:"description"`
		Options      []string `json:"options"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Find the property
	var property models.Property
	if err := unscopedDB.First(&property, "id = ?", propertyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "property not found",
		})
		return
	}

	// Check for duplicate name (platform-wide, excluding current property)
	var existingProperty models.Property
	if err := unscopedDB.Where("name = ? AND id != ?", req.Name, propertyID).First(&existingProperty).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "属性名称已存在",
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to check duplicate name: " + err.Error(),
		})
		return
	}

	// Update property fields
	property.Name = req.Name
	property.PropertyType = req.PropertyType
	property.Unit = req.Unit
	property.Description = req.Description

	if err := unscopedDB.Save(&property).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update property: " + err.Error(),
		})
		return
	}

	// If options are provided, replace existing options
	if len(req.Options) > 0 {
		// Delete existing options for this property
		unscopedDB.Where("property_name = ?", property.Name).Delete(&models.PropertyOption{})

		// Create new options
		for _, optionValue := range req.Options {
			option := models.PropertyOption{
				PropertyName: property.Name,
				Value:        optionValue,
				Status:       "confirmed",
				TenantID:     tenantID,
			}
			if err := unscopedDB.Create(&option).Error; err != nil {
				// Log error but don't fail the whole operation
				fmt.Printf("Warning: failed to create option %s for property %s: %v\n", optionValue, property.ID, err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": property,
	})
}

// POST /api/property/option - Create property option
func (h *PropertyHandler) CreatePropertyOption(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var req struct {
		PropertyID string `json:"property_id" binding:"required"`
		Value      string `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	var property models.Property
	if err := database.GetDB().First(&property, "id = ?", req.PropertyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "property not found",
		})
		return
	}

	option := models.PropertyOption{
		PropertyName: property.Name,
		Value:        req.Value,
		Status:       "pending",
		TenantID:     tenantID,
	}

	if err := db.Create(&option).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create option: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": option,
	})
}

// PUT /api/property/confirm - Confirm property value (with optional rename)
func (h *PropertyHandler) ConfirmPropertyValue(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	unscopedDB := database.GetDB()

	var req struct {
		PropertyID string `json:"property_id" binding:"required"`
		Value      string `json:"value" binding:"required"`
		NewValue   string `json:"new_value"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	var option models.PropertyOption
	if err := unscopedDB.Where("property_name = ? AND value = ?", req.PropertyID, req.Value).First(&option).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "option not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query option: " + err.Error(),
		})
		return
	}

	if option.Status == "confirmed" {
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{"message": "already confirmed"},
		})
		return
	}

	if req.NewValue != "" && req.NewValue != req.Value {
		tx := unscopedDB.Begin()

		newOption := models.PropertyOption{
			ID:               uuid.New().String(),
			TenantID:         option.TenantID,
			PropertyName:     option.PropertyName,
			Value:            req.NewValue,
			Status:           "confirmed",
			ScopeCategoryID:  option.ScopeCategoryID,
			ScopeParentValue: option.ScopeParentValue,
		}
		if err := tx.Create(&newOption).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to create renamed option: " + err.Error(),
			})
			return
		}

		aliasID := newOption.ID
		if err := tx.Model(&option).Updates(map[string]interface{}{
			"status": "abort",
			"alias":  aliasID,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to update original option: " + err.Error(),
			})
			return
		}

		if err := tx.Model(&models.InstrumentProperty{}).
			Where("property_id = ? AND value = ? AND tenant_id = ?", req.PropertyID, req.Value, tenantID).
			Update("value", req.NewValue).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to update instrument properties: " + err.Error(),
			})
			return
		}

		tx.Commit()

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{"confirmed": true, "renamed": true, "new_value": req.NewValue},
		})
		return
	}

	if err := unscopedDB.Model(&option).Update("status", "confirmed").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to confirm option: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"confirmed": true},
	})
}

// PUT /api/property/merge - Merge property values
func (h *PropertyHandler) MergePropertyValues(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	unscopedDB := database.GetDB()

	var req struct {
		PropertyID  string `json:"property_id" binding:"required"`
		SourceValue string `json:"source_value" binding:"required"`
		TargetValue string `json:"target_value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	var targetOption models.PropertyOption
	if err := unscopedDB.Where("property_name = ? AND value = ? AND status = 'confirmed'", req.PropertyID, req.TargetValue).First(&targetOption).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "target value must be in confirmed status",
		})
		return
	}

	tx := unscopedDB.Begin()

	var sourceOption models.PropertyOption
	if err := tx.Where("property_name = ? AND value = ?", req.PropertyID, req.SourceValue).First(&sourceOption).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "source option not found",
		})
		return
	}

	if err := tx.Model(&sourceOption).Updates(map[string]interface{}{
		"status": "abort",
		"alias":  targetOption.ID,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update source option: " + err.Error(),
		})
		return
	}

	if err := tx.Model(&models.InstrumentProperty{}).
		Where("property_id = ? AND value = ? AND tenant_id = ?", req.PropertyID, req.SourceValue, tenantID).
		Update("value", req.TargetValue).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update instrument properties: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"merged": true},
	})
}

// GET /api/properties/:id/options/search - Autocomplete property options with frequency sorting
func (h *PropertyHandler) SearchPropertyOptions(c *gin.Context) {
	propertyID := c.Param("id")
	searchQuery := c.Query("q")
	limitStr := c.DefaultQuery("limit", "6")
	categoryID := c.Query("category_id")
	parentValue := c.Query("parent_value")

	tenantID := middleware.GetTenantID(c.Request.Context())
	unscopedDB := database.GetDB()

	var limit int
	if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || limit <= 0 {
		limit = 6
	}
	if limit > 50 {
		limit = 50
	}

	var prop models.Property
	if err := unscopedDB.Where("id = ?", propertyID).First(&prop).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "property not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query property: " + err.Error(),
		})
		return
	}

	type SearchResult struct {
		Value     string `json:"value"`
		Status    string `json:"status"`
		Frequency int    `json:"frequency"`
	}

	var results []SearchResult
	q := unscopedDB.Table("property_options po").
		Select("po.value, po.status, COALESCE(ip_cnt.cnt, 0) AS frequency").
		Joins("LEFT JOIN (SELECT property_name, value, tenant_id, COUNT(*) AS cnt FROM instrument_properties WHERE tenant_id = ? GROUP BY property_name, value, tenant_id) ip_cnt ON ip_cnt.property_name = po.property_name AND ip_cnt.value = po.value AND ip_cnt.tenant_id = po.tenant_id",
			tenantID).
		Where("po.property_name = ? AND po.status != ?", prop.Name, "obsolete")

	if searchQuery != "" {
		q = q.Where("po.value ILIKE ?", "%"+searchQuery+"%")
	}

	if categoryID != "" {
		q = q.Where("po.scope_category_id = ?", categoryID)
	} else {
		q = q.Where("po.scope_category_id IS NULL")
	}
	if parentValue != "" {
		q = q.Where("po.scope_parent_value = ?", parentValue)
	} else {
		q = q.Where("po.scope_parent_value IS NULL")
	}

	if err := q.Order("frequency DESC, po.value ASC").Limit(limit).Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to search property options: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": results,
	})
}

// DELETE /api/properties/:id - Delete a property (default properties protected)
func (h *PropertyHandler) DeleteProperty(c *gin.Context) {
	propertyID := c.Param("id")
	db := database.GetDB()

	// Check if property exists
	var prop models.Property
	if err := db.Where("id = ?", propertyID).First(&prop).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "property not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query property: " + err.Error(),
		})
		return
	}

	// Protect default properties (brand, model)
	defaultProperties := []string{"brand", "model"}
	for _, defaultProp := range defaultProperties {
		if prop.Name == defaultProp {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40300,
				"message": "default properties (brand, model) cannot be deleted",
			})
			return
		}
	}

	// Soft delete: update status to 'deleted'
	if err := db.Model(&prop).Update("status", "deleted").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to delete property: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"deleted": true},
	})
}
