package handlers

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserStaffHandler handles staff/user management
type UserStaffHandler struct{}

// ListStaff returns staff list with pagination and search
// GET /api/staff
func (h *UserStaffHandler) ListStaff(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Pagination parameters
	page := 1
	pageSize := 20
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "20")); err == nil && ps > 0 {
		pageSize = ps
	}
	offset := (page - 1) * pageSize

	// Build query
	query := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

	if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
		query = scopedDB
	}

	// Optional filters
	if name := c.Query("name"); name != "" {
		query = query.Where("name ILIKE ?", "%"+name+"%")
	}
	if phone := c.Query("phone"); phone != "" {
		query = query.Where("phone = ?", phone)
	}
	if email := c.Query("email"); email != "" {
		query = query.Where("email = ?", email)
	}
	if userType := c.Query("user_type"); userType != "" {
		query = query.Where("user_type = ?", userType)
	}
	if siteID := c.Query("site_id"); siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}

	var users []models.User
	var total int64

	// Count total
	if err := query.Model(&models.User{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to count users: " + err.Error()})
		return
	}

	// Query users with pagination
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query users: " + err.Error()})
		return
	}

	// Load site info + role from site_members (canonical user-site relationship)
	type userSiteInfo struct {
		UserID   string
		SiteID   string
		SiteName string
		Role     string
	}
	userSiteMap := make(map[string]userSiteInfo)
	if len(users) > 0 {
		var userIDs []string
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
		var memberSites []userSiteInfo
		db.Table("site_members").
			Select("site_members.user_id, site_members.site_id, sites.name as site_name, site_members.role").
			Joins("JOIN sites ON sites.id = site_members.site_id").
			Where("site_members.user_id IN ?", userIDs).
			Find(&memberSites)
		for _, ms := range memberSites {
			if _, exists := userSiteMap[ms.UserID]; !exists {
				userSiteMap[ms.UserID] = ms
			}
		}
	}

	var result []gin.H
	for _, user := range users {
		item := gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"phone":      user.Phone,
			"email":      user.Email,
			"position":   user.Position,
			"user_type":  user.UserType,
			"role":       user.Role, // fallback if no site_member record
			"status":     user.Status,
			"iam_sub":    user.IAMSub,
			"org_id":     user.OrgID,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		}
		if usi, ok := userSiteMap[user.ID]; ok {
			item["site_id"] = usi.SiteID
			item["site_name"] = usi.SiteName
			item["role"] = usi.Role
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list":      result,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// CreateUser creates a new user with uniqueness validation
// POST /api/users
func (h *UserStaffHandler) CreateUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Username           string    `json:"username"`
		Name               string    `json:"name" binding:"required"`
		Phone              string    `json:"phone" binding:"required"`
		Email              string    `json:"email"`
		Position           string    `json:"position"`
		UserType           string    `json:"user_type"`
		SiteID             uuid.UUID `json:"site_id"`
		Role               string    `json:"role"`
		Password           string    `json:"password"`
		AutoGenerate       bool      `json:"auto_generate"`
		ForcePasswordChange bool     `json:"force_password_change"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}

	// Check uniqueness constraints (phone OR email OR username)
	db := database.GetDB().WithContext(ctx)
	var conflicts []gin.H

	// Check username
	if req.Username != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND username = ? AND deleted_at IS NULL", tenantID, req.Username).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":       existingUser.ID,
				"name":     existingUser.Name,
				"phone":    existingUser.Phone,
				"email":    existingUser.Email,
				"username": existingUser.Username,
			})
		}
	}

	// Check phone
	if req.Phone != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND phone = ? AND deleted_at IS NULL", tenantID, req.Phone).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUser.ID,
				"name":  existingUser.Name,
				"phone": existingUser.Phone,
				"email": existingUser.Email,
			})
		}
	}

	// Check email if provided
	if req.Email != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND email = ? AND deleted_at IS NULL", tenantID, req.Email).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUser.ID,
				"name":  existingUser.Name,
				"phone": existingUser.Phone,
				"email": existingUser.Email,
			})
		}
	}

	// Return conflicts if any
	if len(conflicts) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "user with same phone, email, or username already exists",
			"data":    conflicts,
		})
		return
	}

	// Create user
	user := models.User{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Username:  req.Username,
		Name:      req.Name,
		Phone:     req.Phone,
		Email:     req.Email,
		Position:  req.Position,
		UserType:  req.UserType,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	var orgID string
	if req.SiteID != uuid.Nil {
		siteIDStr := req.SiteID.String()

		var site models.Site
		if err := db.Where("id = ? AND tenant_id = ?", siteIDStr, tenantID).First(&site).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "site not found"})
			return
		}
		orgID = site.OrgID
		if orgID == "" {
			orgID = tenantID
		}
		user.OrgID = orgID
	}

	// Resolve role with first-user default
	resolvedRole := req.Role
	if resolvedRole == "" {
		if req.SiteID != uuid.Nil {
			var memberCount int64
			db.Model(&models.SiteMember{}).Where("site_id = ? AND tenant_id = ?", req.SiteID.String(), tenantID).Count(&memberCount)
			if memberCount == 0 {
				resolvedRole = "site_admin"
			} else {
				resolvedRole = "site_member"
			}
		} else {
			resolvedRole = "site_member"
		}
	}

	validRoles := map[string]bool{"site_member": true, "site_admin": true, "worker": true}
	if !validRoles[resolvedRole] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid role, must be one of: site_member, site_admin, worker"})
		return
	}

	if resolvedRole == "site_admin" {
		callerRole := middleware.GetBusinessRole(c.Request.Context())
		if callerRole != middleware.BusinessRoleMerchantAdmin && callerRole != middleware.BusinessRoleSiteAdmin && callerRole != middleware.BusinessRoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "only merchant_admin or site_admin can create site_admin users"})
			return
		}
	}

	iamClient := services.NewIAMClient()
	username := req.Username
	if username == "" {
		username = req.Email
	}
	createReq := &services.CreateUserRequest{
		Username: username,
		Name:     req.Name,
		Email:    req.Email,
		Phone:    req.Phone,
	}

	var initialPassword string
	var hasPassword bool
	if req.AutoGenerate {
		initialPassword = generatePassword()
		createReq.Password = initialPassword
		createReq.SendNotificationEmail = req.Email != ""
		createReq.NotificationLang = "zh"
		hasPassword = true
	} else if req.Password != "" {
		if err := validatePassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
			return
		}
		createReq.Password = req.Password
		hasPassword = true
	}

	if hasPassword {
		createReq.SkipActivation = true
		createReq.ForcePasswordChange = req.ForcePasswordChange
	}

	iamResp, err := iamClient.CreateUser(createReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM user creation failed: " + err.Error()})
		return
	}
	user.IAMSub = iamResp.UserID
	if user.IAMSub == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM user creation failed: empty user_id returned"})
		return
	}
	if hasPassword {
		user.Status = "active"
	} else {
		user.Status = iamResp.Status
		if user.Status == "" {
			user.Status = "pending"
		}
	}
	user.ForcePasswordChange = req.ForcePasswordChange

	if orgID != "" && user.IAMSub != "" {
		userToken := services.ExtractUserToken(c)
		operatorID := middleware.GetUserID(c.Request.Context())
		iamRole := toIAMRole(resolvedRole)
		if err := iamClient.BindUserToOrganizationWithToken(userToken, user.IAMSub, orgID, iamRole, operatorID); err != nil {
			iamClient.DeleteUser(user.IAMSub)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM bind user to organization failed: " + err.Error()})
			return
		}

		template := services.AllRoleTemplates[resolvedRole]
		if len(template.CusPermCodes) > 0 {
			cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(template.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
			if err := iamClient.SetUserCustomerPermissions(orgID, user.IAMSub, cusPerm, cusPermExt); err != nil {
				iamClient.UnbindUserFromOrganization(user.IAMSub, orgID, "")
				iamClient.DeleteUser(user.IAMSub)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM set user permissions failed: " + err.Error()})
				return
			}
		}

		nsID := middleware.GetNamespaceID(c.Request.Context())
		if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
			for _, t := range templates {
				if t.Code == resolvedRole {
				if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, user.IAMSub, orgID, t.ID); err != nil {
					log.Printf("[CreateUser] AssignRoleTemplateToUserWithToken failed for user %s code %s: %v", user.IAMSub, resolvedRole, err)
					}
					break
				}
			}
		} else {
			log.Printf("[CreateUser] ListRoleTemplates failed: %v", err)
		}
	}

	if err := db.Create(&user).Error; err != nil {
		if orgID != "" && user.IAMSub != "" {
			iamClient.UnbindUserFromOrganization(user.IAMSub, orgID, "")
			iamClient.DeleteUser(user.IAMSub)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user: " + err.Error()})
		return
	}

	// Create site_member record if site_id was provided
	if req.SiteID != uuid.Nil {
		siteMember := models.SiteMember{
			TenantID: tenantID,
			SiteID:   req.SiteID.String(),
			UserID:   user.ID,
			Role:     resolvedRole,
		}
		if err := db.Create(&siteMember).Error; err != nil {
			log.Printf("[WARN] Failed to create site_member for user %s: %v", user.ID, err)
		}
	}

	respData := gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"name":       user.Name,
		"phone":      user.Phone,
		"email":      user.Email,
		"position":   user.Position,
		"user_type":  user.UserType,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	}
	if initialPassword != "" {
		respData["initial_password"] = initialPassword
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    respData,
	})
}

// ActivateUser creates IAM user for an existing local user that has no iam_sub
// POST /api/users/:id/activate
func (h *UserStaffHandler) ActivateUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user ID is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", userID, tenantID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query user: " + err.Error()})
		}
		return
	}

	if user.IAMSub != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user already activated"})
		return
	}

	iamClient := services.NewIAMClient()
	username := user.Email
	createReq := &services.CreateUserRequest{
		Username: username,
		Name:     user.Name,
		Email:    user.Email,
		Phone:    user.Phone,
	}
	iamResp, err := iamClient.CreateUser(createReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM user creation failed: " + err.Error()})
		return
	}

	if iamResp.UserID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM user creation failed: empty user_id returned"})
		return
	}

	orgID := user.OrgID
	if orgID == "" {
		orgID = tenantID
	}

	iamRole := services.GetBusinessRole("site_member")
	if iamRole == "" {
		iamRole = "staff"
	}
	if err := iamClient.BindUserToOrganization(iamResp.UserID, orgID, iamRole, ""); err != nil {
		iamClient.DeleteUser(iamResp.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM bind user to organization failed: " + err.Error()})
		return
	}

	template := services.AllRoleTemplates["site_member"]
	if len(template.CusPermCodes) > 0 {
		cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(template.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
		if err := iamClient.SetUserCustomerPermissions(orgID, iamResp.UserID, cusPerm, cusPermExt); err != nil {
			iamClient.UnbindUserFromOrganization(iamResp.UserID, orgID, "")
			iamClient.DeleteUser(iamResp.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "IAM set user permissions failed: " + err.Error()})
			return
		}
	}

	status := iamResp.Status
	if status == "" {
		status = "active"
	}
	if err := db.Model(&user).Updates(map[string]interface{}{
		"iam_sub": iamResp.UserID,
		"status":  status,
	}).Error; err != nil {
		iamClient.UnbindUserFromOrganization(iamResp.UserID, orgID, "")
		iamClient.DeleteUser(iamResp.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update user iam_sub: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    gin.H{"iam_sub": iamResp.UserID},
	})
}

// UpdateUser updates existing user information
// PUT /api/users/:id
func (h *UserStaffHandler) UpdateUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user ID is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	// Check if user exists
	var existingUser models.User
	if err := db.Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", userID, tenantID).First(&existingUser).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query user: " + err.Error()})
		}
		return
	}

	var req struct {
		Name     string     `json:"name"`
		Phone    string     `json:"phone"`
		Email    string     `json:"email"`
		Position string     `json:"position"`
		UserType string     `json:"user_type"`
		SiteID   *uuid.UUID `json:"site_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}

	// Check uniqueness constraints for updated fields
	var conflicts []gin.H

	// Check phone
	if req.Phone != "" && req.Phone != existingUser.Phone {
		var existingUserTemp models.User
		if err := db.Where("tenant_id = ? AND phone = ? AND deleted_at IS NULL AND id != ?", tenantID, req.Phone, userID).First(&existingUserTemp).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUserTemp.ID,
				"name":  existingUserTemp.Name,
				"phone": existingUserTemp.Phone,
				"email": existingUserTemp.Email,
			})
		}
	}

	// Check email
	if req.Email != "" && req.Email != existingUser.Email {
		var existingUserTemp models.User
		if err := db.Where("tenant_id = ? AND email = ? AND deleted_at IS NULL AND id != ?", tenantID, req.Email, userID).First(&existingUserTemp).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUserTemp.ID,
				"name":  existingUserTemp.Name,
				"phone": existingUserTemp.Phone,
				"email": existingUserTemp.Email,
			})
		}
	}

	// Return conflicts if any
	if len(conflicts) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "user with same phone, email, or username already exists",
			"data":    conflicts,
		})
		return
	}

	// Update user
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Phone != "" {
		updates["phone"] = req.Phone
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.Position != "" {
		updates["position"] = req.Position
	}
	if req.UserType != "" {
		updates["user_type"] = req.UserType
	}
	if req.SiteID != nil {
		newSiteIDStr := req.SiteID.String()

		if *req.SiteID == uuid.Nil {
			updates["org_id"] = nil

			if existingUser.IAMSub != "" {
				oldOrgID := existingUser.OrgID
				if oldOrgID != "" {
					iamClient := services.NewIAMClient()
					if err := iamClient.UnbindUserFromOrganization(existingUser.IAMSub, oldOrgID, ""); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to unbind user from organization: " + err.Error()})
						return
					}
				}
			}
		} else {
			siteIDChanged := true

			if siteIDChanged {
				iamClient := services.NewIAMClient()
				var newOrgID, oldOrgID string

				var newSite models.Site
				if err := db.Where("id = ? AND tenant_id = ?", newSiteIDStr, tenantID).First(&newSite).Error; err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "site not found"})
					return
				}
				newOrgID = newSite.OrgID
				if newOrgID == "" {
					newOrgID = tenantID
				}

			if existingUser.OrgID != "" {
				oldOrgID = existingUser.OrgID
			}

				if existingUser.IAMSub != "" {
					oldUnbound := false
					if oldOrgID != "" {
						if err := iamClient.UnbindUserFromOrganization(existingUser.IAMSub, oldOrgID, ""); err != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to unbind user from old organization: " + err.Error()})
							return
						}
						oldUnbound = true
					}

					if newOrgID != "" {
						iamRole := services.GetBusinessRole(existingUser.Role)
						if iamRole == "" {
							iamRole = "staff"
						}
						if err := iamClient.BindUserToOrganization(existingUser.IAMSub, newOrgID, iamRole, ""); err != nil {
							if oldUnbound && oldOrgID != "" {
								oldIamRole := services.GetBusinessRole(existingUser.Role)
								if oldIamRole == "" {
									oldIamRole = "staff"
								}
								if rebindErr := iamClient.BindUserToOrganization(existingUser.IAMSub, oldOrgID, oldIamRole, ""); rebindErr != nil {
									log.Printf("[CRITICAL] Compensation rebind failed for user %s to org %s: %v", existingUser.IAMSub, oldOrgID, rebindErr)
								}
							}
							c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to bind user to new organization: " + err.Error()})
							return
						}

						template, ok := services.AllRoleTemplates[existingUser.Role]
						if ok && len(template.CusPermCodes) > 0 {
							cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(template.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
							if err := iamClient.SetUserCustomerPermissions(newOrgID, existingUser.IAMSub, cusPerm, cusPermExt); err != nil {
								c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to set user permissions: " + err.Error()})
								return
							}
						}
					}
				}

				updates["org_id"] = newOrgID
			}
		}
	}

	if err := db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    gin.H{"id": userID},
	})
}

