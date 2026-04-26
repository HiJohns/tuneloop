package handlers

import (
	"fmt"
	"log"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MerchantHandler struct{}

func NewMerchantHandler() *MerchantHandler {
	return &MerchantHandler{}
}

// ListMerchants GET /api/merchants - List merchants (project_admin only)
func (h *MerchantHandler) ListMerchants(c *gin.Context) {
	role := middleware.GetRole(c.Request.Context())
	if role != "project_admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Only project admin can access merchant management",
		})
		return
	}

	var merchants []models.Merchant
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

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     merchants,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// GetMerchant GET /api/merchants/:id - Get merchant detail
func (h *MerchantHandler) GetMerchant(c *gin.Context) {
	role := middleware.GetRole(c.Request.Context())
	if role != "project_admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Only project admin can access merchant management",
		})
		return
	}

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
	role := middleware.GetRole(c.Request.Context())
	if role != "project_admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Only project admin can create merchants",
		})
		return
	}

	var input struct {
		Name         string                   `json:"name" binding:"required"`
		Code         string                   `json:"code" binding:"required"`
		ContactName  string                   `json:"contact_name"`
		ContactEmail string                   `json:"contact_email"`
		ContactPhone string                   `json:"contact_phone"`
		AdminUID     string                   `json:"admin_uid"`
		AdminName    string                   `json:"admin_name"`
		AdminEmail   string                   `json:"admin_email"`
		AdminPhone   string                   `json:"admin_phone"`
		UserIDs      []map[string]interface{} `json:"user_ids"`
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
	db.Model(&models.Merchant{}).Where("tenant_id = ? AND code = ?", tenantID, input.Code).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Merchant code already exists",
		})
		return
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)

	var adminUserID string
	var userIDsToProcess []map[string]interface{}

	if input.AdminUID != "" && len(input.UserIDs) == 0 {
		adminUserID = input.AdminUID
		userIDsToProcess = []map[string]interface{}{{"user_id": input.AdminUID, "action_type": "merchant_admin"}}
	} else if input.AdminName != "" && input.AdminEmail != "" && len(input.UserIDs) == 0 {
		userIDsToProcess = []map[string]interface{}{{"action_type": "merchant_admin"}}
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

	callbackURL := fmt.Sprintf("https://%s/api/iam/confirmation-callback", c.Request.Host)

	var adminName, adminEmail, adminPhone string
	var adminUser models.User
	if adminUserID != "" {
		if err := db.Where("id = ?", adminUserID).First(&adminUser).Error; err == nil {
			adminName = adminUser.Name
			adminEmail = adminUser.Email
			adminPhone = adminUser.Phone
		}
	}
	if adminName == "" && input.AdminName != "" && input.AdminEmail != "" {
		adminName = input.AdminName
		adminEmail = input.AdminEmail
		adminPhone = input.AdminPhone
	}

	orgResp, err := iamClient.CreateOrganizationWithToken(userToken, &services.CreateOrganizationRequest{
		Name:    input.Name,
		Address: input.ContactName,
		AdminInfo: &services.OrganizationAdmin{
			Name:     adminName,
			Username: adminName,
			Email:    adminEmail,
			Phone:    adminPhone,
		},
		CallbackURL: callbackURL,
		OperatorID:  middleware.GetUserID(c.Request.Context()),
	})
	if err != nil {
		log.Printf("[CreateMerchant] IAM CreateOrganization failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create organization in IAM: " + err.Error(),
		})
		return
	}

	iamOrgID := orgResp.OrgID
	if iamOrgID == "" {
		iamOrgID = middleware.GetOrgID(c.Request.Context())
	}

	merchant := models.Merchant{
		TenantID:     tenantID,
		OrgID:        iamOrgID,
		Name:         input.Name,
		Code:         input.Code,
		ContactName:  input.ContactName,
		ContactEmail: input.ContactEmail,
		ContactPhone: input.ContactPhone,
		AdminUID:     adminUserID,
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

	if adminUserID == "" && orgResp.AdminID != "" {
		adminUserID = orgResp.AdminID
	}

	var directlyAdded []string
	for _, userEntry := range userIDsToProcess {
		if userID, ok := userEntry["user_id"].(string); ok && userID != "" {
			directlyAdded = append(directlyAdded, userID)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id":             merchant.ID,
			"name":           merchant.Name,
			"code":           merchant.Code,
			"iam_org_id":     iamOrgID,
			"admin_uid":      adminUserID,
			"directly_added": directlyAdded,
			"iam_admin_id":   orgResp.AdminID,
			"callback_url":   callbackURL,
		},
	})
}

// UpdateMerchant PUT /api/merchants/:id - Update merchant
func (h *MerchantHandler) UpdateMerchant(c *gin.Context) {
	role := middleware.GetRole(c.Request.Context())
	if role != "project_admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Only project admin can update merchants",
		})
		return
	}

	id := c.Param("id")

	var input struct {
		Name         string `json:"name"`
		ContactName  string `json:"contact_name"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
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
	merchant.ContactName = input.ContactName
	merchant.ContactEmail = input.ContactEmail
	merchant.ContactPhone = input.ContactPhone

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
	role := middleware.GetRole(c.Request.Context())
	if role != "project_admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Only project admin can delete merchants",
		})
		return
	}

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
