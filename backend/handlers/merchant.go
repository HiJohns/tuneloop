package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

	query := db.Model(&models.Merchant{})
	if middleware.GetBusinessRole(c.Request.Context()) == middleware.BusinessRoleMerchantAdmin {
		if orgID := middleware.GetOrgID(c.Request.Context()); orgID != "" {
			// #2274/#2276：商户管理员按商户 org 锚（共享 tenant 下不得看其他商户）
			query = query.Where("org_id = ?", orgID)
		} else {
			query = query.Where("tenant_id = ?", tenantID)
		}
	} else {
		query = query.Where("tenant_id = ?", tenantID)
	}

	// Apply status filter if provided
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Pagination
	var total int64
	query.Count(&total)

	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("pageSize", "20"), 20)
	pageSize = clampPageSize(pageSize, 20, maxPageSize)
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
		// #2546 D1b：读取侧归一化 —— admin_uid 历史行可能是 IAM 用户 id
		// （users.iam_sub）；两种 ID 空间都要能解析到同一条 users 记录。
		var adminUsers []models.User
		database.GetDB().Where("(id IN ? OR iam_sub IN ?) AND deleted_at IS NULL", adminUIDs, adminUIDs).Find(&adminUsers)
		for _, u := range adminUsers {
			adminMap[u.ID] = u
			if u.IAMSub != "" {
				adminMap[u.IAMSub] = u
			}
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

	var q *gorm.DB
	if middleware.GetBusinessRole(c.Request.Context()) == middleware.BusinessRoleMerchantAdmin {
		if oid := middleware.GetOrgID(c.Request.Context()); oid != "" {
			q = db.Where("id = ? AND org_id = ?", id, oid)
		} else {
			q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
		}
	} else {
		q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
	}
	result := q.First(&merchant)
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
		Name               string                   `json:"name" binding:"required"`
		Phone              string                   `json:"phone"`
		Address            string                   `json:"address"`
		MerchantType       string                   `json:"merchant_type"`
		TransitAddress     string                   `json:"transit_address"`
		TransitPhone       string                   `json:"transit_phone"`
		TransitContactName string                   `json:"transit_contact_name"`
		AdminUID           string                   `json:"admin_uid"`
		AdminName          string                   `json:"admin_name"`
		AdminUsername      string                   `json:"admin_username"`
		AdminEmail         string                   `json:"admin_email"`
		AdminPhone         string                   `json:"admin_phone"`
		UserIDs            []map[string]interface{} `json:"user_ids"`
		SkipActivation     bool                     `json:"skip_activation"`
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

	// #1924-era cleanup: transit address/phone are NOT mandatory for controlled
	// merchants — transit logistics are handled by transit SITES + TransitRoute
	// (cases/cases.md §4.1.5). The merchant-level fields remain OPTIONAL (retail
	// order/return display via GetMerchantTransitInfo, spec/features.md).

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
	var initialPassword string

	if input.AdminUID != "" && len(input.UserIDs) == 0 {
		adminUserID = input.AdminUID
		userIDsToProcess = []map[string]interface{}{{"user_id": input.AdminUID, "action_type": "merchant_admin"}}
	} else if input.AdminName != "" && input.AdminEmail != "" && len(input.UserIDs) == 0 {
		// Scenario 2: create IAM user first, then use the returned ID
		createReq := &services.CreateUserRequest{
			Username:   input.AdminUsername,
			Name:       input.AdminName,
			Email:      input.AdminEmail,
			Phone:      input.AdminPhone,
			Reason:     "商户管理员 - " + input.Name,
			OperatorID: middleware.GetUserID(c.Request.Context()),
		}

		if input.SkipActivation {
			pwd := generatePassword()
			createReq.Password = pwd
			createReq.SkipActivation = true
			createReq.SendNotificationEmail = true
			createReq.NotificationLang = middleware.GetCulture(c)
			initialPassword = pwd
		} else {
			createReq.CallbackURL = callbackURL
		}

		userResult, err := iamClient.CreateOrGetUser(userToken, createReq)
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
	}
	// Merchant may be created without an admin — members (including the
	// merchant admin) are added later via the merchant member management API.

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
	orgReq := &services.CreateOrganizationRequest{
		Name:             input.Name,
		Address:          input.Address,
		NamespaceID:      middleware.GetNamespaceID(c.Request.Context()),
		NotificationLang: middleware.GetCulture(c),
		AdminInfo: &services.OrganizationAdmin{
			Name:     adminName,
			Username: adminUserName,
			Email:    adminEmail,
			Phone:    adminPhone,
		},
		OperatorID: middleware.GetUserID(c.Request.Context()),
	}

	if input.SkipActivation {
		orgReq.SkipActivation = true
	} else {
		orgReq.CallbackURL = callbackURL
	}

	orgResp, err := iamClient.CreateOrganizationWithToken(userToken, orgReq)
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
		log.Printf("[CreateMerchant] Error: iamOrgID is empty after create/recovery — cannot determine merchant org")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to determine merchant organization ID",
		})
		return
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
		AdminPending:       !input.SkipActivation && adminUserID != "" && adminUserID != "00000000-0000-0000-0000-000000000000",
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

	// Initialize system roles for the new tenant
	if iamOrgID != "" {
		nsID := middleware.GetNamespaceID(c.Request.Context())
		initSystemRoles(db, iamClient, iamOrgID, nsID)
	}

	// Assign merchant_admin role template to admin user in IAM
	// Cus_perm comes from the role template (created by system during bootstrap),
	// so namespace admin doesn't need to possess those bits directly.
	var roleErrors []gin.H
	if adminIAMSub != "" && iamOrgID != "" {
		nsID := middleware.GetNamespaceID(c.Request.Context())
		templates, err := iamClient.ListRoleTemplates(nsID)
		if err == nil {
			for _, t := range templates {
				if t.Code == "merchant_admin" {
					if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, adminIAMSub, iamOrgID, t.Code); err != nil {
						log.Printf("[CreateMerchant] Warning: failed to assign merchant_admin role: %v", err)
						roleErrors = append(roleErrors, gin.H{
							"user_id":       adminIAMSub,
							"template_code": "merchant_admin",
							"error":         err.Error(),
						})
					} else {
						log.Printf("[CreateMerchant] Assigned merchant_admin role to %s", adminIAMSub)
					}
					break
				}
			}
		} else {
			log.Printf("[CreateMerchant] Warning: failed to list role templates: %v", err)
			roleErrors = append(roleErrors, gin.H{
				"error": "failed to list role templates: " + err.Error(),
			})
		}
	}

	// Update local user record with org_id, tenant_id and role.
	// #2626 D1/D2：users.role / users.roles 只是 IAM 的派生镜像 —— 新建商户的
	// 管理员在 IAM 侧是「merchant 级组织的 OWNER + merchant_admin 功能角色」，
	// 必须经 DeriveRoles 统一派生后同时写主角色与多值镜像，禁止在此内联赋值。
	if adminIAMSub != "" && iamOrgID != "" {
		mirror := services.DeriveRoles(services.OrgLevelMerchant, services.IAMRoleOwner, []string{services.RoleMerchantAdmin})
		db.Model(&models.User{}).Where("iam_sub = ?", adminIAMSub).Updates(map[string]interface{}{
			"org_id":    iamOrgID,
			"tenant_id": iamOrgID,
			"role":      mirror.Primary,
			"roles":     mirror.All,
			"status":    "active",
		})
	}

	var directlyAdded []string
	for _, userEntry := range userIDsToProcess {
		if userID, ok := userEntry["user_id"].(string); ok && userID != "" {
			directlyAdded = append(directlyAdded, userID)
		}
	}

	pwd := initialPassword
	orgInitPass := ""
	if orgResp != nil {
		orgInitPass = orgResp.InitialPassword
	}
	if pwd == "" {
		pwd = orgInitPass
	}
	log.Printf("[CreateMerchant] skip_activation=%v initialPassword=%q orgResp.InitialPassword=%q finalPassword=%q respHasPassword=%v",
		input.SkipActivation, initialPassword, orgInitPass, pwd, pwd != "")

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
	if pwd != "" {
		responseData["initial_password"] = pwd
	}
	if len(roleErrors) > 0 {
		responseData["role_errors"] = roleErrors
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
		Name               string          `json:"name"`
		Phone              string          `json:"phone"`
		Address            string          `json:"address"`
		MerchantType       string          `json:"merchant_type"`
		TransitAddress     string          `json:"transit_address"`
		TransitPhone       string          `json:"transit_phone"`
		TransitContactName string          `json:"transit_contact_name"`
		AdminUID           json.RawMessage `json:"admin_uid"`
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
	var q *gorm.DB
	if middleware.GetBusinessRole(c.Request.Context()) == middleware.BusinessRoleMerchantAdmin {
		if oid := middleware.GetOrgID(c.Request.Context()); oid != "" {
			q = db.Where("id = ? AND org_id = ?", id, oid)
		} else {
			q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
		}
	} else {
		q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
	}
	result := q.First(&merchant)
	if result.Error == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return
	}

	// #2546 D15：普通字段更新与所有权转让分离且有序。
	// 1) 先只写请求中出现的普通字段（map Updates，避免 Save 回写整行）。
	updates := map[string]interface{}{}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Phone != "" {
		updates["phone"] = input.Phone
	}
	if input.Address != "" {
		updates["address"] = input.Address
	}
	if input.MerchantType != "" {
		updates["merchant_type"] = input.MerchantType
	}
	if input.TransitAddress != "" {
		updates["transit_address"] = input.TransitAddress
	}
	if input.TransitPhone != "" {
		updates["transit_phone"] = input.TransitPhone
	}
	if input.TransitContactName != "" {
		updates["transit_contact_name"] = input.TransitContactName
	}
	if len(updates) > 0 {
		upd := db.Model(&models.Merchant{}).
			Where("id = ? AND tenant_id = ?", merchant.ID, merchant.TenantID).
			Updates(updates)
		if upd.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to update merchant: " + upd.Error.Error(),
			})
			return
		}
	}

	// 2) 再转让所有权（唯一写入路径，见 transferMerchantOwnership）。
	// RawMessage 区分「字段缺省（不动所有权）」「显式 null（拒绝）」「有值」：
	// OWNER 不能删除也不能清空，只能转让（#2546 D4/Low-3，fail-fast 不静默）。
	if input.AdminUID != nil {
		if strings.TrimSpace(string(input.AdminUID)) == "null" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "所有人不能置空，只能转让所有权"})
			return
		}
		var requestedOwner string
		if err := json.Unmarshal(input.AdminUID, &requestedOwner); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "admin_uid 必须是字符串"})
			return
		}
		requestedOwner = strings.TrimSpace(requestedOwner)
		if requestedOwner == nilUUID2546 {
			requestedOwner = ""
		}
		if requestedOwner == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "所有人不能清空，只能转让所有权"})
			return
		}
		// #2546 D1b/D14：所有权比较统一在本地 users.id 空间。
		oldOwner, _, ownerErr := merchantOwnerLocalUID(c.Request.Context(), db, merchant)
		if ownerErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "解析商户所有人失败：" + ownerErr.Error()})
			return
		}
		if requestedOwner != oldOwner {
			previousOwnerID, transferErr := transferMerchantOwnership(c, db, merchant, requestedOwner)
			if transferErr != nil {
				var typed *ownershipTransferError
				if errors.As(transferErr, &typed) {
					c.JSON(typed.Status, gin.H{"code": typed.Code, "message": typed.Message})
					return
				}
				c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "转让所有权失败（IAM）：" + transferErr.Error()})
				return
			}
			log.Printf("[UpdateMerchant] ownership transferred merchant=%s previous_owner=%s new_owner=%s", merchant.ID, previousOwnerID, requestedOwner)
		}
	}

	// 3) 转让已写库，仅重新读取用于响应快照（不再 Save，避免覆盖内存字段）。
	if err := db.Where("id = ?", merchant.ID).First(&merchant).Error; err != nil {
		log.Printf("[UpdateMerchant] reload after update failed merchant=%s: %v", merchant.ID, err)
	}

	resp := gin.H{
		"code": 20000,
		"data": merchant,
	}
	c.JSON(http.StatusOK, resp)
}

