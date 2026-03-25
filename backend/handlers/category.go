package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

type CategoryHandler struct {
	db *gorm.DB
}

func NewCategoryHandler(db *gorm.DB) *CategoryHandler {
	return &CategoryHandler{db: db}
}

// GetCategories - GET /api/categories
func (h *CategoryHandler) GetCategories(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "tenant_id is required",
		})
		return
	}

	var categories []models.Category
	result := h.db.Where("tenant_id = ?", tenantID).
		Order("parent_id NULLS FIRST, created_at ASC").
		Find(&categories)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch categories",
			"error":   result.Error.Error(),
		})
		return
	}

	// Build tree structure
	categoryMap := make(map[string]*models.Category)
	var rootCategories []gin.H

	for i := range categories {
		categoryMap[categories[i].ID] = &categories[i]
	}

	for _, category := range categories {
		if category.ParentID == nil {
			subCategories := getSubCategories(category.ID, categoryMap)
			rootCategories = append(rootCategories, gin.H{
				"id":             category.ID,
				"name":           category.Name,
				"icon":           category.Icon,
				"parent_id":      category.ParentID,
				"tenant_id":      category.TenantID,
				"created_at":     category.CreatedAt,
				"sub_categories": subCategories,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": rootCategories,
	})
}

// CreateCategory - POST /api/categories
func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "tenant_id is required",
		})
		return
	}

	var req struct {
		Name     string  `json:"name" binding:"required"`
		Icon     string  `json:"icon"`
		ParentID *string `json:"parent_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	category := models.Category{
		Name:     req.Name,
		Icon:     req.Icon,
		ParentID: req.ParentID,
		TenantID: tenantID,
	}

	result := h.db.Create(&category)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create category",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": category,
	})
}

// UpdateCategory - PUT /api/categories/:id
func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	categoryID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if categoryID == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "category_id and tenant_id are required",
		})
		return
	}

	var category models.Category
	result := h.db.Where("id = ? AND tenant_id = ?", categoryID, tenantID).First(&category)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "category not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch category",
			"error":   result.Error.Error(),
		})
		return
	}

	var req struct {
		Name     string  `json:"name"`
		Icon     string  `json:"icon"`
		ParentID *string `json:"parent_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	if req.Name != "" {
		category.Name = req.Name
	}
	if req.Icon != "" {
		category.Icon = req.Icon
	}
	if req.ParentID != nil {
		category.ParentID = req.ParentID
	}

	result = h.db.Save(&category)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update category",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": category,
	})
}

// DeleteCategory - DELETE /api/categories/:id
func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	categoryID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if categoryID == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "category_id and tenant_id are required",
		})
		return
	}

	// Check if category has sub-categories
	var subCategoryCount int64
	h.db.Model(&models.Category{}).Where("parent_id = ?", categoryID).Count(&subCategoryCount)

	if subCategoryCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "cannot delete category with sub-categories",
		})
		return
	}

	// Check if category is referenced by instruments
	var instrumentCount int64
	h.db.Model(&models.Instrument{}).Where("category_id = ?", categoryID).Count(&instrumentCount)

	if instrumentCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "cannot delete category referenced by instruments",
		})
		return
	}

	var category models.Category
	result := h.db.Where("id = ? AND tenant_id = ?", categoryID, tenantID).First(&category)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "category not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch category",
			"error":   result.Error.Error(),
		})
		return
	}

	result = h.db.Delete(&category)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to delete category",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":         categoryID,
			"deleted":    true,
			"deleted_at": category.CreatedAt, // Using same field to avoid extra import
		},
	})
}

// Helper function to get subcategories
func getSubCategories(parentID string, categoryMap map[string]*models.Category) []gin.H {
	var sub []gin.H

	for _, category := range categoryMap {
		if category.ParentID != nil && *category.ParentID == parentID {
			subSub := getSubCategories(category.ID, categoryMap)
			sub = append(sub, gin.H{
				"id":             category.ID,
				"name":           category.Name,
				"icon":           category.Icon,
				"parent_id":      category.ParentID,
				"tenant_id":      category.TenantID,
				"created_at":     category.CreatedAt,
				"sub_categories": subSub,
			})
		}
	}

	return sub
}