// GetCurrentUser returns the current user's profile
// GET /api/users/me
func (h *UserStaffHandler) GetCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var user models.User
	if err := db.Where("iam_sub = ? AND deleted_at IS NULL", userID).First(&user).Error; err != nil {
		// Fallback: try querying by local id (for users whose iam_sub doesn't match JWT sub)
		if err2 := db.Where("id = ? AND deleted_at IS NULL", userID).First(&user).Error; err2 != nil {
		result := gin.H{
			"id":            userID,
			"role":          middleware.GetRole(ctx),
			"business_role": middleware.GetBusinessRole(ctx),
			"gid":           middleware.GetGid(ctx),
			"sys_perm":      middleware.GetSysPerm(ctx),
			"cus_perm":      middleware.GetCusPerm(ctx),
			"cus_perm_ext":  middleware.GetCusPermExt(ctx),
			"site_id":       nil,
		}
		if orgID := middleware.GetOrgID(ctx); orgID != "" {
			var site models.Site
			if err := db.Where("org_id = ? AND status = ?", orgID, "active").First(&site).Error; err == nil {
				result["site_id"] = site.ID
				result["site_name"] = site.Name
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"message": "success",
			"data":    result,
		})
		return
	}
	}

	result := gin.H{
		"id":            user.ID,
		"name":          user.Name,
		"phone":         user.Phone,
		"email":         user.Email,
		"position":      user.Position,
		"user_type":     user.UserType,
		"force_password_change": user.ForcePasswordChange,
		"role":          middleware.GetRole(ctx),
		"business_role": middleware.GetBusinessRole(ctx),
		"gid":           middleware.GetGid(ctx),
		"sys_perm":      middleware.GetSysPerm(ctx),
		"cus_perm":      middleware.GetCusPerm(ctx),
		"cus_perm_ext":  middleware.GetCusPermExt(ctx),
		"site_id":       nil,
	}

	// Load user's primary site from site_members
	var memberSite struct{ SiteID string; SiteName string }
	if err := db.Table("site_members").
		Select("site_members.site_id, sites.name as site_name").
		Joins("JOIN sites ON sites.id = site_members.site_id").
		Where("site_members.user_id = ?", user.ID).
		First(&memberSite).Error; err == nil && memberSite.SiteID != "" {
		result["site_id"] = memberSite.SiteID
		result["site_name"] = memberSite.SiteName
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    result,
	})
}

