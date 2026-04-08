package handlers

import (
	"fmt"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PropertyHandler struct{}

func NewPropertyHandler() *PropertyHandler {
	return &PropertyHandler{}
}

// GET /api/properties - List all properties
func (h *PropertyHandler) ListProperties(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var properties []models.Property
	if err := db.Where("tenant_id = ?", tenantID).Find(&properties).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query properties: " + err.Error(),
		})
		return
	}

	type PropertyWithOptions struct {
		models.Property
		Options []models.PropertyOption `json:"options"`
	}

	var result []PropertyWithOptions
	for _, prop := range properties {
		var options []models.PropertyOption
		db.Where("property_id = ?", prop.ID).Find(&options)
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

	var req struct {
		Name         string   `json:"name" binding:"required"`
		PropertyType string   `json:"property_type" binding:"required"`
		Unit         string   `json:"unit"`
		Options      []string `json:"options"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Check for duplicate name
	var existingProperty models.Property
	if err := db.Where("tenant_id = ? AND name = ?", tenantID, req.Name).First(&existingProperty).Error; err == nil {
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
		TenantID:     tenantID,
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
				PropertyID: property.ID,
				Value:      optionValue,
				Status:     "confirmed", // Confirmed by default as requested
				TenantID:   tenantID,
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
	db := database.GetDB().WithContext(c.Request.Context())

	propertyID := c.Param("id")

	var req struct {
		Name         string   `json:"name" binding:"required"`
		PropertyType string   `json:"property_type" binding:"required"`
		Unit         string   `json:"unit"`
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
	if err := db.First(&property, "id = ? AND tenant_id = ?", propertyID, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "property not found",
		})
		return
	}

	// Check for duplicate name (excluding current property)
	var existingProperty models.Property
	if err := db.Where("tenant_id = ? AND name = ? AND id != ?", tenantID, req.Name, propertyID).First(&existingProperty).Error; err == nil {
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

	if err := db.Save(&property).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update property: " + err.Error(),
		})
		return
	}

	// If options are provided, replace existing options
	if len(req.Options) > 0 {
		// Delete existing options for this property
		db.Where("property_id = ? AND tenant_id = ?", propertyID, tenantID).Delete(&models.PropertyOption{})

		// Create new options
		for _, optionValue := range req.Options {
			option := models.PropertyOption{
				PropertyID: property.ID,
				Value:      optionValue,
				Status:     "confirmed",
				TenantID:   tenantID,
			}
			if err := db.Create(&option).Error; err != nil {
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
	if err := db.First(&property, "id = ? AND tenant_id = ?", req.PropertyID, tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "property not found",
		})
		return
	}

	option := models.PropertyOption{
		PropertyID: req.PropertyID,
		Value:      req.Value,
		Status:     "pending",
		TenantID:   tenantID,
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

// PUT /api/property/confirm - Confirm property value
func (h *PropertyHandler) ConfirmPropertyValue(c *gin.Context) {
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

	var option models.PropertyOption
	if err := db.Where("property_id = ? AND value = ? AND tenant_id = ?", req.PropertyID, req.Value, tenantID).First(&option).Error; err != nil {
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

	if err := db.Model(&option).Update("status", "confirmed").Error; err != nil {
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
	db := database.GetDB().WithContext(c.Request.Context())

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
	if err := db.Where("property_id = ? AND value = ? AND status = 'confirmed' AND tenant_id = ?", req.PropertyID, req.TargetValue, tenantID).First(&targetOption).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "target value must be in confirmed status",
		})
		return
	}

	tx := db.Begin()

	var sourceOption models.PropertyOption
	if err := tx.Where("property_id = ? AND value = ? AND tenant_id = ?", req.PropertyID, req.SourceValue, tenantID).First(&sourceOption).Error; err != nil {
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
