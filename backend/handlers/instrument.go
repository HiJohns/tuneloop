package handlers

import (
	"encoding/json"
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
	Name           string                   `json:"name" binding:"required"`
	Brand          string                   `json:"brand"`
	Level          string                   `json:"level" binding:"required"`
	Model          string                   `json:"model"`
	CategoryID     string                   `json:"category_id" binding:"required"`
	Pricing        map[string]interface{}   `json:"pricing"`
	Description    string                   `json:"description"`
	Images         []string                 `json:"images"`
	Video          string                   `json:"video"`
	Specifications []map[string]interface{} `json:"specifications"`
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	log.Printf("[DEBUG CreateInstrument] Creating instrument: name=%s, brand=%s, level=%s", req.Name, req.Brand, req.Level)

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

	// Map level to level_name
	levelName := ""
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

	instrument := models.Instrument{
		TenantID:     tenantID,
		OrgID:        tenantID,
		Name:         req.Name,
		Brand:        req.Brand,
		Level:        req.Level,
		LevelName:    levelName,
		CategoryID:   req.CategoryID,
		CategoryName: categoryName,
		Description:  req.Description,
		StockStatus:  "available",
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

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id":    instrument.ID,
			"name":  instrument.Name,
			"brand": instrument.Brand,
			"level": instrument.Level,
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

	instrument.Name = req.Name
	instrument.Model = req.Model
	instrument.Brand = req.Brand
	instrument.Level = req.Level
	instrument.CategoryID = req.CategoryID
	instrument.Description = req.Description
	instrument.Video = req.Video

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

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":    instrument.ID,
			"name":  instrument.Name,
			"brand": instrument.Brand,
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