// UpdateCurrentUser PUT /api/users/me
func (h *UserStaffHandler) UpdateCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}

	callbackURL := fmt.Sprintf("https://%s/api/iam/confirmation-callback", c.Request.Host)

	iamClient := services.NewIAMClient()
	iamReq := &services.UpdateUserRequest{
		Name:        req.Name,
		Email:       req.Email,
		Phone:       req.Phone,
		Password:    req.Password,
		CallbackURL: callbackURL,
		OperatorID:  userID,
	}

	if err := iamClient.UpdateUser(userID, iamReq); err != nil {
		log.Printf("[UpdateCurrentUser] IAM UpdateUser failed for %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update user in IAM: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	localUpdates := map[string]interface{}{}
	if req.Name != "" {
		localUpdates["name"] = req.Name
	}
	if req.Phone != "" {
		localUpdates["phone"] = req.Phone
	}
	if len(localUpdates) > 0 {
		db.Model(&models.User{}).Where("iam_sub = ? AND tenant_id = ?", userID, tenantID).Updates(localUpdates)
	}

	emailChanged := req.Email != ""
	responseData := gin.H{
		"status": "success",
	}
	if emailChanged {
		responseData["email_confirmation"] = "pending"
		responseData["message"] = "Email change requires confirmation. IAM will send a confirmation email."
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    responseData,
	})
}

