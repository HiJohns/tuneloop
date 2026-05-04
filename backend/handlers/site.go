package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	tenantID := middleware.GetTenantID(c.Request.Context())

	query := db.Model(&models.Site{}).Where("tenant_id = ?", tenantID)

	// Apply org scope for data isolation (site_member only sees their org)
	if scopedDB, err := middleware.ApplyOrgScope(query, c.Request.Context()); err == nil {
		query = scopedDB
	}

	if status := c.Query("status"); status != "" {
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

	// Get manager info if manager_id exists
	var managerInfo *map[string]interface{}
	if site.ManagerID != nil {
		user, err := lookupOrSyncManager(c, db, site.ManagerID.String())
		if err != nil {
			fmt.Printf("[WARN] Failed to lookup manager %s: %v\n", site.ManagerID.String(), err)
		} else if user != nil {
			// Prioritize email, fallback to phone if email is empty
			displayContact := user.Email
			if displayContact == "" {
				displayContact = user.Phone
			}
			m := map[string]interface{}{
				"id":    user.ID,
				"name":  user.Name,
				"email": user.Email,
				"phone": user.Phone,
			}
			managerInfo = &m
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"site":         site,
			"manager":      managerInfo, // 新增：负责人信息
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
		Type          string  `json:"type"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Phone         string  `json:"phone"`
		BusinessHours string  `json:"business_hours"`
		ParentID      *string `json:"parent_id"`
		ManagerID     *string `json:"manager_id"`
		AdminName     string  `json:"admin_name"`
		AdminEmail    string  `json:"admin_email"`
		AdminPhone    string  `json:"admin_phone"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())
	orgID := middleware.GetOrgID(c.Request.Context())

	var parentUUID *uuid.UUID
	if req.ParentID != nil && *req.ParentID != "" {
		uuidVal, err := uuid.Parse(*req.ParentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "invalid parent_id format: " + err.Error(),
			})
			return
		}
		parentUUID = &uuidVal
	}

	var managerUUID *uuid.UUID
	if req.ManagerID != nil && *req.ManagerID != "" {
		uuidVal, err := uuid.Parse(*req.ManagerID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "invalid manager_id format: " + err.Error(),
			})
			return
		}
		managerUUID = &uuidVal
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)

	iamReq := &services.CreateOrganizationRequest{
		Name:        req.Name,
		ParentID:    orgID,
		NamespaceID: middleware.GetNamespaceID(c.Request.Context()),
		Address:     req.Address,
		OperatorID:  middleware.GetUserID(c.Request.Context()),
	}

	orgResp, err := iamClient.CreateOrganizationWithToken(userToken, iamReq)
	if err != nil {
		log.Printf("[CreateSite] IAM CreateOrganization failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create sub-organization in IAM: " + err.Error(),
		})
		return
	}

	siteOrgID := orgResp.OrgID
	if siteOrgID == "" {
		siteOrgID = orgID
	}

	operatorID := middleware.GetUserID(c.Request.Context())

	if managerUUID != nil {
		if req.AdminEmail != "" {
			createUserReq := &services.CreateUserRequest{
				Username: req.AdminEmail,
				Name:     req.AdminName,
				Email:    req.AdminEmail,
				Phone:    req.AdminPhone,
			}
			if _, createErr := iamClient.CreateUser(createUserReq); createErr != nil {
				log.Printf("[CreateSite] IAM CreateUser for manager %s: %v — will attempt bind", req.AdminEmail, createErr)
			}
		}
		if bindErr := iamClient.BindUserToOrganizationWithToken(userToken, managerUUID.String(), siteOrgID, "manager", operatorID); bindErr != nil {
			log.Printf("[CreateSite] IAM BindUser failed for manager %s to org %s: %v", managerUUID.String(), siteOrgID, bindErr)
		}
	}

	site := models.Site{
		Name:          req.Name,
		TenantID:      tenantID,
		OrgID:         siteOrgID,
		Address:       req.Address,
		Type:          req.Type,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		Phone:         req.Phone,
		BusinessHours: req.BusinessHours,
		ParentID:      parentUUID,
		ManagerID:     managerUUID,
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
		"data": gin.H{
			"id":           site.ID,
			"site":         site,
			"iam_org_id":   siteOrgID,
			"iam_admin_id": orgResp.AdminID,
		},
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
		Type          string  `json:"type"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Phone         string  `json:"phone"`
		BusinessHours string  `json:"business_hours"`
		ManagerID     *string `json:"manager_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Convert manager_id string to uuid if provided
	var managerUUID *uuid.UUID
	if req.ManagerID != nil {
		if *req.ManagerID == "" {
			// Empty string means clear manager
			managerUUID = nil
		} else {
			uuidVal, err := uuid.Parse(*req.ManagerID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    40002,
					"message": "invalid manager_id format: " + err.Error(),
				})
				return
			}
			managerUUID = &uuidVal
		}
	}

	db := database.GetDB().WithContext(c.Request.Context())

	result := db.Model(&models.Site{}).Where("id = ?", siteID).Updates(map[string]interface{}{
		"name":           req.Name,
		"address":        req.Address,
		"type":           req.Type,
		"latitude":       req.Latitude,
		"longitude":      req.Longitude,
		"phone":          req.Phone,
		"business_hours": req.BusinessHours,
		"manager_id":     managerUUID,
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
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check if site has children
	var childCount int64
	db.Model(&models.Site{}).Where("parent_id = ?", siteID).Count(&childCount)
	if childCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "该网点下存在子网点，无法删除",
		})
		return
	}

	// Enhanced check: Check for available instruments
	var availableCount int64
	db.Model(&models.Instrument{}).
		Where("site_id = ? AND stock_status = ?", siteID, "available").
		Count(&availableCount)
	if availableCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": fmt.Sprintf("该网点下存在 %d 件在库乐器，请先转移资产", availableCount),
		})
		return
	}

	// Enhanced check: Check for rented instruments
	var rentedCount int64
	db.Model(&models.Instrument{}).
		Where("site_id = ? AND stock_status = ?", siteID, "rented").
		Count(&rentedCount)
	if rentedCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": fmt.Sprintf("该网点下存在 %d 件在租乐器，请先处理在租订单", rentedCount),
		})
		return
	}

	// Enhanced check: Check for site members
	var memberCount int64
	db.Model(&models.SiteMember{}).
		Where("site_id = ?", siteID).
		Count(&memberCount)
	if memberCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": fmt.Sprintf("该网点下存在 %d 名成员，请先移除所有成员", memberCount),
		})
		return
	}

	// Soft delete by setting status to closed
	result := db.Model(&models.Site{}).Where("id = ? AND tenant_id = ?", siteID, tenantID).Update("status", "closed")

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

