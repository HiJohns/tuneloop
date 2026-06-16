package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// resolveTenantName resolves tenant name from local DB with IAM fallback
func resolveTenantName(db *gorm.DB, tenantID string) string {
	if tenantID == "" {
		return ""
	}
	var tenant models.Tenant
	if err := db.First(&tenant, "id = ?", tenantID).Error; err == nil {
		return tenant.Name
	}
	// Fallback to IAM
	iamClient := services.NewIAMClient()
	org, err := iamClient.GetOrganization(tenantID)
	if err != nil || org == nil {
		return ""
	}
	// Async sync to local DB for future requests
	go func() {
		syncDB := database.GetDB()
		if err := syncDB.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.Tenant{ID: tenantID, Name: org.Name}).Error; err != nil {
			log.Printf("[resolveTenantName] Failed to sync tenant %s: %v", tenantID, err)
		}
	}()
	return org.Name
}

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
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&instruments).Error; err != nil {
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
		sitePhone := "-"
		if instrument.SiteID != nil {
			var site models.Site
			if err := db.First(&site, "id = ?", instrument.SiteID).Error; err == nil {
				siteName = site.Name
				if site.Address != "" {
					siteAddress = site.Address
				}
				if site.Phone != "" {
					sitePhone = site.Phone
				}
			}
		}

		instrTransitInfo := GetMerchantTransitInfo(c.Request.Context(), instrument.TenantID)
		if instrTransitInfo != nil && instrTransitInfo.MerchantType == models.MerchantTypeControlled {
			siteAddress = instrTransitInfo.Address
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
			"name":            instrument.Name,
			"sn":              instrument.SN,
			"category_id":    instrument.CategoryID,
			"category_name":  instrument.CategoryName,
			"level_name":     instrument.LevelName,
			"level_id":       instrument.LevelID,
			"images":          instrument.Images,
			"pricing":        instrument.Pricing,
			"base_daily_rate": instrument.BaseDailyRate,
			"stock_status":   instrument.StockStatus,
			"tenant_id":      instrument.TenantID,
			"tenant_name":    tenantName,
			"site_id":        instrument.SiteID,
			"site_name":      siteName,
			"site_address":   siteAddress,
			"site_phone":     sitePhone,
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

	// Get site info (name, address, phone) - fallback to CurrentSiteID if SiteID is nil
	siteName := "-"
	siteAddress := "-"
	sitePhone := "-"
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
			if site.Phone != "" {
				sitePhone = site.Phone
			}
		}
	}

	transitInfo := GetMerchantTransitInfo(c.Request.Context(), instrument.TenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		siteAddress = transitInfo.Address
	}

	// Get tenant name with IAM fallback
	tenantName := resolveTenantName(db, instrument.TenantID)

	response := map[string]interface{}{
		"id":              instrument.ID,
		"name":            instrument.Name,
		"sn":               instrument.SN,
		"category_id":     instrument.CategoryID,
		"category_name":   instrument.CategoryName,
		"level_name":      instrument.LevelName,
		"level_id":        instrument.LevelID,
		"images":          instrument.Images,
		"video":           instrument.Video,
		"pricing":         instrument.Pricing,
		"base_daily_rate": instrument.BaseDailyRate,
		"stock_status":    instrument.StockStatus,
		"tenant_id":       instrument.TenantID,
		"tenant_name":     tenantName,
		"site_id":         instrument.SiteID,
		"site_name":       siteName,
		"site_address":    siteAddress,
		"site_phone":      sitePhone,
		"description":     instrument.Description,
		"specifications":  instrument.Specifications,
	}

	// Fetch dynamic properties from instrument_properties table
	var instrumentProps []models.InstrumentProperty
	if err := db.Where("instrument_id = ?", id).Find(&instrumentProps).Error; err == nil {
		propsMap := make(map[string][]string)
		for _, prop := range instrumentProps {
			propsMap[prop.PropertyName] = append(propsMap[prop.PropertyName], prop.Value)
		}
		response["properties"] = propsMap
	} else {
		response["properties"] = map[string]interface{}{}
	}

	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		response["transit_info"] = map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		}
	} else {
		response["transit_info"] = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": response,
	})
}

