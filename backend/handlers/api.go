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
	"tuneloop-backend/services"
)

func getAbsPath(relativePath string) string {
	execDir, _ := os.Getwd()
	return filepath.Join(execDir, relativePath)
}

func GetInstrumentByID(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

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

	// Get site_name and site_address from Site table
	var siteName = "-"
	var siteAddress = ""
	if instrument.SiteID != nil {
		var site models.Site
		if err := db.First(&site, "id = ?", instrument.SiteID).Error; err == nil {
			siteName = site.Name
			siteAddress = site.Address
		}
	}

	transitInfo := GetMerchantTransitInfo(c.Request.Context(), instrument.TenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		siteAddress = transitInfo.Address
	}

	// Compute pricing fallback: if instrument has no explicit pricing, use CalculatePricing with merchant defaults
	pricingField := json.RawMessage(instrument.Pricing)
	if instrument.Pricing == "" || instrument.Pricing == "{}" || instrument.Pricing == "[]" {
		var mConfig models.MerchantPricingConfig
		configJSON := "{}"
		if err := db.Where("tenant_id = ?", instrument.TenantID).First(&mConfig).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				var defaultTemplate models.PricingTemplate
				if err2 := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err2 == nil {
					configJSON = defaultTemplate.ConfigSchema
				}
			}
		} else {
			configJSON = mConfig.Config
		}
		baseRate := 0.0
		if instrument.BaseDailyRate != nil {
			baseRate = *instrument.BaseDailyRate
		}
		computed := services.CalculatePricing(baseRate, configJSON, instrument.PricingOverrides)
		dailyRent := 0.0
		if len(computed.Tiers) > 0 {
			dailyRent = computed.Tiers[0].DailyRate
		}
		computedArr := []map[string]interface{}{
			{"daily_rent": dailyRent, "deposit": computed.Deposit, "shipping_fee": computed.ShippingFee},
		}
		raw, _ := json.Marshal(computedArr)
		pricingField = json.RawMessage(raw)
	}

	instrumentMap := map[string]interface{}{
		"id":              instrument.ID,
		"tenant_id":       instrument.TenantID,
		"org_id":          instrument.OrgID,
		"category_id":     instrument.CategoryID,
		"category_name":   instrument.CategoryName,
		"sn":              instrument.SN,
		"level_id":        instrument.LevelID,
		"level_name":      instrument.LevelName,
		"site_id":         instrument.SiteID,
		"site_name":       siteName,
		"site_address":    siteAddress,
		"description":     instrument.Description,
		"images":          json.RawMessage(instrument.Images),
		"video":           instrument.Video,
		"poster":          instrument.Poster,
		"base_daily_rate": instrument.BaseDailyRate,
		"stock_status":    instrument.StockStatus,
		"status":          instrument.StockStatus,
		"created_at":      instrument.CreatedAt,
		"updated_at":      instrument.UpdatedAt,
		"specifications":  specsArray,
		"pricing":         pricingField,
	}

	// Fetch dynamic properties from instrument_properties table
	var instrumentProps []models.InstrumentProperty
	if err := db.Where("instrument_id = ?", instrumentID).Find(&instrumentProps).Error; err == nil {
		// Group by property_name and collect values
		propsMap := make(map[string][]string)
		for _, prop := range instrumentProps {
			propsMap[prop.PropertyName] = append(propsMap[prop.PropertyName], prop.Value)
		}
		instrumentMap["properties"] = propsMap
	} else {
		instrumentMap["properties"] = map[string]interface{}{}
	}

	// Fetch booker info for reserved instruments (staff only)
	if instrument.StockStatus == models.StockStatusRented {
		var order models.Order
		if err := db.Where("instrument_id = ?", instrumentID).
			Where("status NOT IN ?", []string{models.OrderStatusCancelled, models.OrderStatusCompleted}).
			Order("created_at DESC").
			Limit(1).
			First(&order).Error; err == nil {
			// Get user info
			var user models.User
			if err := db.First(&user, "id = ?", order.UserID).Error; err == nil {
				instrumentMap["booker_name"] = user.Name
				instrumentMap["booker_phone"] = user.Phone
				instrumentMap["booker_email"] = user.Email
			}
			// Try to get delivery address from lease_sessions
			var leaseSession struct {
				DeliveryAddress string
			}
			if err := db.Table("lease_sessions").Select("delivery_address").Where("order_id = ?", order.ID).First(&leaseSession).Error; err == nil {
				instrumentMap["delivery_address"] = leaseSession.DeliveryAddress
			}
		}
	}

	// Return instrument data with parsed JSON
	instrumentMap["transit_info"] = nil
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		instrumentMap["transit_info"] = map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		}
	}

	// Fetch media from instrument_media table
	var mediaList []models.InstrumentMedia
	if err := db.Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
		Order("sort_order asc, created_at desc").
		Find(&mediaList).Error; err == nil && len(mediaList) > 0 {
		type mediaItem struct {
			BatchID   string `json:"batch_id"`
			BatchType string `json:"batch_type"`
			FileType  string `json:"file_type"`
			URL       string `json:"url"`
			ThumbURL  string `json:"thumb_url,omitempty"`
			SortOrder int    `json:"sort_order"`
		}
		type batchInfo struct {
			BatchID   string `json:"batch_id"`
			BatchType string `json:"batch_type"`
			Count     int    `json:"count"`
			CreatedAt string `json:"created_at"`
		}
		var displayItems []mediaItem
		var videoItem *mediaItem
		batchesMap := make(map[string]*batchInfo)
		thumbMap := make(map[string]string)
		for _, m := range mediaList {
			url := "/uploads/media/" + m.StorageKey
			if m.FileType == "video_thumb" {
				thumbMap[m.BatchID] = url
				continue
			}
			item := mediaItem{
				BatchID:   m.BatchID,
				BatchType: m.BatchType,
				FileType:  m.FileType,
				URL:       url,
				SortOrder: m.SortOrder,
			}
			if m.IsDisplay && m.FileType != "video" {
				displayItems = append(displayItems, item)
			}
			if m.FileType == "video" {
				videoItem = &item
			}
			if _, ok := batchesMap[m.BatchID]; !ok {
				batchesMap[m.BatchID] = &batchInfo{
					BatchID:   m.BatchID,
					BatchType: m.BatchType,
					CreatedAt: m.CreatedAt.Format(time.RFC3339),
				}
			}
			batchesMap[m.BatchID].Count++
		}
		if videoItem != nil {
			if thumbURL, ok := thumbMap[videoItem.BatchID]; ok {
				videoItem.ThumbURL = thumbURL
			}
		}
		var batches []batchInfo
		for _, b := range batchesMap {
			batches = append(batches, *b)
		}
		instrumentMap["media"] = gin.H{
			"display": displayItems,
			"batches": batches,
			"video":   videoItem,
		}
	} else {
		instrumentMap["media"] = gin.H{
			"display": []interface{}{},
			"batches": []interface{}{},
			"video":   nil,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instrumentMap,
	})
}

