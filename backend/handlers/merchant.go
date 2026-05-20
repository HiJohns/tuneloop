package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MerchantHandler struct{}

func NewMerchantHandler() *MerchantHandler {
	return &MerchantHandler{}
}

// ListMerchants GET /api/merchants - List merchants (system_admin only)
func (h *MerchantHandler) ListMerchants(c *gin.Context) {
	merchants := make([]models.Merchant, 0)
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	query := db.Model(&models.Merchant{}).Where("tenant_id = ?", tenantID)

	// Apply status filter if provided
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Pagination
	var total int64
	query.Count(&total)

	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("pageSize", "20"), 20)
	offset := (page - 1) * pageSize

	query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&merchants)

	// Batch load admin user info
	adminMap := make(map[string]models.User)
	var adminUIDs []string
	for _, m := range merchants {
		if m.AdminUID != "" {
			adminUIDs = append(adminUIDs, m.AdminUID)
		}
	}
	if len(adminUIDs) > 0 {
		var adminUsers []models.User
		db.Where("id IN ? AND deleted_at IS NULL", adminUIDs).Find(&adminUsers)
		for _, u := range adminUsers {
			adminMap[u.ID] = u
		}
	}

	// Build list with admin info
	var list []gin.H
	for _, m := range merchants {
		item := gin.H{
			"id":            m.ID,
			"tenant_id":     m.TenantID,
			"org_id":        m.OrgID,
			"name":          m.Name,
			"code":          m.Code,
			"contact_name":  m.ContactName,
			"contact_email": m.ContactEmail,
			"contact_phone": m.ContactPhone,
			"phone":         m.Phone,
			"address":       m.Address,
			"admin_uid":     m.AdminUID,
			"status":        m.Status,
			"created_at":    m.CreatedAt,
			"updated_at":    m.UpdatedAt,
		}
		if u, ok := adminMap[m.AdminUID]; ok {
			item["admin_name"] = u.Name
			item["admin_email"] = u.Email
			item["admin_phone"] = u.Phone
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     list,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// GetMerchant GET /api/merchants/:id - Get merchant detail
func (h *MerchantHandler) GetMerchant(c *gin.Context) {
	id := c.Param("id")
	var merchant models.Merchant
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	result := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&merchant)
	if result.Error == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return
	}
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Database error: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": merchant,
	})
}

