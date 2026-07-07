package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
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
		catIDs := getDescendantCategoryIDs(db, categoryID)
		query = query.Where("category_id IN ?", catIDs)
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

	// Batch query first display image per instrument for thumbnails
	instrumentIDs := make([]string, len(instruments))
	for i, inst := range instruments {
		instrumentIDs[i] = inst.ID
	}
	type thumbResult struct {
		InstrumentID string
		StorageKey   string
	}
	var thumbs []thumbResult
	db.Raw("SELECT DISTINCT ON (instrument_id) instrument_id, storage_key "+
		"FROM instrument_media WHERE instrument_id IN ? AND file_type = 'image' "+
		"ORDER BY instrument_id, sort_order ASC, created_at DESC",
		instrumentIDs).Scan(&thumbs)

	storage := services.NewMediaStorage()
	thumbMap := make(map[string]string)
	for _, t := range thumbs {
		key := normalizeMediaKey(t.StorageKey)
		url, _ := storage.GetURL(context.Background(), key)
		if url == "" {
			url = "/uploads/media/" + key
		}
		thumbMap[t.InstrumentID] = url
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
			"sn":              instrument.SN,
			"category_id":     instrument.CategoryID,
			"category_name":   instrument.CategoryName,
			"level_name":      instrument.LevelName,
			"level_id":        instrument.LevelID,
			"images":          instrument.Images,
			"pricing":         instrument.Pricing,
			"base_daily_rate": instrument.BaseDailyRate,
			"total_price":     instrument.TotalPrice,
			"stock_status":    instrument.StockStatus,
			"tenant_id":       instrument.TenantID,
			"tenant_name":     tenantName,
			"site_id":         instrument.SiteID,
			"site_name":       siteName,
			"site_address":    siteAddress,
			"site_phone":      sitePhone,
			"description":     instrument.Description,
			"thumbnail":       thumbMap[instrument.ID],
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
		"sn":              instrument.SN,
		"category_id":     instrument.CategoryID,
		"category_name":   instrument.CategoryName,
		"level_name":      instrument.LevelName,
		"level_id":        instrument.LevelID,
		"images":          instrument.Images,
		"cover_image":     instrument.CoverImage,
		"video":           instrument.Video,
		"poster":          instrument.Poster,
		"pricing":         instrument.Pricing,
		"base_daily_rate": instrument.BaseDailyRate,
		"total_price":     instrument.TotalPrice,
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

	// Fetch first display image for thumbnail
	var thumb models.InstrumentMedia
	if db.Where("instrument_id = ? AND file_type = 'image'", id).
		Order("sort_order asc, created_at desc").First(&thumb).Error == nil {
		storage := services.NewMediaStorage()
		key := normalizeMediaKey(thumb.StorageKey)
		url, _ := storage.GetURL(c.Request.Context(), key)
		if url == "" {
			url = "/uploads/media/" + key
		}
		response["thumbnail"] = url
		// Fallback: use first display image as cover if none set
		if response["cover_image"] == "" {
			response["cover_image"] = url
		}
	}

	// Resolve video URL from instrument_media storage_key (more reliable than instrument.Video)
	var videoURL string
	var mediaVideo models.InstrumentMedia
	if db.Where("instrument_id = ? AND file_type = 'video'", id).
		Order("created_at desc").First(&mediaVideo).Error == nil {
		storage := services.NewMediaStorage()
		key := normalizeMediaKey(mediaVideo.StorageKey)
		url, _ := storage.GetURL(c.Request.Context(), key)
		if url == "" {
			url = "/uploads/media/" + key
		}
		videoURL = url
	}
	if videoURL != "" {
		response["video"] = videoURL
	}

	// Fetch dynamic properties from instrument_properties table
	var instrumentProps []models.InstrumentProperty
	db.Where("instrument_id = ?", id).Find(&instrumentProps)
	propsMap := make(map[string][]string)
	for _, prop := range instrumentProps {
		propsMap[prop.PropertyName] = append(propsMap[prop.PropertyName], prop.Value)
	}

	// Also include all property definitions (even without assigned values)
	var propDefs []models.Property
	globalQuery := db.Where("scope_type = ?", "global")
	if instrument.CategoryID != nil {
		globalQuery = globalQuery.Or("related_category_id = ?", *instrument.CategoryID)
	}
	globalQuery.Find(&propDefs)
	for _, p := range propDefs {
		if _, exists := propsMap[p.Name]; !exists {
			propsMap[p.Name] = []string{}
		}
	}
	response["properties"] = propsMap

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

	// Check for home menu config (visible categories + sort order)
	var homeConfig models.SystemSetting
	if tenantID != "" {
		db.Where("tenant_id = ? AND setting_key = ?", tenantID, "home_menu_config").First(&homeConfig)
	} else {
		db.Where("setting_key = ? AND setting_value IS NOT NULL", "home_menu_config").First(&homeConfig)
	}
	if homeConfig.SettingValue != "" {
		type menuConfig struct {
			VisibleIDs []string       `json:"visible_ids"`
			SortOrder  map[string]int `json:"sort_order"`
		}
		var cfg menuConfig
		if err := json.Unmarshal([]byte(homeConfig.SettingValue), &cfg); err == nil && len(cfg.VisibleIDs) > 0 {
			idSet := make(map[string]bool, len(cfg.VisibleIDs))
			for _, id := range cfg.VisibleIDs {
				idSet[id] = true
			}
			filtered := make([]models.Category, 0, len(cfg.VisibleIDs))
			for _, id := range cfg.VisibleIDs {
				for _, cat := range categories {
					if cat.ID == id {
						filtered = append(filtered, cat)
						break
					}
				}
			}
			// If sort_order provided, apply it
			if cfg.SortOrder != nil && len(cfg.SortOrder) > 0 {
				sorted := make([]models.Category, len(filtered))
				for _, cat := range filtered {
					pos := cfg.SortOrder[cat.ID]
					if pos < 0 || pos >= len(filtered) {
						pos = len(filtered) - 1
					}
					for sorted[pos].ID != "" {
						pos++
						if pos >= len(filtered) {
							pos = 0
						}
					}
					sorted[pos] = cat
				}
				filtered = sorted
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 20000,
				"data": gin.H{
					"list": filtered,
				},
			})
			return
		}
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

	query := db.Where("status = ?", "active")

	if merchantID := c.Query("merchant_id"); merchantID != "" {
		var merchant models.Merchant
		if err := db.Where("id = ?", merchantID).First(&merchant).Error; err == nil {
			query = query.Where("tenant_id = ?", merchant.OrgID)
		}
	}
	if typeFilter := c.Query("type"); typeFilter != "" {
		query = query.Where("type = ?", typeFilter)
	}

	var sites []models.Site
	if err := query.Find(&sites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "Failed to fetch sites"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": sites}})
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
			config.Config = `{"deposit_mode":"ratio","deposit_multiplier":7,"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20},{"days_max":-1,"discount_percent":40}]}`
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query pricing config"})
			return
		}
	}

	totalPrice := 0.0
	if instrument.TotalPrice != nil {
		totalPrice = *instrument.TotalPrice
	}
	result := services.CalculatePricing(*instrument.BaseDailyRate, totalPrice, config.Config, instrument.PricingOverrides, instrument.Pricing)
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
	db.Where("(instrument_id = ? OR (object_type = 'instrument' AND object_id = ?)) AND (is_display = ? OR file_type = ?)", id, id, true, "video").
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
	db.Where("(instrument_id = ? OR (object_type = 'instrument' AND object_id = ?)) AND is_display = ?", id, id, true).
		Order("sort_order asc, created_at desc").
		Find(&displayMedia)

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

