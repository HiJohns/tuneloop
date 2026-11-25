package handlers

import (
	"context"
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
		Name               string `json:"name"`
		Phone              string `json:"phone"`
		Address            string `json:"address"`
		MerchantType       string `json:"merchant_type"`
		TransitAddress     string `json:"transit_address"`
		TransitPhone       string `json:"transit_phone"`
		TransitContactName string `json:"transit_contact_name"`
		AdminUID           string `json:"admin_uid"`
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

	// #2459: IAM organizations.name 是商户名的权威源，merchants.name 只是本地镜像（D1）。
	// 改名必须先写 IAM，成功后才写本地；任何 IAM 失败都不落库（含 admin_uid 变更，D3）。
	origName := merchant.Name
	newName := strings.TrimSpace(input.Name)
	nameChanged := newName != "" && newName != origName

	if nameChanged {
		// D7（Owner O3）：商户名只允许商户管理员（OWNER）修改；平台/系统管理员改名一律 403。
		if middleware.GetBusinessRole(c.Request.Context()) != middleware.BusinessRoleMerchantAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40301,
				"message": "仅商户管理员可修改商户名称",
			})
			return
		}
		// 未绑定 IAM 组织时只能单边改本地 → 必然漂移，直接拒绝。
		if strings.TrimSpace(merchant.OrgID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "该商户未绑定 IAM 组织，无法改名（禁止本地单边改名）",
			})
			return
		}

		iamClient := services.NewIAMClient()
		userToken := services.ExtractUserToken(c)

		// D8（Owner O3）：namespace 内唯一预检（IAM 侧尚无唯一索引，见 tracking Issue）。
		// 先做本地跨商户兜底（廉价、不依赖 IAM 可见性），再用服务凭据查 IAM 全 namespace。
		localConflictID, localErr := findLocalMerchantNameConflict(c.Request.Context(), merchant.ID, newName)
		if localErr != nil {
			log.Printf("[UpdateMerchant] failed to verify merchant name uniqueness locally org_id=%s: %v", merchant.OrgID, localErr)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "无法校验商户名唯一性（本地）: " + localErr.Error(),
			})
			return
		}
		if localConflictID != "" {
			log.Printf("[UpdateMerchant] merchant name conflict (local) org_id=%s conflict_merchant=%s name=%q",
				merchant.OrgID, localConflictID, newName)
			c.JSON(http.StatusConflict, gin.H{
				"code":    40900,
				"message": "商户名在命名空间内已存在",
			})
			return
		}

		conflictOrgID, listErr := findOrganizationNameConflict(iamClient, merchant.OrgID, newName)
		if listErr != nil {
			log.Printf("[UpdateMerchant] failed to verify merchant name uniqueness org_id=%s: %v", merchant.OrgID, listErr)
			c.JSON(http.StatusBadGateway, gin.H{
				"code":    50000,
				"message": "无法校验商户名唯一性: " + listErr.Error(),
			})
			return
		}
		if conflictOrgID != "" {
			log.Printf("[UpdateMerchant] merchant name conflict org_id=%s conflict_org=%s name=%q",
				merchant.OrgID, conflictOrgID, newName)
			c.JSON(http.StatusConflict, gin.H{
				"code":    40900,
				"message": "商户名在命名空间内已存在",
			})
			return
		}

		var iamErr error
		if userToken == "" {
			// 无调用者令牌（内部调用/测试）：直接使用服务凭据。
			_, iamErr = iamClient.UpdateOrganizationName(merchant.OrgID, newName)
		} else {
			_, iamErr = iamClient.UpdateOrganizationNameWithToken(userToken, merchant.OrgID, newName)
			if iamErr != nil && errors.Is(iamErr, services.ErrIAMForbidden) &&
				middleware.GetSysPerm(c.Request.Context())&(1<<middleware.SysPermTenantUpdate) != 0 {
				// D4：用户令牌被 IAM 拒绝但本地已通过 tenant_update →
				// 用服务凭据重试一次（不静默只改本地），并留审计日志。
				log.Printf("[UpdateMerchant] IAM rename escalated to service credentials user=%s org=%s",
					middleware.GetUserID(c.Request.Context()), merchant.OrgID)
				_, iamErr = iamClient.UpdateOrganizationName(merchant.OrgID, newName)
			}
		}
		if iamErr != nil {
			log.Printf("[UpdateMerchant] IAM organization rename failed org_id=%s: %v", merchant.OrgID, iamErr)
			switch {
			case errors.Is(iamErr, services.ErrIAMForbidden):
				c.JSON(http.StatusForbidden, gin.H{
					"code":    40301,
					"message": "IAM 拒绝修改商户组织名（权限不足）: " + iamErr.Error(),
				})
			case errors.Is(iamErr, services.ErrIAMOrgNotFound):
				c.JSON(http.StatusNotFound, gin.H{
					"code":    40400,
					"message": "IAM 组织不存在（org_id 失效）: " + iamErr.Error(),
				})
			case errors.Is(iamErr, services.ErrIAMOrgNameConflict):
				c.JSON(http.StatusConflict, gin.H{
					"code":    40900,
					"message": "商户名在命名空间内已存在（IAM 组织名冲突）: " + iamErr.Error(),
				})
			default:
				// D11（#2459 v3）：默认失败分支与 #2457 站点改名同构 —— 网关级 502，
				// 而不是 500（上游 IAM 故障属于 bad gateway 语义）。
				c.JSON(http.StatusBadGateway, gin.H{
					"code":    50000,
					"message": "IAM 更新商户组织名失败: " + iamErr.Error(),
				})
			}
			// IAM 失败 → 不落库（D3：admin_uid 变更一并放弃，避免半更新）。
			return
		}
		log.Printf("[UpdateMerchant] IAM organization renamed org_id=%s name=%q", merchant.OrgID, newName)
	}

	// Update fields
	if nameChanged {
		merchant.Name = newName
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

	// #2461 D3：换管理员 = 补偿式全成败（bind 新 → demote 旧；demote 失败则解绑新并 502）。
	adminWarning := ""
	if newAdmin != oldAdmin {
		iamClient := services.NewIAMClient()
		operatorID := middleware.GetUserID(c.Request.Context())
		userToken := services.ExtractUserToken(c)

		newBound := false
		var newUser models.User

		// Bind new admin if provided (bind first per #618 best practice)
		if input.AdminUID != "" && input.AdminUID != nilUUID && merchant.OrgID != "" {
			if err := database.GetDB().Where("id = ?", input.AdminUID).First(&newUser).Error; err == nil && newUser.IAMSub != "" {
				if bindErr := iamClient.BindUserToOrganizationWithToken(userToken, newUser.IAMSub, merchant.OrgID, "OWNER", operatorID); bindErr != nil {
					// #2461：bind 失败不得只 log —— 通过 admin_pending + admin_sync_warning 显式可见。
					log.Printf("[UpdateMerchant] Failed to bind new admin %s: %v", newUser.IAMSub, bindErr)
					adminWarning = "新管理员绑定 IAM 失败：" + bindErr.Error()
				} else {
					newBound = true
				}
				merchant.AdminPending = true
			} else {
				merchant.AdminPending = true
			}
		}

		// Demote old admin if there was one
		if oldAdmin != "" && merchant.OrgID != "" {
			var oldUser models.User
			if err := database.GetDB().Where("id = ?", merchant.AdminUID).First(&oldUser).Error; err == nil && oldUser.IAMSub != "" {
				if demoteErr := iamClient.UpdateUserRoleInOrgWithToken(userToken, merchant.OrgID, oldUser.IAMSub, "USER"); demoteErr != nil {
					log.Printf("[UpdateMerchant] Failed to demote old admin %s: %v", oldUser.IAMSub, demoteErr)
					// 补偿：解绑刚绑定的新管理员，保证「换管理员」全成败
					if newBound {
						if unbindErr := iamClient.UnbindUserFromOrganizationWithToken(userToken, newUser.IAMSub, merchant.OrgID, newUser.IAMSub); unbindErr != nil {
							log.Printf("[UpdateMerchant] CRITICAL: compensation unbind failed new_admin=%s org=%s: %v", newUser.IAMSub, merchant.OrgID, unbindErr)
							c.JSON(http.StatusInternalServerError, gin.H{
								"code":    50000,
								"message": "换管理员失败（旧管理员降级失败且新管理员解绑失败），请联系管理员人工核对 IAM 组织关系",
							})
							return
						}
					}
					c.JSON(http.StatusBadGateway, gin.H{
						"code":    50000,
						"message": "换管理员失败（旧管理员降级失败），请重试",
					})
					return
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
		if nameChanged {
			// IAM 已改名、本地失败：显式告知可重试（重试走同名分支补本地，D3）。
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "IAM 已改名，本地同步失败，请重试: " + result.Error.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update merchant: " + result.Error.Error(),
		})
		return
	}

	resp := gin.H{
		"code": 20000,
		"data": merchant,
	}
	// #2461：新管理员 bind 失败时除 admin_pending 外还要有显式告警字段（禁止只 log）。
	if adminWarning != "" {
		resp["admin_sync_warning"] = adminWarning
	}
	c.JSON(http.StatusOK, resp)
}

// findOrganizationNameConflict 在 IAM 同 namespace 的组织中查找与 newName 重名
// （lower + trim 归一）的其它组织，返回其 org_id；无冲突返回空串。
// #2459 D8（Owner O3）：IAM 侧尚无 (namespace_id, lower(name)) 唯一索引，
// tuneloop 在改名前置预检覆盖常规路径；并发绕过由 beaconiam 跟踪 Issue 收敛。
//
// 必须用**服务凭据**（namespace 级可见性）读取列表：#2459 审计阻断项 A —— 调用者
// 令牌会被 IAM ListOrgs 的 visibility scope 裁剪（商户管理员只看到本商户子树），
// 跨商户同名会被放行；SelfOnly 角色下用户令牌列表直接 403，改名被误判为故障。
// 权限判定仍由 IAM 在真正的改名 PUT 上完成（D7 不变）。
func findOrganizationNameConflict(client *services.IAMClient, ownOrgID, newName string) (string, error) {
	orgs, err := client.ListOrganizations()
	if err != nil {
		return "", err
	}
	target := strings.ToLower(strings.TrimSpace(newName))
	ownOrgID = strings.TrimSpace(ownOrgID)
	for _, org := range orgs {
		if strings.TrimSpace(org.ID) == ownOrgID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(org.Name)) == target {
			return org.ID, nil
		}
	}
	return "", nil
}

// findLocalMerchantNameConflict 是 namespace 内唯一性的本地兜底预检：
// IAM 列表受可见性/分页/故障影响可能漏掉对方组织，而同 namespace 的其它商户在本地
// merchants 表里一定可见（#2459 审计建议 2，双保险）。
//
// 这里显式跳过自动 tenant 过滤（ExplicitTenantScopeOnly）：本次查询的目的正是跨
// tenant 比较同 namespace 的商户名，且只回报「是否重名」，不返回其它字段。
func findLocalMerchantNameConflict(ctx context.Context, excludeMerchantID, newName string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(newName))
	excludeMerchantID = strings.TrimSpace(excludeMerchantID)
	if target == "" || excludeMerchantID == "" {
		return "", nil
	}
	var hit models.Merchant
	err := database.GetDB().WithContext(database.ExplicitTenantScopeOnly(ctx)).
		Model(&models.Merchant{}).
		Select("id").
		Where("id <> ?", excludeMerchantID).
		Where("lower(btrim(name)) = ?", target).
		Order("created_at ASC").
		Limit(1).
		Find(&hit).Error
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(hit.ID), nil
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
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "级联解除商户成员失败：" + err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "级联解除网点成员失败：" + err.Error()})
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