// lookupOrSyncManager looks up a manager by ID, falling back to IAM if not found locally
func lookupOrSyncManager(ctx *gin.Context, db *gorm.DB, managerID string) (*models.User, error) {
	var user models.User

	// First, try to find user in local database
	if err := db.First(&user, "id = ?", managerID).Error; err == nil {
		// User found locally
		return &user, nil
	}

	// User not found locally, query IAM
	fmt.Printf("[DEBUG] Manager %s not found locally, querying IAM\n", managerID)

	// Get IAM base URL
	iamBaseURL := getEnv("BEACONIAM_INTERNAL_URL", "http://localhost:5551")

	// Create IAM lookup request
	url := fmt.Sprintf("%s/api/v1/users/%s", iamBaseURL, managerID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM request: %w", err)
	}

	// Add authentication header if available
	if token, err := ctx.Cookie("token"); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read IAM response: %w", err)
	}

	// Check status code
	if resp.StatusCode == http.StatusNotFound {
		// User not found in IAM either
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IAM returned status %d", resp.StatusCode)
	}

	// Parse IAM response
	var iamResp struct {
		Code int `json:"code"`
		Data struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email,omitempty"`
			Phone string `json:"phone,omitempty"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &iamResp); err != nil {
		return nil, fmt.Errorf("failed to parse IAM response: %w", err)
	}

	if iamResp.Code != 20000 {
		return nil, fmt.Errorf("IAM returned code %d", iamResp.Code)
	}

	// Create shadow user in local database
	tenantID := middleware.GetTenantID(ctx.Request.Context())
	orgID := middleware.GetOrgID(ctx.Request.Context())

	user = models.User{
		ID:       iamResp.Data.ID,
		IAMSub:   iamResp.Data.ID, // Use ID as IAM sub for shadow users
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     iamResp.Data.Name,
		Email:    iamResp.Data.Email,
		Phone:    iamResp.Data.Phone,
		IsShadow: true,
	}

	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create shadow user: %w", err)
	}

	fmt.Printf("[DEBUG] Created shadow user for manager %s\n", managerID)
	return &user, nil
}

// Helper to get environment variable with default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GET /api/sites/tree - Get site tree structure
func (h *SiteHandler) GetSiteTree(c *gin.Context) {
	rootID := c.Query("root")
	tenantID := middleware.GetTenantID(c.Request.Context())

	db := database.GetDB().WithContext(c.Request.Context())

	var sites []models.Site
	query := db.Where("tenant_id = ? AND status = 'active'", tenantID)

	if rootID != "" {
		// Return direct children of the given root (not root itself)
		query = query.Where("parent_id = ?", rootID)
	} else {
		// Return top-level sites only
		query = query.Where("parent_id IS NULL")
	}

	if err := query.Find(&sites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query sites: " + err.Error(),
		})
		return
	}

	type SiteTreeNode struct {
		ID          string                  `json:"id"`
		Name        string                  `json:"name"`
		Address     string                  `json:"address"`
		Type        string                  `json:"type"`
		ParentID    *string                 `json:"parent_id"`
		Manager     *map[string]interface{} `json:"manager"`
		IsLeaf      bool                    `json:"isLeaf"`
		HasChildren bool                    `json:"hasChildren"`
		Children    []SiteTreeNode          `json:"children"`
	}

	var result []SiteTreeNode
	for _, site := range sites {
		var manager *map[string]interface{}
		if site.ManagerID != nil {
			user, err := lookupOrSyncManager(c, db, site.ManagerID.String())
			if err == nil && user != nil {
				m := map[string]interface{}{"id": user.ID, "name": user.Name}
				manager = &m
			}
		}

		var parentID *string
		if site.ParentID != nil {
			pid := site.ParentID.String()
			parentID = &pid
		}

		hasChildren := false
		var childCount int64
		db.Model(&models.Site{}).Where("parent_id = ? AND tenant_id = ? AND status = 'active'", site.ID, tenantID).Count(&childCount)
		if childCount > 0 {
			hasChildren = true
		}

		result = append(result, SiteTreeNode{
			ID:          site.ID,
			Name:        site.Name,
			Address:     site.Address,
			Type:        site.Type,
			ParentID:    parentID,
			Manager:     manager,
			IsLeaf:      !hasChildren,
			HasChildren: hasChildren,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": result},
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