// getDescendantCategoryIDs returns the given category ID and all recursive descendant IDs.
func getDescendantCategoryIDs(db *gorm.DB, parentID string) []string {
	ids := []string{parentID}
	var children []struct{ ID string }
	db.Table("categories").Select("id").Where("parent_id = ?", parentID).Find(&children)
	for _, child := range children {
		ids = append(ids, getDescendantCategoryIDs(db, child.ID)...)
	}
	return ids
}

// ListPublicMerchants returns full-control merchants and a has_controlled flag.
// No auth required — used by customer-facing pages (create repair, etc.).
func ListPublicMerchants(c *gin.Context) {
	db := database.GetDB()

	var merchants []models.Merchant
	db.Where("merchant_type = ? AND status = ?", models.MerchantTypeFull, "active").
		Select("id, name, address, phone").
		Find(&merchants)

	var controlledCount int64
	db.Model(&models.Merchant{}).Where("merchant_type = ? AND status = ?", models.MerchantTypeControlled, "active").
		Count(&controlledCount)

	type merchantItem struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
	}
	list := make([]merchantItem, 0, len(merchants))
	for _, m := range merchants {
		list = append(list, merchantItem{ID: m.ID, Name: m.Name, Address: m.Address, Phone: m.Phone})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"merchants":      list,
			"has_controlled": controlledCount > 0,
		},
	})
}

// ListTransitSites returns transit site info for a controlled merchant.
func ListTransitSites(c *gin.Context) {
	merchantID := c.Param("id")
	if merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "merchant id is required"})
		return
	}

	db := database.GetDB()

	var merchant models.Merchant
	if err := db.Where("id = ?", merchantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "merchant not found"})
		return
	}

	if merchant.MerchantType != "controlled" {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"sites": []interface{}{}}})
		return
	}

	sites := []gin.H{{
		"id":           merchant.ID,
		"name":         merchant.Name + "-中转",
		"address":      merchant.TransitAddress,
		"phone":        merchant.TransitPhone,
		"contact_name": merchant.TransitContactName,
		"is_transit":   true,
	}}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"sites": sites}})
}

// LookupInstrumentBySN looks up instrument info by serial number.
// Checks both instruments and user_instruments tables.
func LookupInstrumentBySN(c *gin.Context) {
	sn := c.Query("sn")
	if sn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "sn is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var instr models.Instrument
	if err := db.Where("sn = ?", sn).First(&instr).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
			"instrument": gin.H{
				"instrument_type": instr.CategoryName,
				"brand":           "",
				"model":           "",
			},
		}})
		return
	}

	var ui models.UserInstrument
	if err := db.Where("sn = ?", sn).First(&ui).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
			"instrument": gin.H{
				"instrument_type": ui.InstrumentType,
				"brand":           ui.Brand,
				"model":           ui.Model,
			},
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"instrument": nil}})
}

// GetHomeMenuConfig returns the home page category menu configuration for a tenant.
func GetHomeMenuConfig(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var setting models.SystemSetting
	db.Where("tenant_id = ? AND setting_key = ?", tenantID, "home_menu_config").First(&setting)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"config": setting.SettingValue}})
}

// SetHomeMenuConfig saves the home page category menu configuration for a tenant.
func SetHomeMenuConfig(c *gin.Context) {
	var req struct {
		Config string `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "config required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	key := "home_menu_config"

	var setting models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, key).First(&setting).Error; err == nil {
		db.Model(&setting).Update("setting_value", req.Config)
	} else {
		db.Create(&models.SystemSetting{TenantID: tenantID, SettingKey: key, SettingValue: req.Config})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "home menu config saved"})
}