// CheckUserExists checks if a user exists by phone/email/username
// GET /api/users/check
func (h *UserStaffHandler) CheckUserExists(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	phone := c.Query("phone")
	email := c.Query("email")
	username := c.Query("username")

	if phone == "" && email == "" && username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "phone, email, or username is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)
	scope := "tenant_id = ? AND deleted_at IS NULL"
	args := []interface{}{tenantID}

	var orClauses []string
	if phone != "" {
		orClauses = append(orClauses, "phone = ?")
		args = append(args, phone)
	}
	if email != "" {
		orClauses = append(orClauses, "email = ?")
		args = append(args, email)
	}
	if username != "" {
		orClauses = append(orClauses, "username = ?")
		args = append(args, username)
	}

	query := scope + " AND (" + strings.Join(orClauses, " OR ") + ")"
	var users []models.User
	if err := db.Where(query, args...).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query user: " + err.Error()})
		return
	}

	if len(users) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"message": "user not found",
			"data":    gin.H{"exists": false},
		})
		return
	}

	var conflictList []gin.H
	for _, u := range users {
		conflictList = append(conflictList, gin.H{
			"id":       u.ID,
			"name":     u.Name,
			"phone":    u.Phone,
			"email":    u.Email,
			"username": u.Username,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"exists": true,
			"user":   conflictList[0],
			"users":  conflictList,
		},
	})
}