func GetInstruments(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	query := db.Model(&models.Instrument{})

	tenantID := middleware.GetTenantID(ctx)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
		query = scopedDB
	}

	if sn := c.Query("sn"); sn != "" {
		query = query.Where("sn ILIKE ?", "%"+sn+"%")
	}
	if categoryID := c.Query("category_id"); categoryID != "" {
		var childIDs []string
		db.WithContext(ctx).
			Model(&models.Category{}).
			Where("parent_id = ? OR id = ?", categoryID, categoryID).
			Pluck("id", &childIDs)
		if len(childIDs) > 0 {
			query = query.Where("category_id IN ?", childIDs)
		} else {
			query = query.Where("category_id = ?", categoryID)
		}
	}
	if levelID := c.Query("level_id"); levelID != "" {
		query = query.Where("level_id = ?", levelID)
	}
	if stockStatus := c.Query("stock_status"); stockStatus != "" {
		query = query.Where("stock_status = ?", stockStatus)
	}

	sortParam := c.DefaultQuery("sort", "-created_at")
	orderClause := "created_at DESC"
	if sortParam == "created_at" || sortParam == "+created_at" {
		orderClause = "created_at ASC"
	}
	query = query.Order(orderClause)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to count instruments",
		})
		return
	}

	// Get paginated results
	var instruments []models.Instrument
	if err := query.Offset(offset).Limit(pageSize).Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch instruments",
		})
		return
	}

	// Build thumbnail map from InstrumentMedia (first image per instrument)
	thumbMap := make(map[string]string)
	var instIDs []string
	for _, inst := range instruments {
		instIDs = append(instIDs, inst.ID)
	}
	var allMedia []models.InstrumentMedia
	if len(instIDs) > 0 {
		db.Where("instrument_id IN ? AND file_type = ?", instIDs, "image").Order("sort_order asc, created_at desc").Find(&allMedia)
		for _, m := range allMedia {
			if _, exists := thumbMap[m.InstrumentID]; !exists {
				key := normalizeMediaKey(m.StorageKey)
				storageSvc := services.NewMediaStorage()
				url, _ := storageSvc.GetURL(ctx, key)
				if url == "" {
					url = "/uploads/media/" + key
				}
				thumbMap[m.InstrumentID] = url
			}
		}
	}

	// Process instruments to parse specifications and pricing into specs array
	var responseInstruments []map[string]interface{}
	for _, instrument := range instruments {
		// Get site_name from Site table if SiteID exists
		var siteName = "-"
		if instrument.SiteID != nil {
			var site models.Site
			if err := db.First(&site, "id = ?", instrument.SiteID).Error; err == nil {
				siteName = site.Name
			}
		}
		// Get category_name from Category table if CategoryID exists
		var catName string
		if instrument.CategoryID != nil {
			var cat models.Category
			if err := database.GetDB().First(&cat, "id = ?", *instrument.CategoryID).Error; err == nil {
				catName = cat.Name
			}
		}

		instrumentMap := map[string]interface{}{
			"id":              instrument.ID,
			"tenant_id":       instrument.TenantID,
			"org_id":          instrument.OrgID,
			"sn":              instrument.SN,
			"site_id":         instrument.SiteID,
			"site_name":       siteName,
			"category_id":     instrument.CategoryID,
			"category_name":   catName,
			"level_id":        instrument.LevelID,
			"level_name":      instrument.LevelName,
			"description":     instrument.Description,
			"images":          json.RawMessage(instrument.Images),
			"thumbnail":       thumbMap[instrument.ID],
			"video":           instrument.Video,
			"poster":          instrument.Poster,
			"base_daily_rate": instrument.BaseDailyRate,
			"stock_status":    instrument.StockStatus,
			"status":          instrument.StockStatus,
			"created_at":      instrument.CreatedAt,
			"updated_at":      instrument.UpdatedAt,
			"specifications":  json.RawMessage(instrument.Specifications),
			"pricing":         json.RawMessage(instrument.Pricing),
		}

		// Fetch dynamic properties from instrument_properties table
		var instrumentProps []models.InstrumentProperty
		if err := db.Where("instrument_id = ?", instrument.ID).Find(&instrumentProps).Error; err == nil {
			propsMap := make(map[string][]string)
			for _, prop := range instrumentProps {
				propsMap[prop.PropertyName] = append(propsMap[prop.PropertyName], prop.Value)
			}
			instrumentMap["properties"] = propsMap
		} else {
			instrumentMap["properties"] = map[string]interface{}{}
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
		"data": gin.H{
			"list":       responseInstruments,
			"total":      total,
			"page":       page,
			"pageSize":   pageSize,
			"totalPages": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	})
}

