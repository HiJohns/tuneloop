package handlers

import (
	"log"
	"net/http"
	"strconv"
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

	// Batch load site names
	siteNames := make(map[string]string)
	var siteIDs []string
	for _, user := range users {
		if user.SiteID != nil && *user.SiteID != "" {
			siteIDs = append(siteIDs, *user.SiteID)
		}
	}
	if len(siteIDs) > 0 {
		var sites []models.Site
		db.Where("id IN ?", siteIDs).Find(&sites)
		for _, s := range sites {
			siteNames[s.ID] = s.Name
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
			"role":       user.Role,
			"status":     user.Status,
			"iam_sub":    user.IAMSub,
			"org_id":     user.OrgID,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		}
		if user.SiteID != nil {
			item["site_id"] = *user.SiteID
			if name, ok := siteNames[*user.SiteID]; ok {
				item["site_name"] = name
			}
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
		Name     string    `json:"name" binding:"required"`
		Phone    string    `json:"phone" binding:"required"`
		Email    string    `json:"email"`
		Position string    `json:"position"`
		UserType string    `json:"user_type"`
		SiteID   uuid.UUID `json:"site_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}

	// Check uniqueness constraints (name OR phone OR email)
	db := database.GetDB().WithContext(ctx)
	var conflicts []gin.H

	// Check name
	if req.Name != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND name = ? AND deleted_at IS NULL", tenantID, req.Name).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUser.ID,
				"name":  existingUser.Name,
				"phone": existingUser.Phone,
				"email": existingUser.Email,
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
			"message": "user with same name, phone, or email already exists",
			"data":    conflicts,
		})
		return
	}

	// Create user
	user := models.User{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Name:      req.Name,
		Phone:     req.Phone,
		Email:     req.Email,
		Position:  req.Position,
		UserType:  req.UserType,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if req.SiteID != uuid.Nil {
		siteIDStr := req.SiteID.String()
		user.SiteID = &siteIDStr
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user: " + err.Error()})
		return
	}

	iamClient := services.NewIAMClient()
	createReq := &services.CreateUserRequest{
		Username: req.Email,
		Name:     req.Name,
		Email:    req.Email,
		Phone:    req.Phone,
	}
	if _, err := iamClient.CreateUser(createReq); err != nil {
		log.Printf("[UserStaff] IAM CreateUser failed for %s: %v", req.Email, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"phone":      user.Phone,
			"email":      user.Email,
			"position":   user.Position,
			"user_type":  user.UserType,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
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
		Name     string    `json:"name"`
		Phone    string    `json:"phone"`
		Email    string    `json:"email"`
		Position string    `json:"position"`
		UserType string    `json:"user_type"`
		SiteID   uuid.UUID `json:"site_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request body: " + err.Error()})
		return
	}

	// Check uniqueness constraints for updated fields
	var conflicts []gin.H

	// Check name
	if req.Name != "" && req.Name != existingUser.Name {
		var existingUserTemp models.User
		if err := db.Where("tenant_id = ? AND name = ? AND deleted_at IS NULL AND id != ?", tenantID, req.Name, userID).First(&existingUserTemp).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":    existingUserTemp.ID,
				"name":  existingUserTemp.Name,
				"phone": existingUserTemp.Phone,
				"email": existingUserTemp.Email,
			})
		}
	}

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
			"message": "user with same name, phone, or email already exists",
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
	if req.SiteID != uuid.Nil {
		siteIDStr := req.SiteID.String()
		updates["site_id"] = &siteIDStr
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
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var user models.User
	if err := db.Where("iam_sub = ? AND tenant_id = ? AND deleted_at IS NULL", userID, tenantID).First(&user).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"message": "success",
			"data": gin.H{
				"id":            userID,
				"role":          middleware.GetRole(ctx),
				"business_role": middleware.GetBusinessRole(ctx),
				"gid":           middleware.GetGid(ctx),
				"sys_perm":      middleware.GetSysPerm(ctx),
				"cus_perm":      middleware.GetCusPerm(ctx),
				"cus_perm_ext":  middleware.GetCusPermExt(ctx),
				"site_id":       nil,
			},
		})
		return
	}

	result := gin.H{
		"id":            user.ID,
		"name":          user.Name,
		"phone":         user.Phone,
		"email":         user.Email,
		"position":      user.Position,
		"user_type":     user.UserType,
		"role":          middleware.GetRole(ctx),
		"business_role": middleware.GetBusinessRole(ctx),
		"gid":           middleware.GetGid(ctx),
		"sys_perm":      middleware.GetSysPerm(ctx),
		"cus_perm":      middleware.GetCusPerm(ctx),
		"cus_perm_ext":  middleware.GetCusPermExt(ctx),
		"site_id":       nil,
	}
	if user.SiteID != nil {
		result["site_id"] = *user.SiteID
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    result,
	})
}

// CheckUserExists checks if a user exists by phone/email
// GET /api/users/check
func (h *UserStaffHandler) CheckUserExists(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	phone := c.Query("phone")
	email := c.Query("email")

	if phone == "" && email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "phone or email is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)
	query := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

	if phone != "" {
		query = query.Where("phone = ?", phone)
	} else if email != "" {
		query = query.Where("email = ?", email)
	}

	var user models.User
	if err := query.First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{
				"code":    20000,
				"message": "user not found",
				"data":    gin.H{"exists": false},
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query user: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"exists": true,
			"user": gin.H{
				"id":    user.ID,
				"name":  user.Name,
				"phone": user.Phone,
				"email": user.Email,
			},
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

// ResendConfirmation resends confirmation email to users
// POST /api/users/resend-confirmation
func (h *UserStaffHandler) ResendConfirmation(c *gin.Context) {
	var req struct {
		UserIDs []string `json:"user_ids" binding:"required"`
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

	iamClient := services.NewIAMClient()
	result, err := iamClient.ResendConfirmationWithToken(userToken, iamSubs)
	if err != nil {
		log.Printf("[ResendConfirmation] IAM ResendConfirmation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to resend confirmation: " + err.Error()})
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
