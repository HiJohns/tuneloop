package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func GetInstruments(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var instruments []models.Instrument
	if err := db.Where("tenant_id = ?", tenantID).Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch instruments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instruments,
	})
}

func GetCategories(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var categories []models.Category
	if err := db.Where("tenant_id = ? AND visible = ?", tenantID, true).
		Order("sort ASC").
		Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch categories",
		})
		return
	}

	categoryMap := make(map[string]map[string]interface{})
	var result []map[string]interface{}

	for _, cat := range categories {
		categoryData := map[string]interface{}{
			"id":        cat.ID,
			"name":      cat.Name,
			"icon":      cat.Icon,
			"level":     cat.Level,
			"sort":      cat.Sort,
			"visible":   cat.Visible,
			"parent_id": cat.ParentID,
		}

		if cat.ParentID == nil {
			categoryData["sub_categories"] = []map[string]interface{}{}
			categoryMap[cat.ID] = categoryData
			result = append(result, categoryData)
		}
	}

	for _, cat := range categories {
		if cat.ParentID != nil {
			if parent, exists := categoryMap[*cat.ParentID]; exists {
				if subCats, ok := parent["sub_categories"].([]map[string]interface{}); ok {
					parent["sub_categories"] = append(subCats, map[string]interface{}{
						"id":      cat.ID,
						"name":    cat.Name,
						"icon":    cat.Icon,
						"level":   cat.Level,
						"sort":    cat.Sort,
						"visible": cat.Visible,
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}

// CreateCategory creates a new category
func CreateCategory(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Name    string `json:"name" binding:"required"`
		Icon    string `json:"icon"`
		Level   int    `json:"level"`
		Visible bool   `json:"visible"`
		Sort    int    `json:"sort"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	category := models.Category{
		TenantID: tenantID,
		Name:     req.Name,
		Icon:     req.Icon,
		Level:    req.Level,
		Visible:  req.Visible,
		Sort:     req.Sort,
	}

	if err := db.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create category",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20100,
		"data":    category,
		"message": "Category created successfully",
	})
}

func GetSites(c *gin.Context) {
	c.File("data/sites.json")
}

func HandleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No file uploaded",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"fileName": file.Filename,
		"url":      "https://dummy.tuneloop.com/uploads/mock-image.jpg",
		"size":     file.Size,
	})
}

// GetOverdueLeases returns overdue lease data (replaces the old abnormal work orders API)
func GetOverdueLeases(c *gin.Context) {
	overdueLeases := []gin.H{
		{
			"id":              "LEASE-001",
			"instrument_name": "雅马哈 U1 立式钢琴",
			"renter_name":     "张三",
			"lease_end_date":  "2026-03-15",
			"overdue_days":    3,
			"contact":         "138****1234",
			"status":          "逾期",
		},
		{
			"id":              "LEASE-002",
			"instrument_name": "卡马 F1 民谣吉他",
			"renter_name":     "李四",
			"lease_end_date":  "2026-03-10",
			"overdue_days":    8,
			"contact":         "139****5678",
			"status":          "逾期",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  overdueLeases,
		"total": len(overdueLeases),
	})
}