func GetInstrumentFilterOptions(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	query := db.Model(&models.Instrument{}).Where("tenant_id = ?", tenantID)
	if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
		query = scopedDB
	}

	type categoryOption struct {
		CategoryID   string `json:"category_id"`
		CategoryName string `json:"category_name"`
	}
	var categories []categoryOption
	database.GetDB().Model(&models.Category{}).
		Select("id AS category_id, name AS category_name").
		Find(&categories)

	type levelOption struct {
		LevelID   string `json:"level_id"`
		LevelName string `json:"level_name"`
	}
	var levels []levelOption
	db.Raw(`SELECT DISTINCT i.level_id, l.caption AS level_name
		FROM instruments i
		JOIN instrument_levels l ON l.id = i.level_id
		WHERE i.tenant_id = ? AND i.level_id IS NOT NULL`, tenantID).Scan(&levels)

	type statusOption struct {
		Value string `gorm:"column:stock_status" json:"value"`
	}
	var statuses []statusOption
	query.Select("DISTINCT stock_status").Find(&statuses)

	type siteOption struct {
		SiteID   string `json:"site_id"`
		SiteName string `json:"site_name"`
	}
	var sites []siteOption
	db.Raw(`SELECT DISTINCT i.site_id, s.name AS site_name
		FROM instruments i
		JOIN sites s ON s.id = i.site_id
		WHERE i.tenant_id = ? AND i.site_id IS NOT NULL`, tenantID).Scan(&sites)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"categories": categories,
			"levels":     levels,
			"statuses":   statuses,
			"sites":      sites,
		},
	})
}