// CreateMerchant POST /api/merchants - Create a new merchant
func (h *MerchantHandler) CreateMerchant(c *gin.Context) {
	var input struct {
		Name          string   `json:"name" binding:"required"`
		Phone         string   `json:"phone"`
		Address       string   `json:"address"`
		AdminUID      string   `json:"admin_uid"`
		AdminName     string   `json:"admin_name"`
		AdminUsername string   `json:"admin_username"`
		AdminEmail    string   `json:"admin_email"`
		AdminPhone    string   `json:"admin_phone"`
		UserIDs       []map[string]interface{} `json:"user_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	var count int64
	db.Model(&models.Merchant{}).Where("tenant_id = ? AND name = ?", tenantID, input.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "商户名已存在",
		})
		return
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)

	callbackHost := os.Getenv("TUNELOOP_EXTERNAL_URL")
	if callbackHost == "" {
		callbackHost = c.Request.Host
	}
	callbackURL := fmt.Sprintf("https://%s/api/iam/confirmation-callback", callbackHost)

	var adminUserID string
	var userIDsToProcess []map[string]interface{}

	if input.AdminUID != "" && len(input.UserIDs) == 0 {
		adminUserID = input.AdminUID
		userIDsToProcess = []map[string]interface{}{{"user_id": input.AdminUID, "action_type": "merchant_admin"}}
	} else if input.AdminName != "" && input.AdminEmail != "" && len(input.UserIDs) == 0 {
		// Scenario 2: create IAM user first, then use the returned ID
		userResult, err := iamClient.CreateOrGetUser(userToken, &services.CreateUserRequest{
			Username:    input.AdminUsername,
			Name:        input.AdminName,
			Email:       input.AdminEmail,
			Phone:       input.AdminPhone,
			CallbackURL: callbackURL,
			OperatorID:  middleware.GetUserID(c.Request.Context()),
		})
		if err != nil {
			log.Printf("[CreateMerchant] CreateOrGetUser failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to create admin user: " + err.Error(),
			})
			return
		}
		if userResult.Conflict {
			c.JSON(http.StatusConflict, gin.H{
				"code": 40901,
				"data": gin.H{
					"conflicts": userResult.ExistingUsers,
				},
			})
			return
		}
		adminUserID = userResult.UserID
		userIDsToProcess = []map[string]interface{}{{"user_id": userResult.UserID, "action_type": "merchant_admin"}}
	} else if len(input.UserIDs) > 0 {
		adminUserID = input.UserIDs[0]["user_id"].(string)
		userIDsToProcess = input.UserIDs
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Either admin_uid (old format) or user_ids (new format) must be provided",
		})
		return
	}

	var adminName, adminEmail, adminPhone string
	var adminUser models.User
	if adminUserID != "" {
		if _, err := uuid.Parse(adminUserID); err == nil {
			if err := db.Where("id = ?", adminUserID).First(&adminUser).Error; err == nil {
				adminName = adminUser.Name
				adminEmail = adminUser.Email
				adminPhone = adminUser.Phone
			}
		}
	}
	if adminName == "" && input.AdminName != "" && input.AdminEmail != "" {
		adminName = input.AdminName
		adminEmail = input.AdminEmail
		adminPhone = input.AdminPhone
	}

	var iamOrgID string
	adminUserName := adminName
	if input.AdminUsername != "" {
		adminUserName = input.AdminUsername
	}
	orgResp, err := iamClient.CreateOrganizationWithToken(userToken, &services.CreateOrganizationRequest{
		Name:        input.Name,
		Address:     input.Address,
		NamespaceID: middleware.GetNamespaceID(c.Request.Context()),
		AdminInfo: &services.OrganizationAdmin{
			Name:     adminName,
			Username: adminUserName,
			Email:    adminEmail,
			Phone:    adminPhone,
		},
		CallbackURL: callbackURL,
		OperatorID:  middleware.GetUserID(c.Request.Context()),
	})
	if err != nil {
		if strings.Contains(err.Error(), "name conflict") {
			orgs, listErr := iamClient.ListOrganizations()
			if listErr == nil {
				for _, org := range orgs {
					if org.Name == input.Name {
						iamOrgID = org.ID
						break
					}
				}
			}
		}
		if iamOrgID == "" {
			log.Printf("[CreateMerchant] IAM CreateOrganization failed: %v", err)
			c.JSON(http.StatusConflict, gin.H{
				"code":    40900,
				"message": "Merchant name conflict: " + err.Error(),
			})
			return
		}
		// Name-conflict recovery: adminUserID is still client temp ID, clear it
		if _, parseErr := uuid.Parse(adminUserID); parseErr != nil {
			adminUserID = uuid.Nil.String()
		}
	} else {
		iamOrgID = orgResp.OrgID
		if _, parseErr := uuid.Parse(adminUserID); parseErr != nil && orgResp.AdminID != "" {
			adminUserID = orgResp.AdminID
		}
	}
	if iamOrgID == "" {
		iamOrgID = middleware.GetOrgID(c.Request.Context())
	}

	merchant := models.Merchant{
		TenantID:     tenantID,
		OrgID:        iamOrgID,
		Name:         input.Name,
		Code:         input.Name,
		Phone:         input.Phone,
		Address:       input.Address,
		AdminUID:      adminUserID,
		Status:       "active",
	}

	result := db.Create(&merchant)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create merchant: " + result.Error.Error(),
		})
		return
	}

	if adminUserID != "" && iamOrgID != "" {
		iamUserID := adminUserID
		if orgResp != nil && orgResp.AdminID != "" {
			iamUserID = orgResp.AdminID
		} else {
			var adminUser models.User
			if err := db.Where("id = ?", adminUserID).First(&adminUser).Error; err == nil && adminUser.IAMSub != "" {
				iamUserID = adminUser.IAMSub
			}
		}
		operatorID := middleware.GetUserID(c.Request.Context())
		if bindErr := iamClient.BindUserToOrganization(iamUserID, iamOrgID, "OWNER", operatorID); bindErr != nil {
			log.Printf("[CreateMerchant] BindUserToOrganization failed for admin %s to org %s: %v", iamUserID, iamOrgID, bindErr)
		}
	}

	var directlyAdded []string
	for _, userEntry := range userIDsToProcess {
		if userID, ok := userEntry["user_id"].(string); ok && userID != "" {
			directlyAdded = append(directlyAdded, userID)
		}
	}

	responseData := gin.H{
		"id":             merchant.ID,
		"name":           merchant.Name,
		"code":           merchant.Code,
		"iam_org_id":     iamOrgID,
		"admin_uid":      adminUserID,
		"directly_added": directlyAdded,
		"callback_url":   callbackURL,
	}
	if orgResp != nil {
		responseData["iam_admin_id"] = orgResp.AdminID
	}
	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": responseData,
	})
}

// UpdateMerchant PUT /api/merchants/:id - Update merchant
func (h *MerchantHandler) UpdateMerchant(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Address string `json:"address"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check if merchant exists
	var merchant models.Merchant
	result := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&merchant)
	if result.Error == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return
	}

	// Update fields
	if input.Name != "" {
		merchant.Name = input.Name
	}
	if input.Phone != "" {
		merchant.Phone = input.Phone
	}
	if input.Address != "" {
		merchant.Address = input.Address
	}

	result = db.Save(&merchant)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update merchant: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": merchant,
	})
}

// DeleteMerchant DELETE /api/merchants/:id - Delete merchant
func (h *MerchantHandler) DeleteMerchant(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check if merchant exists
	var merchant models.Merchant
	result := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&merchant)
	if result.Error == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return
	}

	// Check for active sites
	var siteCount int64
	db.Model(&models.Site{}).Where("org_id = ? AND status = ?", merchant.OrgID, "active").Count(&siteCount)
	if siteCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Cannot delete merchant with active sites",
		})
		return
	}

	// Check for incomplete orders (paid or in_lease status)
	var orderCount int64
	db.Model(&models.Order{}).Where("org_id = ? AND status IN (?)", merchant.OrgID, []string{"paid", "in_lease"}).Count(&orderCount)
	if orderCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Cannot delete merchant with incomplete orders",
		})
		return
	}

	// Soft delete by setting status to inactive
	merchant.Status = "inactive"
	result = db.Save(&merchant)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to delete merchant: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Merchant deleted successfully",
	})
}

// Helper function to parse integer with default
func parseInt(s string, defaultValue int) int {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	if err != nil {
		return defaultValue
	}
	return result
}