// BatchDeleteUsers soft-deletes multiple users
// DELETE /api/users/batch
func (h *UserStaffHandler) BatchDeleteUsers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		IDs []string `json:"ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "ids is required and must not be empty"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var users []models.User
	if err := db.Where("id IN ? AND tenant_id = ? AND deleted_at IS NULL", req.IDs, tenantID).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query users: " + err.Error()})
		return
	}

	now := time.Now()
	deleted := 0
	iamClient := services.NewIAMClient()

	for _, user := range users {
		if err := db.Model(&models.User{}).Where("id = ?", user.ID).Update("deleted_at", now).Error; err != nil {
			log.Printf("[BatchDeleteUsers] failed to soft-delete user %s: %v", user.ID, err)
			continue
		}
		deleted++

		if user.IAMSub != "" {
			if err := iamClient.DeleteUser(user.IAMSub); err != nil {
				log.Printf("[BatchDeleteUsers] IAM DeleteUser failed for %s: %v", user.IAMSub, err)
			}
			if user.OrgID != "" {
				if err := iamClient.UnbindUserFromOrganization(user.IAMSub, user.OrgID, ""); err != nil {
					log.Printf("[BatchDeleteUsers] IAM UnbindUser failed for %s from org %s: %v", user.IAMSub, user.OrgID, err)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"deleted": deleted,
			"failed":  len(req.IDs) - deleted,
		},
	})
}

// ResetPassword sends password reset emails to users
// POST /api/users/reset-password
func (h *UserStaffHandler) ResetPassword(c *gin.Context) {
	var req struct {
		UserIDs     []string `json:"user_ids" binding:"required"`
		RedirectURL string   `json:"redirect_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_ids is required and must not be empty"})
		return
	}

	// Map local user IDs to IAM subs
	db := database.GetDB().WithContext(c.Request.Context())
	var users []models.User
	if err := db.Where("id IN ? AND tenant_id = ? AND deleted_at IS NULL", req.UserIDs, middleware.GetTenantID(c.Request.Context())).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query users: " + err.Error()})
		return
	}

	var iamSubs []string
	for _, user := range users {
		if user.IAMSub != "" {
			iamSubs = append(iamSubs, user.IAMSub)
		}
	}
	if len(iamSubs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"message": "success",
			"data":    gin.H{"sent": 0, "skipped": len(req.UserIDs)},
		})
		return
	}

	userToken := services.ExtractUserToken(c)
	if userToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "user token is required"})
		return
	}

	redirectURL := req.RedirectURL
	if redirectURL == "" {
		redirectURL = getResetPasswordRedirectURL(c)
	}

	iamClient := services.NewIAMClient()
	result, err := iamClient.ResetPasswordWithToken(userToken, iamSubs, redirectURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset password: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"sent":    result.Sent,
			"skipped": result.Skipped,
		},
	})
}