func GetCategories(c *gin.Context) {
	db := database.GetDB()

	var categories []models.Category
	if err := db.Order("sort ASC").
		Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch categories",
		})
		return
	}

	categoryMap := make(map[string]map[string]interface{})
	result := make([]map[string]interface{}, 0)

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
		"data": gin.H{"list": result},
	})
}

// GetCategoryChildren retrieves direct children of a category for TreeSelect
func GetCategoryChildren(c *gin.Context) {
	db := database.GetDB()
	parentID := c.Param("id")

	var categories []models.Category
	query := db.Model(&models.Category{})

	if parentID == "0" || parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", parentID)
	}

	if err := query.Order("sort ASC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch category children",
		})
		return
	}

	var result []map[string]interface{}
	for _, cat := range categories {
		hasChildren := false
		var count int64
		db.Model(&models.Category{}).Where("parent_id = ?", cat.ID).Count(&count)
		if count > 0 {
			hasChildren = true
		}

		result = append(result, map[string]interface{}{
			"id":          cat.ID,
			"name":        cat.Name,
			"icon":        cat.Icon,
			"level":       cat.Level,
			"sort":        cat.Sort,
			"visible":     cat.Visible,
			"parent_id":   cat.ParentID,
			"isLeaf":      !hasChildren,
			"hasChildren": hasChildren,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": result},
	})
}

// GetCategoryByID gets a single category by ID
func GetCategoryByID(c *gin.Context) {
	db := database.GetDB()
	categoryID := c.Param("id")

	var category models.Category
	if err := db.Where("id = ?", categoryID).First(&category).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "Category not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch category",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": category,
	})
}

// CreateCategory creates a new category
func CreateCategory(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	unscopedDB := database.GetDB()
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Name     string  `json:"name" binding:"required"`
		Icon     string  `json:"icon"`
		Level    int     `json:"level"`
		Visible  bool    `json:"visible"`
		Sort     int     `json:"sort"`
		ParentID *string `json:"parent_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data: " + err.Error(),
			"error":   err.Error(), // ADD: Detailed error
		})
		return
	}

	// Check name uniqueness (platform-wide)
	var existingCategory models.Category
	if err := unscopedDB.Where("name = ?", req.Name).First(&existingCategory).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "分类名称已存在",
		})
		return
	}

	category := models.Category{
		TenantID: tenantID,
		Name:     req.Name,
		Icon:     req.Icon,
		Visible:  req.Visible,
		Sort:     req.Sort,
		ParentID: req.ParentID,
	}

	// Auto-calculate level based on parent_id
	if req.ParentID != nil && *req.ParentID != "" {
		category.Level = 2
		// Auto-set sort to max+1 for level 2 categories
		var maxSort int
		unscopedDB.Model(&models.Category{}).
			Where("parent_id = ?", *req.ParentID).
			Select("COALESCE(MAX(sort), 0)").Scan(&maxSort)
		category.Sort = maxSort + 1
	} else {
		category.Level = 1
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

// UpdateCategory updates an existing category
func UpdateCategory(c *gin.Context) {
	unscopedDB := database.GetDB()
	categoryID := c.Param("id")

	var req struct {
		Name     string  `json:"name"`
		Icon     string  `json:"icon"`
		Level    int     `json:"level"`
		Visible  bool    `json:"visible"`
		Sort     int     `json:"sort"`
		ParentID *string `json:"parent_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	// Check name uniqueness (platform-wide, exclude self)
	var existingCategory models.Category
	if err := unscopedDB.Where("name = ? AND id != ?", req.Name, categoryID).First(&existingCategory).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "分类名称已存在",
		})
		return
	}

	// Build update map
	updates := map[string]interface{}{
		"name":    req.Name,
		"icon":    req.Icon,
		"visible": req.Visible,
	}

	// Only update parent_id/level when explicitly provided in request
	if req.ParentID != nil {
		updates["parent_id"] = req.ParentID
		if *req.ParentID != "" {
			updates["level"] = 2
		} else {
			updates["level"] = 1
		}
	}

	if req.Sort > 0 {
		updates["sort"] = req.Sort
	}

	if err := unscopedDB.Model(&models.Category{}).Where("id = ?", categoryID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update category",
		})
		return
	}

	// Fetch updated category
	var category models.Category
	if err := unscopedDB.Where("id = ?", categoryID).First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Category not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"data":    category,
		"message": "Category updated successfully",
	})
}