func GetPublicCategories(c *gin.Context) {
	db := database.GetDB()
	tenantID := c.Query("tenant")

	var categories []models.Category
	query := db.Model(&models.Category{})
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Find(&categories).Error; err != nil {
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

// GET /api/public/instruments/:id/pricing-v2 — Public pricing info (no auth)
func GetPublicInstrumentPricingV2(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument id is required"})
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "invalid instrument id format"})
		return
	}

	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	// Fallback: extract base_daily_rate from JSONB pricing field (object or array format)
	if instrument.BaseDailyRate == nil {
		var dailyRent float64
		var pricing map[string]interface{}
		if err := json.Unmarshal([]byte(instrument.Pricing), &pricing); err == nil {
			if v, ok := pricing["daily_rent"].(float64); ok && v > 0 {
				dailyRent = v
			}
		} else {
			var arr []map[string]interface{}
			if err := json.Unmarshal([]byte(instrument.Pricing), &arr); err == nil && len(arr) > 0 {
				if v, ok := arr[0]["daily_rent"].(float64); ok && v > 0 {
					dailyRent = v
				}
			}
		}
		if dailyRent > 0 {
			instrument.BaseDailyRate = &dailyRent
		}
	}

	if instrument.BaseDailyRate == nil || *instrument.BaseDailyRate <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "instrument has no base daily rate configured"})
		return
	}

	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", instrument.TenantID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var defaultTemplate models.PricingTemplate
			if err2 := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err2 != nil {
				c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "no pricing template found"})
				return
			}
			config.Config = defaultTemplate.ConfigSchema
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query pricing config"})
			return
		}
	}

	result := services.CalculatePricing(*instrument.BaseDailyRate, config.Config, instrument.PricingOverrides)
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": services.FormatPricingResult(result),
	})
}

// GET /api/public/instruments/:id/media — Public instrument media (no auth)
func GetPublicInstrumentMedia(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	var mediaList []models.InstrumentMedia
	db.Where("instrument_id = ? AND (is_display = ? OR file_type = ?)", id, true, "video").
		Order("sort_order asc, created_at desc").
		Find(&mediaList)

	type mediaItem struct {
		URL      string `json:"url"`
		ThumbURL string `json:"thumb_url,omitempty"`
		FileType string `json:"file_type"`
	}

	storage := services.NewMediaStorage()
	var images []mediaItem
	var video *mediaItem

	for _, m := range mediaList {
		key := normalizeMediaKey(m.StorageKey)
		url, _ := storage.GetURL(c.Request.Context(), key)
		if url == "" {
			url = "/uploads/media/" + key
		}

		item := mediaItem{URL: url, FileType: m.FileType}

		if m.FileType == "video" {
			var thumb models.InstrumentMedia
			if db.Where("instrument_id = ? AND batch_id = ? AND file_type = ?", id, m.BatchID, "video_thumb").First(&thumb).Error == nil {
				thumbURL, _ := storage.GetURL(c.Request.Context(), thumb.StorageKey)
				if thumbURL != "" {
					item.ThumbURL = thumbURL
				}
			}
			video = &item
		} else if m.FileType != "video_thumb" {
			images = append(images, item)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"images": images,
			"video":  video,
		},
	})
}

// GET /api/public/instruments/:id/display-media — Return display images, fallback to latest archives
func GetPublicInstrumentDisplayMedia(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	var displayMedia []models.InstrumentMedia
	db.Where("instrument_id = ? AND is_display = ?", id, true).
		Order("sort_order asc, created_at desc").
		Find(&displayMedia)

	// If no display images, fallback to latest archive batch
	if len(displayMedia) == 0 {
		var latestBatch models.InstrumentMedia
		if err := db.Where("instrument_id = ? AND file_type != ?", id, "video_thumb").
			Order("created_at desc").First(&latestBatch).Error; err == nil {
			// Get all media from that batch
			db.Where("instrument_id = ? AND batch_id = ? AND file_type != ?", id, latestBatch.BatchID, "video_thumb").
				Order("sort_order asc, created_at desc").
				Find(&displayMedia)
		}
	}

	storage := services.NewMediaStorage()
	type mediaItem struct {
		URL      string `json:"url"`
		ThumbURL string `json:"thumb_url,omitempty"`
		FileType string `json:"file_type"`
	}
	var images []mediaItem
	var video *mediaItem

	for _, m := range displayMedia {
		key := normalizeMediaKey(m.StorageKey)
		url, _ := storage.GetURL(c.Request.Context(), key)
		if url == "" {
			url = "/uploads/media/" + key
		}

		item := mediaItem{URL: url, FileType: m.FileType}

		if m.FileType == "video" {
			var thumb models.InstrumentMedia
			if db.Where("instrument_id = ? AND batch_id = ? AND file_type = ?", id, m.BatchID, "video_thumb").First(&thumb).Error == nil {
				thumbURL, _ := storage.GetURL(c.Request.Context(), thumb.StorageKey)
				if thumbURL != "" {
					item.ThumbURL = thumbURL
				}
			}
			video = &item
		} else if m.FileType != "video_thumb" {
			images = append(images, item)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"images": images,
			"video":  video,
		},
	})
}

func normalizeMediaKey(storageKey string) string {
	if strings.HasPrefix(storageKey, "/uploads/media/") {
		return strings.TrimPrefix(storageKey, "/uploads/media/")
	}
	if strings.HasPrefix(storageKey, "uploads/media/") {
		return strings.TrimPrefix(storageKey, "uploads/media/")
	}
	return storageKey
}
