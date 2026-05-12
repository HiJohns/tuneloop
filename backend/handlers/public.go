package handlers

import (
	"net/http"
	"strconv"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetPublicInstruments(c *gin.Context) {
	db := database.GetDB()

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	categoryID := c.Query("category_id")
	siteID := c.Query("site_id")
	levelID := c.Query("level_id")
	tenantID := c.Query("tenant")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	query := db.Model(&models.Instrument{})
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if siteID != "" {
		query = query.Where("current_site_id = ?", siteID)
	}
	if levelID != "" {
		query = query.Where("level_id = ?", levelID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to count instruments",
		})
		return
	}

	var instruments []models.Instrument
	if err := query.Offset(offset).Limit(pageSize).Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch instruments",
		})
		return
	}

	var responseInstruments []map[string]interface{}
	for _, instrument := range instruments {
		siteName := "-"
		siteAddress := "-"
		if instrument.SiteID != nil {
			var site models.Site
			if err := db.First(&site, "id = ?", instrument.SiteID).Error; err == nil {
				siteName = site.Name
				if site.Address != "" {
					siteAddress = site.Address
				}
			}
		}

		// Get tenant name
		tenantName := ""
		if instrument.TenantID != "" {
			var tenant models.Tenant
			if err := db.First(&tenant, "id = ?", instrument.TenantID).Error; err == nil {
				tenantName = tenant.Name
			}
		}

		responseInstruments = append(responseInstruments, map[string]interface{}{
			"id":              instrument.ID,
			"sn":              instrument.SN,
			"category_id":    instrument.CategoryID,
			"category_name":  instrument.CategoryName,
			"level_name":     instrument.LevelName,
			"level_id":       instrument.LevelID,
			"images":          instrument.Images,
			"pricing":        instrument.Pricing,
			"stock_status":   instrument.StockStatus,
			"tenant_id":      instrument.TenantID,
			"tenant_name":    tenantName,
			"site_id":        instrument.SiteID,
			"site_name":      siteName,
			"site_address":   siteAddress,
			"description":    instrument.Description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     responseInstruments,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

func GetPublicInstrumentByID(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Instrument not found",
		})
		return
	}

	// Get site info (name and address) - fallback to CurrentSiteID if SiteID is nil
	siteName := "-"
	siteAddress := "-"
	lookupSiteID := instrument.SiteID
	if lookupSiteID == nil {
		lookupSiteID = instrument.CurrentSiteID
	}
	if lookupSiteID != nil {
		var site models.Site
		if err := db.First(&site, "id = ?", lookupSiteID).Error; err == nil {
			siteName = site.Name
			if site.Address != "" {
				siteAddress = site.Address
			}
		}
	}

	// Get tenant name (fallback to empty if not found)
	tenantName := ""
	if instrument.TenantID != "" {
		var tenant models.Tenant
		if err := db.First(&tenant, "id = ?", instrument.TenantID).Error; err == nil {
			tenantName = tenant.Name
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": map[string]interface{}{
			"id":              instrument.ID,
			"sn":               instrument.SN,
			"category_id":     instrument.CategoryID,
			"category_name":   instrument.CategoryName,
			"level_name":      instrument.LevelName,
			"level_id":        instrument.LevelID,
			"images":          instrument.Images,
			"video":           instrument.Video,
			"pricing":         instrument.Pricing,
			"stock_status":    instrument.StockStatus,
			"tenant_id":       instrument.TenantID,
			"tenant_name":     tenantName,
			"site_id":         instrument.SiteID,
			"site_name":       siteName,
			"site_address":    siteAddress,
			"description":     instrument.Description,
		},
	})
}

func GetPublicCategories(c *gin.Context) {
	db := database.GetDB()

	var categories []models.Category
	if err := db.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch categories",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": categories,
		},
	})
}

func GetPublicSites(c *gin.Context) {
	db := database.GetDB()

	var sites []models.Site
	if err := db.Where("status = ?", "active").Find(&sites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch sites",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": sites,
		},
	})
}
