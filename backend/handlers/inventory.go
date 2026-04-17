package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryTransfer struct {
	ID          string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AssetID     string     `gorm:"type:uuid;index;not null" json:"asset_id"`
	FromSiteID  string     `gorm:"type:uuid;not null" json:"from_site_id"`
	ToSiteID    string     `gorm:"type:uuid;not null" json:"to_site_id"`
	Reason      string     `gorm:"type:text" json:"reason"`
	Status      string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedBy   string     `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type InventoryHandler struct{}

func NewInventoryHandler() *InventoryHandler {
	return &InventoryHandler{}
}

// GET /api/merchant/inventory - List inventory by site
func (h *InventoryHandler) ListInventory(c *gin.Context) {
	siteID := c.Query("site_id")
	category := c.Query("category")
	status := c.Query("status")

	if siteID == "" {
		ctx := c.Request.Context()
		tenantID := middleware.GetTenantID(ctx)
		userRole := middleware.GetRole(ctx)

		// Check if user is Owner or Admin - allow querying all inventory
		if userRole == "OWNER" || userRole == "ADMIN" {
			// Query all instruments for the tenant (no site filter)
			var instruments []models.Instrument
			db := database.GetDB().WithContext(ctx)

			query := db.Model(&models.Instrument{}).Where("tenant_id = ?", tenantID)

			if category != "" {
				query = query.Where("category_id = ?", category)
			}
			if status != "" {
				query = query.Where("stock_status = ?", status)
			}

			if err := query.Find(&instruments).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "failed to query inventory: " + err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"code": 20000,
				"data": gin.H{
					"list":  instruments,
					"total": len(instruments),
				},
			})
			return
		}

		// For non-owner roles, require site_id
		var sites []models.Site
		db := database.GetDB().WithContext(ctx)
		if err := db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&sites).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "site_id is required",
			})
			return
		}

		siteList := make([]gin.H, len(sites))
		for i, site := range sites {
			siteList[i] = gin.H{
				"id":   site.ID,
				"name": site.Name,
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"code":            40002,
			"message":         "site_id is required",
			"available_sites": siteList,
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	query := db.Model(&models.Instrument{}).Where("current_site_id = ?", siteID)

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if status != "" {
		query = query.Where("stock_status = ?", status)
	}

	var instruments []models.Instrument
	var total int64

	query.Count(&total)
	query.Find(&instruments)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  instruments,
			"total": total,
		},
	})
}

// POST /api/merchant/inventory/transfer - Transfer asset between sites
func (h *InventoryHandler) TransferInventory(c *gin.Context) {
	var req struct {
		AssetID    string `json:"asset_id" binding:"required"`
		FromSiteID string `json:"from_site_id" binding:"required"`
		ToSiteID   string `json:"to_site_id" binding:"required"`
		Reason     string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Validate asset exists and is at from_site
	var asset models.Instrument
	if err := db.First(&asset, "id = ?", req.AssetID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "asset not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to validate asset: " + err.Error(),
		})
		return
	}

	// Validate sites exist
	var fromSite, toSite models.Site
	if err := db.First(&fromSite, "id = ?", req.FromSiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "from_site not found",
		})
		return
	}
	if err := db.First(&toSite, "id = ?", req.ToSiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "to_site not found",
		})
		return
	}

	// Create transfer record
	transfer := InventoryTransfer{
		AssetID:    req.AssetID,
		FromSiteID: req.FromSiteID,
		ToSiteID:   req.ToSiteID,
		Reason:     req.Reason,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	tx := db.Begin()
	if err := tx.WithContext(c.Request.Context()).Create(&transfer).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create transfer: " + err.Error(),
		})
		return
	}

	// Update asset current_site_id
	if err := tx.WithContext(c.Request.Context()).Model(&models.Instrument{}).Where("id = ?", req.AssetID).Update("current_site_id", req.ToSiteID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update asset location: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": gin.H{
			"transfer_id": transfer.ID,
			"asset_id":    transfer.AssetID,
			"status":      transfer.Status,
			"created_at":  transfer.CreatedAt,
		},
	})
}

// GET /api/merchant/inventory/transfers - List transfer records
func (h *InventoryHandler) ListTransfers(c *gin.Context) {
	siteID := c.Query("site_id")
	status := c.Query("status")

	db := database.GetDB().WithContext(c.Request.Context())

	var transfers []InventoryTransfer
	query := db.Model(&InventoryTransfer{})

	if siteID != "" {
		query = query.Where("from_site_id = ? OR to_site_id = ?", siteID, siteID)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Order("created_at DESC").Find(&transfers)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": transfers},
	})
}

// GET /api/inventory/rent-setting - Get inventory rent settings
func (h *InventoryHandler) GetRentSetting(c *gin.Context) {
	brand := c.Query("brand")
	model := c.Query("model")
	categoryID := c.Query("category_id")
	levelID := c.Query("level_id")
	siteID := c.Query("site_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	// Base query
	query := db.Model(&models.Instrument{}).
		Select(`instruments.id, instruments.sn, instruments.category_name, 
				instruments.level_name, instruments.site_id, instruments.pricing`).
		Where("instruments.tenant_id = ?", tenantID)

	// Apply filters
	if siteID != "" {
		query = query.Where("instruments.site_id = ?", siteID)
	}
	if categoryID != "" {
		query = query.Where("instruments.category_id = ?", categoryID)
	}
	if levelID != "" {
		query = query.Where("instruments.level_id = ?", levelID)
	}

	// Handle brand and model filters via instrument_properties
	// This is a simplified version - in production, you'd need a proper join
	if brand != "" || model != "" {
		// For now, we'll filter after fetching due to JSONB complexity
		// A better implementation would use a proper join with instrument_properties
	}

	// Get total count before pagination
	var total int64
	query.Count(&total)

	// Pagination
	offset := (page - 1) * pageSize
	var instruments []models.Instrument
	query.Offset(offset).Limit(pageSize).Find(&instruments)

	// Transform data to include brand, model, site_name, and daily_rent
	type RentSettingItem struct {
		ID           string  `json:"id"`
		SN           string  `json:"sn"`
		CategoryName string  `json:"category_name"`
		LevelName    string  `json:"level_name"`
		Brand        string  `json:"brand"`
		Model        string  `json:"model"`
		SiteName     string  `json:"site_name"`
		DailyRent    float64 `json:"daily_rent"`
	}

	var items []RentSettingItem

	for _, inst := range instruments {
		// Parse pricing JSONB
		var pricing []map[string]interface{}
		if inst.Pricing != "" {
			json.Unmarshal([]byte(inst.Pricing), &pricing)
		}

		dailyRent := 0.0
		if len(pricing) > 0 {
			if dailyRentVal, ok := pricing[0]["daily_rent"].(float64); ok {
				dailyRent = dailyRentVal
			}
		}

		// Get brand and model from instrument_properties (simplified for now)
		brand := ""
		model := ""

		// Get site name
		siteName := ""
		if inst.SiteID != nil {
			var site models.Site
			db.Select("name").First(&site, "id = ?", inst.SiteID)
			siteName = site.Name
		}

		items = append(items, RentSettingItem{
			ID:           inst.ID,
			SN:           inst.SN,
			CategoryName: inst.CategoryName,
			LevelName:    inst.LevelName,
			Brand:        brand,
			Model:        model,
			SiteName:     siteName,
			DailyRent:    dailyRent,
		})
	}

	// Apply brand/model filters after fetching (simplified approach)
	// TODO: Implement proper filtering with JOIN

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     items,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// PUT /api/inventory/rent-setting/batch - Batch update rent settings
func (h *InventoryHandler) BatchUpdateRent(c *gin.Context) {
	var req struct {
		Items []struct {
			ID        string  `json:"id" binding:"required"`
			DailyRent float64 `json:"daily_rent" binding:"required"`
		} `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	updatedCount := 0

	for _, item := range req.Items {
		// Get current instrument
		var instrument models.Instrument
		if err := db.First(&instrument, "id = ?", item.ID).Error; err != nil {
			log.Printf("[WARN] Failed to find instrument %s: %v", item.ID, err)
			continue
		}

		// Parse current pricing
		var pricing []map[string]interface{}
		if instrument.Pricing != "" {
			json.Unmarshal([]byte(instrument.Pricing), &pricing)
		}

		// Ensure pricing array exists
		if len(pricing) == 0 {
			pricing = []map[string]interface{}{
				{"name": "standard"},
			}
		}

		// Update daily_rent
		pricing[0]["daily_rent"] = item.DailyRent

		// Marshal back to JSON
		updatedPricing, err := json.Marshal(pricing)
		if err != nil {
			log.Printf("[WARN] Failed to marshal pricing for instrument %s: %v", item.ID, err)
			continue
		}

		// Update database
		if err := db.Model(&models.Instrument{}).Where("id = ?", item.ID).Update("pricing", string(updatedPricing)).Error; err != nil {
			log.Printf("[WARN] Failed to update pricing for instrument %s: %v", item.ID, err)
			continue
		}

		updatedCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"updated": updatedCount,
		},
	})
}
