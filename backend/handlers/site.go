package handlers

import (
	"math"
	"net/http"
	"strconv"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SiteHandler struct{}

func NewSiteHandler() *SiteHandler {
	return &SiteHandler{}
}

// GET /api/common/sites - List sites
func (h *SiteHandler) ListSites(c *gin.Context) {
	var sites []models.Site
	db := database.GetDB().WithContext(c.Request.Context())

	query := db.Model(&models.Site{})

	if city := c.Query("city"); city != "" {
		// Note: city filter would require city field in Site model
		// Skipping for now as not in current model
	}

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", "active")
	}

	var total int64
	query.Count(&total)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	offset := (page - 1) * pageSize
	query.WithContext(c.Request.Context()).Offset(offset).Limit(pageSize).Find(&sites)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  sites,
			"total": total,
		},
	})
}

// GET /api/common/sites/nearby - Find nearby sites
func (h *SiteHandler) GetNearbySites(c *gin.Context) {
	latStr := c.Query("lat")
	lngStr := c.Query("lng")
	radiusStr := c.DefaultQuery("radius", "5000")

	if latStr == "" || lngStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "lat and lng are required",
		})
		return
	}

	lat, _ := strconv.ParseFloat(latStr, 64)
	lng, _ := strconv.ParseFloat(lngStr, 64)
	radius, _ := strconv.Atoi(radiusStr)

	var sites []models.Site
	db := database.GetDB().WithContext(c.Request.Context())

	// Haversine formula to calculate distance
	subQuery := db.Select("*, (6371 * acos(cos(radians(?)) * cos(radians(latitude)) * cos(radians(longitude) - radians(?)) + sin(radians(?)) * sin(radians(latitude)))) AS distance", lat, lng, lat)

	err := db.Raw("SELECT * FROM (?) AS sites_with_distance WHERE distance <= ? ORDER BY distance", subQuery, float64(radius)/1000).Find(&sites).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query nearby sites: " + err.Error(),
		})
		return
	}

	// Calculate distances for response
	type NearbySite struct {
		models.Site
		Distance float64 `json:"distance"`
	}

	var result []NearbySite
	for _, site := range sites {
		distance := calculateDistance(lat, lng, site.Latitude, site.Longitude)
		result = append(result, NearbySite{
			Site:     site,
			Distance: distance * 1000, // Convert to meters
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": result},
	})
}

// GET /api/common/sites/:id - Get site details
func (h *SiteHandler) GetSiteDetail(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "site id is required",
		})
		return
	}

	var site models.Site
	db := database.GetDB().WithContext(c.Request.Context())

	if err := db.First(&site, "id = ?", siteID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "site not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to get site: " + err.Error(),
		})
		return
	}

	// Get stock status - this would be aggregated from instruments
	// Mock response for now
	stockStatus := map[string]interface{}{
		"piano": map[string]int{
			"available":   12,
			"renting":     8,
			"maintenance": 2,
		},
		"violin": map[string]int{
			"available":   5,
			"renting":     3,
			"maintenance": 0,
		},
	}

	// Get images
	var images []string
	db.WithContext(c.Request.Context()).Raw("SELECT url FROM site_images WHERE site_id = ? ORDER BY sort_order", siteID).Scan(&images)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"site":         site,
			"stock_status": stockStatus,
			"images":       images,
		},
	})
}

// POST /api/merchant/sites - Create site
func (h *SiteHandler) CreateSite(c *gin.Context) {
	var req struct {
		Name          string  `json:"name" binding:"required"`
		Address       string  `json:"address"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Phone         string  `json:"phone"`
		BusinessHours string  `json:"business_hours"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	site := models.Site{
		Name:          req.Name,
		Address:       req.Address,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		Phone:         req.Phone,
		BusinessHours: req.BusinessHours,
		Status:        "active",
	}

	if err := db.Create(&site).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create site: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": site,
	})
}

// PUT /api/merchant/sites/:id - Update site
func (h *SiteHandler) UpdateSite(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "site id is required",
		})
		return
	}

	var req struct {
		Name          string  `json:"name"`
		Address       string  `json:"address"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Phone         string  `json:"phone"`
		BusinessHours string  `json:"business_hours"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	result := db.Model(&models.Site{}).Where("id = ?", siteID).Updates(map[string]interface{}{
		"name":           req.Name,
		"address":        req.Address,
		"latitude":       req.Latitude,
		"longitude":      req.Longitude,
		"phone":          req.Phone,
		"business_hours": req.BusinessHours,
		"updated_at":     gorm.Expr("NOW()"),
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update site: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "site not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"updated": true},
	})
}

// DELETE /api/merchant/sites/:id - Soft delete
func (h *SiteHandler) DeleteSite(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "site id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	result := db.Model(&models.Site{}).Where("id = ?", siteID).Update("status", "closed")

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to delete site: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "site not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"deleted": true},
	})
}

// Helper function to calculate distance between two points
func calculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371 // Earth radius in km

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}
