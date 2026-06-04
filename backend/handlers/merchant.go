package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		database.GetDB().Where("id IN ? AND deleted_at IS NULL", adminUIDs).Find(&adminUsers)
		for _, u := range adminUsers {
			adminMap[u.ID] = u
		}
	}

	// Lazy confirm: check IAM for pending admins
	iamClient := services.NewIAMClient()
	nilUUID := "00000000-0000-0000-0000-000000000000"
	for i := range merchants {
		m := &merchants[i]
		if !m.AdminPending || m.AdminUID == "" || m.AdminUID == nilUUID || m.OrgID == "" {
			continue
		}
		if u, ok := adminMap[m.AdminUID]; ok && u.IAMSub != "" {
			if isBound, err := iamClient.CheckMembership(u.IAMSub, m.OrgID); err == nil && isBound {
				m.AdminPending = false
				database.GetDB().Model(m).Update("admin_pending", false)
			}
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
			"admin_pending": m.AdminPending,
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
		Name              string                    `json:"name" binding:"required"`
		Phone             string                    `json:"phone"`
		Address           string                    `json:"address"`
		MerchantType      string                    `json:"merchant_type"`
		TransitAddress    string                    `json:"transit_address"`
		TransitPhone      string                    `json:"transit_phone"`
		TransitContactName string                   `json:"transit_contact_name"`
		AdminUID          string                    `json:"admin_uid"`
		AdminName         string                    `json:"admin_name"`
		AdminUsername     string                    `json:"admin_username"`
		AdminEmail        string                    `json:"admin_email"`
		AdminPhone        string                    `json:"admin_phone"`
		UserIDs           []map[string]interface{}   `json:"user_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	if input.MerchantType == "" {
		input.MerchantType = models.MerchantTypeFull
	}

	if input.MerchantType == models.MerchantTypeControlled {
		if input.TransitAddress == "" || input.TransitPhone == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40001,
				"message": "受控商户必须填写中转地址和中转电话",
			})
			return
		}
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

	callbackURL := os.Getenv("EXTERNAL_WEB_URL")
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://%s", c.Request.Host)
	}

	var adminUserID string
	var adminIAMSub string
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
			Reason:      "商户管理员 - " + input.Name,
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
		adminIAMSub = userResult.UserID
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
			// Use a DB without tenant scope to find users created via IAM proxy
			rawDB := database.GetDB().WithContext(context.Background())
			if err := rawDB.Where("id = ?", adminUserID).First(&adminUser).Error; err == nil {
				adminName = adminUser.Name
				adminEmail = adminUser.Email
				adminPhone = adminUser.Phone
				adminIAMSub = adminUser.IAMSub
			} else {
				log.Printf("[CreateMerchant] Warning: admin user %s not found in local DB: %v", adminUserID, err)
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
			orgs, listErr := iamClient.ListOrganizationsWithToken(userToken)
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
		TenantID:           tenantID,
		OrgID:              iamOrgID,
		Name:               input.Name,
		Code:               input.Name,
		Phone:              input.Phone,
		Address:            input.Address,
		MerchantType:       input.MerchantType,
		TransitAddress:     input.TransitAddress,
		TransitPhone:       input.TransitPhone,
		TransitContactName: input.TransitContactName,
		AdminUID:           adminUserID,
		AdminPending:       adminUserID != "" && adminUserID != "00000000-0000-0000-0000-000000000000",
		Status:             "active",
	}

	// Create tenants record using IAM org ID as primary key
	tenantRecord := models.Tenant{
		ID:     iamOrgID,
		Name:   input.Name,
		Status: "active",
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&tenantRecord).Error; err != nil {
		log.Printf("[CreateMerchant] Warning: failed to create tenant record: %v", err)
	}

	result := db.Create(&merchant)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create merchant: " + result.Error.Error(),
		})
		return
	}

	// Explicitly bind admin user to the new organization in IAM
	if adminIAMSub != "" && iamOrgID != "" {
		if err := iamClient.BindUserToOrganizationWithToken(userToken, adminIAMSub, iamOrgID, "OWNER", middleware.GetUserID(c.Request.Context())); err != nil {
			log.Printf("[CreateMerchant] Warning: failed to bind admin to org: %v", err)
		} else {
			log.Printf("[CreateMerchant] Bound admin %s to org %s", adminIAMSub, iamOrgID)
		}
	}

	// Set cus_perm for merchant admin using merchant_admin template
	if adminIAMSub != "" && iamOrgID != "" {
		if t, ok := services.AllRoleTemplates["merchant_admin"]; ok && len(t.CusPermCodes) > 0 {
			cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(t.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
			var setErr error
			for attempt := 0; attempt < 5; attempt++ {
				setErr = iamClient.SetUserCustomerPermissionsWithToken(userToken, iamOrgID, adminIAMSub, cusPerm, cusPermExt)
				if setErr == nil {
					break
				}
				log.Printf("[CreateMerchant] set cus_perm attempt %d/5: %v", attempt+1, setErr)
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			}
			if setErr != nil {
				log.Printf("[CreateMerchant] Warning: failed to set admin cus_perm: %v", setErr)
			} else {
				log.Printf("[CreateMerchant] Set merchant_admin cus_perm for %s", adminIAMSub)
			}
		}
	}

	// Initialize system roles for the new tenant
	if iamOrgID != "" {
		nsID := middleware.GetNamespaceID(c.Request.Context())
		initSystemRoles(db, iamClient, iamOrgID, nsID)
	}

	// Assign merchant_admin role template to admin user in IAM
	if adminIAMSub != "" && iamOrgID != "" {
		nsID := middleware.GetNamespaceID(c.Request.Context())
		templates, err := iamClient.ListRoleTemplates(nsID)
		if err == nil {
			for _, t := range templates {
				if t.Code == "merchant_admin" {
					if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, adminIAMSub, t.ID); err != nil {
						log.Printf("[CreateMerchant] Warning: failed to assign merchant_admin role: %v", err)
					} else {
						log.Printf("[CreateMerchant] Assigned merchant_admin role to %s", adminIAMSub)
					}
					break
				}
			}
		} else {
			log.Printf("[CreateMerchant] Warning: failed to list role templates: %v", err)
		}
	}

	// Update local user record with org_id, tenant_id and role
	if adminIAMSub != "" && iamOrgID != "" {
		db.Model(&models.User{}).Where("iam_sub = ?", adminIAMSub).Updates(map[string]interface{}{
			"org_id":    iamOrgID,
			"tenant_id": iamOrgID,
			"role":      "merchant_admin",
			"status":    "active",
		})
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
		Name              string `json:"name"`
		Phone             string `json:"phone"`
		Address           string `json:"address"`
		MerchantType      string `json:"merchant_type"`
		TransitAddress    string `json:"transit_address"`
		TransitPhone      string `json:"transit_phone"`
		TransitContactName string `json:"transit_contact_name"`
		AdminUID          string `json:"admin_uid"`
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
	if input.MerchantType != "" {
		merchant.MerchantType = input.MerchantType
	}
	if input.TransitAddress != "" {
		merchant.TransitAddress = input.TransitAddress
	}
	if input.TransitPhone != "" {
		merchant.TransitPhone = input.TransitPhone
	}
	if input.TransitContactName != "" {
		merchant.TransitContactName = input.TransitContactName
	}

	// Handle admin_uid change
	nilUUID := "00000000-0000-0000-0000-000000000000"
	newAdmin := input.AdminUID
	oldAdmin := merchant.AdminUID

	if newAdmin == "" || newAdmin == nilUUID {
		newAdmin = ""
	}
	if oldAdmin == "" || oldAdmin == nilUUID {
		oldAdmin = ""
	}

	if newAdmin != oldAdmin {
		iamClient := services.NewIAMClient()
		operatorID := middleware.GetUserID(c.Request.Context())
		userToken := services.ExtractUserToken(c)

		// Bind new admin if provided (bind first per #618 best practice)
		if input.AdminUID != "" && input.AdminUID != nilUUID && merchant.OrgID != "" {
			var newUser models.User
			if err := database.GetDB().Where("id = ?", input.AdminUID).First(&newUser).Error; err == nil && newUser.IAMSub != "" {
				if bindErr := iamClient.BindUserToOrganizationWithToken(userToken, newUser.IAMSub, merchant.OrgID, "OWNER", operatorID); bindErr != nil {
					log.Printf("[UpdateMerchant] Failed to bind new admin %s: %v", newUser.IAMSub, bindErr)
				}
				merchant.AdminPending = true
			}
		}

		// Demote old admin if there was one
		if oldAdmin != "" && merchant.OrgID != "" {
			var oldUser models.User
			if err := database.GetDB().Where("id = ?", merchant.AdminUID).First(&oldUser).Error; err == nil && oldUser.IAMSub != "" {
				if demoteErr := iamClient.UpdateUserRoleInOrgWithToken(userToken, merchant.OrgID, oldUser.IAMSub, "USER"); demoteErr != nil {
					log.Printf("[UpdateMerchant] Failed to demote old admin %s: %v", oldUser.IAMSub, demoteErr)
				}
			}
		}

		// Update local record
		if input.AdminUID == "" || input.AdminUID == nilUUID {
			merchant.AdminUID = nilUUID
			merchant.AdminPending = false
		} else {
			merchant.AdminUID = input.AdminUID
		}
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
	db.Model(&models.Order{}).Where("org_id = ? AND status IN (?)", merchant.OrgID, []string{models.OrderStatusPaid, models.OrderStatusInLease}).Count(&orderCount)
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

// initSystemRoles creates the 4 system roles in the local DB for a new tenant.
// Uses services.AllRoleTemplates for role definitions.
func initSystemRoles(db *gorm.DB, iamClient *services.IAMClient, tenantID, nsID string) {
	log.Printf("[initSystemRoles] Starting role init for tenant=%s ns=%s", tenantID, nsID)
	systemRoles := map[string][]string{
		"merchant_admin": {"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:price_config", "instrument:maintain", "order:create", "order:read", "order:update", "order:cancel"},
		"site_admin":     {"instrument:create", "instrument:read", "instrument:update", "instrument:price", "instrument:maintain", "order:read", "order:update", "order:cancel"},
		"site_member":    {"instrument:create", "instrument:read", "instrument:update", "instrument:maintain", "order:create", "order:read", "order:update"},
		"worker":         {"instrument:read", "instrument:maintain"},
	}
	for code, codes := range systemRoles {
		var count int64
		db.Model(&models.Role{}).Where("tenant_id = ? AND code = ?", tenantID, code).Count(&count)
		if count == 0 {
			role := models.Role{
				TenantID:      tenantID,
				IAMTemplateID: "",
				Name:          services.AllRoleTemplates[code].Name,
				Code:          code,
				CusPermCodes:  codes,
				IsSystem:      true,
			}
			if err := db.Create(&role).Error; err != nil {
				log.Printf("[initSystemRoles] Warning: failed to create role %s: %v", code, err)
			} else {
				log.Printf("[initSystemRoles] Created system role %s for tenant %s", code, tenantID)
			}
		}
	}
	// Also sync role templates to IAM for the namespace (idempotent)
	for code := range systemRoles {
		if t, ok := services.AllRoleTemplates[code]; ok {
			if len(t.SysPermBits) > 0 {
				if err := iamClient.SyncRoleTemplateSysPerm(nsID, code, t.SysPermBits); err != nil {
					log.Printf("[initSystemRoles] Warning: failed to sync sys_perm for %s: %v", code, err)
				}
			}
			if len(t.CusPermCodes) > 0 {
				cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(t.CusPermCodes, func(code string) int {
					return middleware.PermissionRegistry.GetCusPermBit(code)
				})
				if err := iamClient.SyncRoleTemplateCusPerm(nsID, code, cusPerm, cusPermExt); err != nil {
					log.Printf("[initSystemRoles] Warning: failed to sync cus_perm for %s: %v", code, err)
				}
			}
		}
	}
}

type MerchantTransitInfo struct {
	MerchantType string
	Address      string
	Phone        string
	ContactName  string
}

func GetMerchantTransitInfo(ctx context.Context, tenantID string) *MerchantTransitInfo {
	db := database.GetDB()
	var merchant models.Merchant
	if err := db.Where("tenant_id = ?", tenantID).First(&merchant).Error; err != nil {
		return nil
	}
	return &MerchantTransitInfo{
		MerchantType: merchant.MerchantType,
		Address:      merchant.TransitAddress,
		Phone:        merchant.TransitPhone,
		ContactName:  merchant.TransitContactName,
	}
}