// getResetPasswordRedirectURL returns the Tuneloop public URL for IAM to redirect after password reset.
// Priority: TUNELOOP_WWW_URL > derived from IAM_PC_REDIRECT_URI (strip /callback) > default localhost
func getResetPasswordRedirectURL(c *gin.Context) string {
	// 1. Detect client type from Referer or User-Agent
	referer := c.GetHeader("Referer")
	ua := c.GetHeader("User-Agent")
	isWechat := strings.Contains(referer, ":5553") || strings.Contains(referer, ":5556") || strings.Contains(ua, "MicroMessenger")

	if isWechat {
		// WeChat end: use EXTERNAL_MOBILE_URL directly
		if redirectURI := os.Getenv("EXTERNAL_MOBILE_URL"); redirectURI != "" {
			if u, err := url.Parse(redirectURI); err == nil {
				return u.String()
			}
		}
	} else {
		// PC Web end: use EXTERNAL_WEB_URL directly
		if redirectURI := os.Getenv("EXTERNAL_WEB_URL"); redirectURI != "" {
			if u, err := url.Parse(redirectURI); err == nil {
				return u.String()
			}
		}
	}

	// 2. Explicit Tuneloop WWW URL
	if wwwURL := os.Getenv("TUNELOOP_WWW_URL"); wwwURL != "" {
		return strings.TrimSuffix(wwwURL, "/")
	}

	// 3. Fallback to legacy env var
	if redirectURI := os.Getenv("IAM_REDIRECT_URI"); redirectURI != "" {
		if u, err := url.Parse(redirectURI); err == nil {
			u.Path = ""
			u.RawQuery = ""
			u.Fragment = ""
			return u.String()
		}
	}

	// 4. Default (should not reach here in production)
	return "http://localhost:5554"
}