// DeleteCategory deletes a category
func DeleteCategory(c *gin.Context) {
	unscopedDB := database.GetDB()
	categoryID := c.Param("id")

	// Check if category has children
	var childCount int64
	unscopedDB.Model(&models.Category{}).Where("parent_id = ?", categoryID).Count(&childCount)
	if childCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "该分类下存在子分类，无法删除",
		})
		return
	}

	// Check if any instruments use this category (cross-tenant check)
	var instrumentCount int64
	unscopedDB.Model(&models.Instrument{}).Where("category_id = ?", categoryID).Count(&instrumentCount)
	if instrumentCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "该分类下存在乐器，无法删除",
		})
		return
	}

	// Delete category
	if err := unscopedDB.Where("id = ?", categoryID).Delete(&models.Category{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to delete category",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Category deleted successfully",
	})
}

// UpdateCategorySort batch updates category sort order
func UpdateCategorySort(c *gin.Context) {
	db := database.GetDB()

	var req struct {
		Items []struct {
			ID   string `json:"id" binding:"required"`
			Sort int    `json:"sort" binding:"required"`
		} `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	tx := db.Begin()
	for _, item := range req.Items {
		if err := tx.Model(&models.Category{}).
			Where("id = ?", item.ID).
			Update("sort", item.Sort).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to update category sort",
			})
			return
		}
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Category sort updated successfully",
	})
}

func GetSites(c *gin.Context) {
	c.File("data/sites.json")
}

func HandleUpload(c *gin.Context) {
	c.Request.ParseMultipartForm(100 << 20)
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "No file uploaded",
		})
		return
	}

	mimeType := file.Header.Get("Content-Type")
	isImage := mimeType == "image/jpeg" || mimeType == "image/png" || mimeType == "image/gif" || mimeType == "image/webp"
	isVideo := mimeType == "video/mp4" || mimeType == "video/webm" || mimeType == "video/quicktime"

	if !isImage && !isVideo {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid file type. Only JPEG, PNG, GIF, WebP, MP4, WebM, MOV allowed",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())
	maxSizeBytes := int64(10 * 1024 * 1024)

	var imageMaxSize, videoMaxSize string
	if tenantID != "" {
		var s models.SystemSetting
		if err := db.Where("tenant_id = ? AND setting_key = 'upload_image_max_size'", tenantID).First(&s).Error; err == nil {
			imageMaxSize = s.SettingValue
		}
		if err := db.Where("tenant_id = ? AND setting_key = 'upload_video_max_size'", tenantID).First(&s).Error; err == nil {
			videoMaxSize = s.SettingValue
		}
	} else {
		imageMaxSize = os.Getenv("UPLOAD_IMAGE_MAX_SIZE")
		videoMaxSize = os.Getenv("UPLOAD_VIDEO_MAX_SIZE")
	}

	if isImage {
		if imageMaxSize != "" {
			if parsed, err := strconv.Atoi(imageMaxSize); err == nil && parsed > 0 {
				maxSizeBytes = int64(parsed * 1024 * 1024)
			}
		} else if envSize := os.Getenv("UPLOAD_MAX_SIZE"); envSize != "" {
			if parsed, err := strconv.Atoi(envSize); err == nil && parsed > 0 {
				maxSizeBytes = int64(parsed * 1024 * 1024)
			}
		}
	} else {
		maxSizeBytes = int64(100 * 1024 * 1024)
		if videoMaxSize != "" {
			if parsed, err := strconv.Atoi(videoMaxSize); err == nil && parsed > 0 {
				maxSizeBytes = int64(parsed * 1024 * 1024)
			}
		}
	}

	if file.Size > maxSizeBytes {
		maxMB := maxSizeBytes / 1024 / 1024
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": fmt.Sprintf("File too large. Max size is %dMB", maxMB),
		})
		return
	}

	ext := filepath.Ext(file.Filename)
	timestamp := time.Now().UnixNano()
	randomStr := fmt.Sprintf("%08x", rand.Int31())
	filename := fmt.Sprintf("%d_%s%s", timestamp, randomStr, ext)

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "Failed to open uploaded file",
		})
		return
	}
	defer reader.Close()

	storage := services.NewMediaStorage()
	if err := storage.Upload(c.Request.Context(), filename, reader, mimeType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50003,
			"message": "Failed to save file: " + err.Error(),
		})
		return
	}

	fileURL, _ := storage.GetURL(c.Request.Context(), filename)
	if fileURL == "" {
		fileURL = fmt.Sprintf("/uploads/media/%s", filename)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"url":      fileURL,
			"file_key": filename,
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
