package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func getAbsPath(relativePath string) string {
	execDir, _ := os.Getwd()
	return filepath.Join(execDir, relativePath)
}

func GetInstrumentByID(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()

	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	// Get tenant_id from context
	tenantID := middleware.GetTenantID(ctx)

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "instrument not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch instrument: " + err.Error(),
		})
		return
	}

	// Parse JSON fields
	var specsArray []interface{}
	if instrument.Specifications != "" && instrument.Specifications != "{}" {
		if err := json.Unmarshal([]byte(instrument.Specifications), &specsArray); err != nil {
			specsArray = []interface{}{}
		}
	}
	if specsArray == nil {
		specsArray = []interface{}{}
	}

	instrumentMap := map[string]interface{}{
		"id":             instrument.ID,
		"tenant_id":      instrument.TenantID,
		"org_id":         instrument.OrgID,
		"category_id":    instrument.CategoryID,
		"category_name":  instrument.CategoryName,
		"name":           instrument.Name,
		"brand":          instrument.Brand,
		"level":          instrument.Level,
		"level_name":     instrument.LevelName,
		"model":          instrument.Model,
		"description":    instrument.Description,
		"images":         json.RawMessage(instrument.Images),
		"video":          instrument.Video,
		"stock_status":   instrument.StockStatus,
		"created_at":     instrument.CreatedAt,
		"updated_at":     instrument.UpdatedAt,
		"specifications": specsArray,
		"pricing":        json.RawMessage(instrument.Pricing),
	}

	// Return instrument data with parsed JSON
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instrumentMap,
	})
}

func GetInstruments(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get total count
	var total int64
	if err := db.Model(&models.Instrument{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to count instruments",
		})
		return
	}

	// Get paginated results
	var instruments []models.Instrument
	if err := db.Where("tenant_id = ?", tenantID).Offset(offset).Limit(pageSize).Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch instruments",
		})
		return
	}

	// Process instruments to parse specifications and pricing into specs array
	var responseInstruments []map[string]interface{}
	for _, instrument := range instruments {
		instrumentMap := map[string]interface{}{
			"id":             instrument.ID,
			"tenant_id":      instrument.TenantID,
			"org_id":         instrument.OrgID,
			"sn":             instrument.SN,
			"site_id":        instrument.SiteID,
			"site_name":      instrument.Site,
			"category_id":    instrument.CategoryID,
			"category_name":  instrument.CategoryName,
			"name":           instrument.Name,
			"brand":          instrument.Brand,
			"level":          instrument.Level,
			"level_name":     instrument.LevelName,
			"model":          instrument.Model,
			"description":    instrument.Description,
			"images":         json.RawMessage(instrument.Images),
			"video":          instrument.Video,
			"stock_status":   instrument.StockStatus,
			"status":         instrument.StockStatus,
			"created_at":     instrument.CreatedAt,
			"updated_at":     instrument.UpdatedAt,
			"specifications": json.RawMessage(instrument.Specifications),
			"pricing":        json.RawMessage(instrument.Pricing),
		}

		// Parse specifications JSON
		var specs []map[string]interface{}
		if instrument.Specifications != "" && instrument.Specifications != "{}" {
			if err := json.Unmarshal([]byte(instrument.Specifications), &specs); err != nil {
				log.Printf("[WARN] Failed to parse specifications for instrument %s: %v", instrument.ID, err)
			}
		}

		// If specs is empty, try parsing as object and convert to array
		if len(specs) == 0 && instrument.Specifications != "" && instrument.Specifications != "{}" {
			var specObj map[string]interface{}
			if err := json.Unmarshal([]byte(instrument.Specifications), &specObj); err == nil {
				// Try to convert to array format
				if _, ok := specObj["name"].(string); ok {
					specs = []map[string]interface{}{specObj}
				}
			}
		}

		// Parse pricing JSON and merge into specs
		if instrument.Pricing != "" && instrument.Pricing != "{}" {
			var pricing map[string]interface{}
			if err := json.Unmarshal([]byte(instrument.Pricing), &pricing); err == nil {
				// If specs is empty, create one from pricing
				if len(specs) == 0 {
					specs = []map[string]interface{}{pricing}
				} else {
					// Merge pricing into first spec (maintain backward compatibility)
					for k, v := range pricing {
						if len(specs) > 0 {
							specs[0][k] = v
						}
					}
				}
			}
		}

		// Add specifications to response
		instrumentMap["specifications"] = specs

		// Calculate total stock from specs
		totalStock := 0
		for _, spec := range specs {
			if stock, ok := spec["stock"].(float64); ok {
				totalStock += int(stock)
			} else if stock, ok := spec["stock"].(int); ok {
				totalStock += stock
			}
		}
		instrumentMap["stock"] = totalStock

		responseInstruments = append(responseInstruments, instrumentMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": responseInstruments,
		"pagination": gin.H{
			"page":       page,
			"pageSize":   pageSize,
			"total":      total,
			"totalPages": (total + int64(pageSize) - 1) / int64(pageSize),
		},
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
			"error":   err.Error(), // ADD: Detailed error
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
			"code":    40001,
			"message": "No file uploaded",
		})
		return
	}

	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}

	if !allowedTypes[file.Header.Get("Content-Type")] {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid file type. Only JPEG, PNG, GIF, WebP allowed",
		})
		return
	}

	maxSizeStr := os.Getenv("UPLOAD_MAX_SIZE")
	maxSizeMB := 10 // default 10MB
	if maxSizeStr != "" {
		if parsed, err := strconv.Atoi(maxSizeStr); err == nil && parsed > 0 {
			maxSizeMB = parsed
		}
	}
	maxSizeBytes := int64(maxSizeMB * 1024 * 1024)

	if file.Size > maxSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": fmt.Sprintf("File too large. Max size is %dMB", maxSizeMB),
		})
		return
	}

	uploadDir := getAbsPath("./uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "Failed to create upload directory",
		})
		return
	}

	ext := filepath.Ext(file.Filename)
	timestamp := time.Now().UnixNano()
	randomStr := fmt.Sprintf("%08x", rand.Int31())
	filename := fmt.Sprintf("%d_%s%s", timestamp, randomStr, ext)
	filepath := filepath.Join(uploadDir, filename)

	if err := c.SaveUploadedFile(file, filepath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "Failed to save file",
		})
		return
	}

	fileURL := fmt.Sprintf("/uploads/%s", filename)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"url":      fileURL,
			"fileName": file.Filename,
			"size":     file.Size,
		},
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