// getDescendantSiteIDs returns the given site ID and all recursive descendant site IDs.
func getDescendantSiteIDs(db *gorm.DB, tenantID string, siteID uuid.UUID) ([]uuid.UUID, error) {
	var allSites []models.Site
	if err := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&allSites).Error; err != nil {
		return nil, err
	}
	children := make(map[uuid.UUID][]uuid.UUID)
	for _, s := range allSites {
		if s.ParentID != nil {
			children[*s.ParentID] = append(children[*s.ParentID], uuid.MustParse(s.ID))
		}
	}
	ids := []uuid.UUID{siteID}
	queue := []uuid.UUID{siteID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, childID := range children[current] {
			ids = append(ids, childID)
			queue = append(queue, childID)
		}
	}
	return ids, nil
}

func generatePassword() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const uppers = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const all = letters + uppers + digits

	b := make([]byte, 12)
	charlen := big.NewInt(int64(len(all)))
	digitlen := big.NewInt(int64(len(digits)))
	upperlen := big.NewInt(int64(len(uppers)))
	letterlen := big.NewInt(int64(len(letters)))

	n1, _ := rand.Int(rand.Reader, digitlen)
	b[0] = digits[n1.Int64()]
	n2, _ := rand.Int(rand.Reader, upperlen)
	b[1] = uppers[n2.Int64()]
	n3, _ := rand.Int(rand.Reader, letterlen)
	b[2] = letters[n3.Int64()]
	for i := 3; i < 12; i++ {
		n, _ := rand.Int(rand.Reader, charlen)
		b[i] = all[n.Int64()]
	}
	mrand.Shuffle(12, func(i, j int) { b[i], b[j] = b[j], b[i] })
	return string(b)
}

var (
	reUpper   = regexp.MustCompile(`[A-Z]`)
	reLower   = regexp.MustCompile(`[a-z]`)
	reDigit   = regexp.MustCompile(`[0-9]`)
)

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码长度不能少于 8 位")
	}
	if !reUpper.MatchString(password) {
		return fmt.Errorf("密码必须包含大写字母")
	}
	if !reLower.MatchString(password) {
		return fmt.Errorf("密码必须包含小写字母")
	}
	if !reDigit.MatchString(password) {
		return fmt.Errorf("密码必须包含数字")
	}
	return nil
}