// DeleteMerchant DELETE /api/merchants/:id - Delete merchant
func (h *MerchantHandler) DeleteMerchant(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check if merchant exists
	var merchant models.Merchant
	var q *gorm.DB
	if middleware.GetBusinessRole(c.Request.Context()) == middleware.BusinessRoleMerchantAdmin {
		if oid := middleware.GetOrgID(c.Request.Context()); oid != "" {
			q = db.Where("id = ? AND org_id = ?", id, oid)
		} else {
			q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
		}
	} else {
		q = db.Where("id = ? AND tenant_id = ?", id, tenantID)
	}
	result := q.First(&merchant)
	if result.Error == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return
	}

	// Check for active sites
	var siteCount int64
	orgAnchor := merchant.OrgID
	if orgAnchor == "" {
		orgAnchor = merchant.TenantID
	}
	db.Model(&models.Site{}).Where("org_id = ? AND status = ?", orgAnchor, "active").Count(&siteCount)
	if siteCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Cannot delete merchant with active sites",
		})
		return
	}

	// Check for incomplete orders (paid or in_lease status)
	var orderCount int64
	db.Model(&models.Order{}).Where("org_id = ? AND status IN (?)", orgAnchor, []string{models.OrderStatusPaid, models.OrderStatusInLease}).Count(&orderCount)
	if orderCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Cannot delete merchant with incomplete orders",
		})
		return
	}

	// #2031 C（D4）：级联解除商户及其下属网点的成员 relation。
	// 用户记录保留（可重新加回）；失败显式返回，不静默吞错。
	iamClient := services.NewIAMClient()
	unbind := func(userID, orgID string) error {
		var u models.User
		if err := db.Where("id = ?", userID).First(&u).Error; err != nil || u.IAMSub == "" {
			return nil // 无 IAM 映射：仅清理本地 relation
		}
		return iamClient.UnbindUserFromOrganization(u.IAMSub, orgID, u.IAMSub)
	}
	var merchantMembers []models.MerchantMember
	db.Where("tenant_id = ?", tenantID).Find(&merchantMembers)
	for _, m := range merchantMembers {
		if err := unbind(m.UserID, merchant.OrgID); err != nil {
			log.Printf("[MerchantDelete] cascade unbind merchant member failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "级联解除商户成员失败，请稍后再试"})
			return
		}
	}
	var siteMembers []models.SiteMember
	db.Where("tenant_id = ?", tenantID).Find(&siteMembers)
	for _, m := range siteMembers {
		var site models.Site
		if err := db.Where("id = ?", m.SiteID).First(&site).Error; err != nil || site.OrgID == "" {
			continue
		}
		if err := unbind(m.UserID, site.OrgID); err != nil {
			log.Printf("[MerchantDelete] cascade unbind site member failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "级联解除网点成员失败，请稍后再试"})
			return
		}
	}
	db.Where("tenant_id = ?", tenantID).Delete(&models.MerchantMember{})
	db.Where("tenant_id = ?", tenantID).Delete(&models.SiteMember{})

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
		"merchant_admin":    {"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:price_config", "instrument:maintain", "repair:start", "repair:complete", "repair:accept", "order:create", "order:read", "order:update", "order:cancel"},
		"site_admin":        {"instrument:create", "instrument:read", "instrument:update", "instrument:price", "instrument:maintain", "repair:start", "repair:complete", "repair:accept", "order:read", "order:update", "order:cancel"},
		"site_member":       {"instrument:create", "instrument:read", "instrument:update", "instrument:maintain", "repair:start", "repair:complete", "repair:accept", "order:create", "order:read", "order:update"},
		"repair_technician": {"instrument:read", "instrument:maintain", "repair:start", "repair:complete"},
	}
	for code, codes := range systemRoles {
		role := models.Role{
			TenantID:      tenantID,
			IAMTemplateID: "",
			Name:          services.AllRoleTemplates[code].Name,
			Code:          code,
			CusPermCodes:  codes,
			IsSystem:      true,
		}
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&role)
		if result.Error != nil {
			log.Printf("[initSystemRoles] Warning: failed to create role %s: %v", code, result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("[initSystemRoles] Created system role %s for tenant %s", code, tenantID)
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

// GetMerchantTransitInfos 批量版（#2451）：一次查询取回全部 tenant 的商户，
// 每个 tenant 取 id 最小的商户——复刻单条 helper 的 First() 语义（主键升序 LIMIT 1）。
// tenantIDs 为空 → 空 map 且不发查询；查询失败 → 空 map，调用方按「无商户」回退，
// 等价于单条 helper 返回 nil。GetMerchantTransitInfo 保持逐条版不动。
func GetMerchantTransitInfos(ctx context.Context, tenantIDs []string) map[string]*MerchantTransitInfo {
	out := make(map[string]*MerchantTransitInfo, len(tenantIDs))
	if len(tenantIDs) == 0 {
		return out
	}
	db := database.GetDB()
	var merchants []models.Merchant
	if err := db.Where("tenant_id IN ?", tenantIDs).Order("id ASC").Find(&merchants).Error; err != nil {
		return out
	}
	for _, merchant := range merchants {
		if _, ok := out[merchant.TenantID]; ok {
			continue
		}
		out[merchant.TenantID] = &MerchantTransitInfo{
			MerchantType: merchant.MerchantType,
			Address:      merchant.TransitAddress,
			Phone:        merchant.TransitPhone,
			ContactName:  merchant.TransitContactName,
		}
	}
	return out
}
