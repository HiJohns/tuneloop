package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MerchantMemberHandler struct{}

func NewMerchantMemberHandler() *MerchantMemberHandler {
	return &MerchantMemberHandler{}
}

// ListMembers GET /admin/merchants/:id/members - List merchant members
func (h *MerchantMemberHandler) ListMembers(c *gin.Context) {
	merchantID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	if !hasMerchantAccess(db, tenantID, merchantID, c) {
		return
	}

	var members []struct {
		UserID    string    `json:"user_id"`
		UserName  string    `json:"user_name"`
		UserEmail string    `json:"user_email"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
	}

	db.Table("merchant_members").
		Select("merchant_members.user_id, users.name as user_name, users.email as user_email, merchant_members.role, merchant_members.created_at").
		Joins("JOIN users ON users.id = merchant_members.user_id").
		Where("merchant_members.merchant_id = ? AND merchant_members.tenant_id = ?", merchantID, tenantID).
		Scan(&members)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": members,
		},
	})
}

// AddMember POST /admin/merchants/:id/members - Add member to merchant.
// The IAM binding org for a merchant is its own tenant_id.
func (h *MerchantMemberHandler) AddMember(c *gin.Context) {
	merchantID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	if !hasMerchantAccess(db, tenantID, merchantID, c) {
		return
	}

	var input struct {
		UserID         string                   `json:"user_id"`
		Role           string                   `json:"role"`
		UserIDs        []map[string]interface{} `json:"user_ids"`
		SkipActivation bool                     `json:"skip_activation"`
		NewUsers       []struct {
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
			Phone    string `json:"phone"`
			Role     string `json:"role"`
		} `json:"new_users"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	var usersToProcess []map[string]interface{}

	if input.UserID != "" && len(input.UserIDs) == 0 {
		usersToProcess = []map[string]interface{}{
			{"user_id": input.UserID, "role": input.Role},
		}
	} else if len(input.UserIDs) > 0 {
		usersToProcess = input.UserIDs
	} else if len(input.NewUsers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Either user_id, user_ids, or new_users must be provided",
		})
		return
	}

	var merchant models.Merchant
	if err := db.Where("id = ? AND tenant_id = ?", merchantID, tenantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Merchant not found"})
		return
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)
	operatorID := middleware.GetUserID(c.Request.Context())

	var directlyAdded []gin.H
	var bindErrors []gin.H
	var initialPasswords []gin.H
	var roleErrors []gin.H

	if len(input.NewUsers) > 0 {
		for _, nu := range input.NewUsers {
			callbackURL := os.Getenv("EXTERNAL_WEB_URL")
			if callbackURL == "" {
				callbackURL = fmt.Sprintf("http://%s", c.Request.Host)
			}
			nuUsername := nu.Username
			if nuUsername == "" {
				nuUsername = nu.Email
			}
			createReq := &services.CreateUserRequest{
				Username:         nuUsername,
				Name:             nu.Name,
				Email:            nu.Email,
				Phone:            nu.Phone,
				Reason:           "商户成员 - " + merchant.Name,
				OperatorID:       operatorID,
				SkipActivation:   input.SkipActivation,
				Password:         generatePassword(),
				SendNotificationEmail: input.SkipActivation,
			}
			if input.SkipActivation {
				createReq.NotificationLang = middleware.GetCulture(c)
				log.Printf("[MerchantAddMember] skip_activation=true, generated password for %s", nu.Email)
			} else {
				createReq.CallbackURL = callbackURL
			}
			userResult, err := iamClient.CreateOrGetUser(userToken, createReq)
			if err != nil {
				var conflictErr *services.UsernameConflictError
				if errors.As(err, &conflictErr) {
					var memberCount int64
					db.Model(&models.MerchantMember{}).
						Where("user_id = ? AND tenant_id = ?", conflictErr.UserID, tenantID).
						Count(&memberCount)

					type merchantInfo struct {
						MerchantName string `json:"merchant_name"`
						Role         string `json:"role"`
					}
					var merchants []merchantInfo
					db.Table("merchant_members").
						Select("merchants.name AS merchant_name, merchant_members.role").
						Joins("JOIN merchants ON merchants.id = merchant_members.merchant_id").
						Where("merchant_members.user_id = ? AND merchant_members.tenant_id = ?", conflictErr.UserID, tenantID).
						Scan(&merchants)

					c.JSON(http.StatusConflict, gin.H{
						"code": 40901,
						"data": gin.H{
							"conflicts": []gin.H{{
								"user_id":       conflictErr.UserID,
								"name":          conflictErr.Name,
								"email":         conflictErr.Email,
								"phone":         conflictErr.Phone,
								"username":      conflictErr.Username,
								"same_merchant": memberCount > 0,
								"merchants":     merchants,
								"error":         conflictErr.Error(),
							}},
						},
					})
					return
				}
				log.Printf("[MerchantAddMember] Failed to create user %s: %v", nu.Email, err)
				bindErrors = append(bindErrors, gin.H{
					"email": nu.Email,
					"error": err.Error(),
				})
				continue
			}
			if userResult.Conflict {
				log.Printf("[MerchantAddMember] User already exists in IAM: %s, proceeding with binding", userResult.UserID)

				var existingLocal models.User
				if err := db.Where("iam_sub = ?", userResult.UserID).First(&existingLocal).Error; err != nil {
					existingUser := userResult.ExistingUsers[0]
					localUser := models.User{
						ID:       userResult.UserID,
						IAMSub:   userResult.UserID,
						TenantID: tenantID,
						OrgID:    tenantID,
						Name:     existingUser.Name,
						Email:    existingUser.Email,
						Phone:    nu.Phone,
						Role:     "site_member",
						Status:   "active",
					}
					if err := db.Create(&localUser).Error; err != nil {
						log.Printf("[MerchantAddMember] Failed to create local user for existing IAM user %s: %v", userResult.UserID, err)
					}
				} else {
					existingUser := userResult.ExistingUsers[0]
					updates := map[string]interface{}{
						"deleted_at": nil,
						"name":       existingUser.Name,
						"email":      existingUser.Email,
						"status":     "active",
					}
					if existingLocal.Phone == "" {
						updates["phone"] = nu.Phone
					}
					if err := db.Model(&models.User{}).
						Where("id = ? AND tenant_id = ?", existingLocal.ID, tenantID).
						Updates(updates).Error; err != nil {
						log.Printf("[MerchantAddMember] Failed to restore local user cache %s: %v", existingLocal.ID, err)
					}
				}

				role := nu.Role
				if role == "" {
					role = "site_member"
				}
				usersToProcess = append(usersToProcess, map[string]interface{}{
					"user_id": userResult.UserID,
					"role":    role,
				})
				continue
			}

			if input.SkipActivation && createReq.Password != "" {
				initialPasswords = append(initialPasswords, gin.H{
					"email":    nu.Email,
					"password": createReq.Password,
				})
			}

			localUser := models.User{
				ID:       userResult.UserID,
				IAMSub:   userResult.UserID,
				TenantID: tenantID,
				OrgID:    tenantID,
				Name:     nu.Name,
				Email:    nu.Email,
				Phone:    nu.Phone,
				Role:     "site_member",
				Status:   "active",
			}
			if err := db.Create(&localUser).Error; err != nil {
				log.Printf("[MerchantAddMember] Failed to create local user %s: %v", userResult.UserID, err)
			}
			role := nu.Role
			if role == "" {
				role = "site_member"
			}
			usersToProcess = append(usersToProcess, map[string]interface{}{
				"user_id": userResult.UserID,
				"role":    role,
			})
		}
	}

	for _, userEntry := range usersToProcess {
		userID, ok := userEntry["user_id"].(string)
		if !ok || userID == "" {
			continue
		}

		role := input.Role
		if r, ok := userEntry["role"].(string); ok && r != "" {
			role = r
		}

		var count int64
		db.Model(&models.MerchantMember{}).
			Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", tenantID, merchantID, userID).
			Count(&count)
		if count > 0 {
			continue
		}

		normalizedRole := normalizeRole(role)
		iamRole := toIAMRole(normalizedRole)
		templateCode := normalizedRole

		if err := iamClient.BindUserToOrganizationWithToken(userToken, userID, tenantID, iamRole, operatorID); err != nil {
			log.Printf("[MerchantAddMember] IAM BindUser failed for user %s to org %s: %v", userID, tenantID, err)
			bindErrors = append(bindErrors, gin.H{
				"user_id": userID,
				"error":   err.Error(),
			})
			continue
		}
		nsID := middleware.GetNamespaceID(c.Request.Context())
		if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
			for _, t := range templates {
				if t.Code == templateCode {
					if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, userID, tenantID, t.Code); err != nil {
						log.Printf("[MerchantAddMember] AssignRoleTemplate failed for user %s code %s: %v", userID, templateCode, err)
						roleErrors = append(roleErrors, gin.H{
							"user_id":       userID,
							"template_code": templateCode,
							"error":         err.Error(),
						})
					}
					break
				}
			}
		} else {
			log.Printf("[MerchantAddMember] ListRoleTemplates failed: %v", err)
			roleErrors = append(roleErrors, gin.H{
				"error": "failed to list role templates: " + err.Error(),
			})
		}

		member := models.MerchantMember{
			TenantID:   tenantID,
			MerchantID: merchantID,
			UserID:     userID,
			Role:       role,
		}
		result := db.Create(&member)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": fmt.Sprintf("Failed to add member %s: %s", userID, result.Error.Error()),
			})
			return
		}

		directlyAdded = append(directlyAdded, gin.H{
			"user_id": userID,
			"role":    role,
		})
	}

	if len(directlyAdded) == 0 && len(bindErrors) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "No valid users to add",
		})
		return
	}

	responseData := gin.H{
		"merchant_id":     merchantID,
		"directly_added":  directlyAdded,
	}
	if len(bindErrors) > 0 {
		responseData["bind_errors"] = bindErrors
	}
	if len(initialPasswords) > 0 {
		responseData["initial_passwords"] = initialPasswords
	}
	if len(roleErrors) > 0 {
		responseData["role_errors"] = roleErrors
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": responseData,
	})
}

// UpdateMemberRole PUT /admin/merchants/:id/members/:uid - Update member role
func (h *MerchantMemberHandler) UpdateMemberRole(c *gin.Context) {
	merchantID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	if !hasMerchantAccess(db, tenantID, merchantID, c) {
		return
	}

	var input struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	var iamUser models.User
	if err := db.Where("id = ?", userID).First(&iamUser).Error; err == nil && iamUser.IAMSub != "" {
		normalizedRole := normalizeRole(input.Role)
		iamRole := toIAMRole(normalizedRole)
		templateCode := normalizedRole

		iamClient := services.NewIAMClient()
		userToken := services.ExtractUserToken(c)

		if normalizedRole == "merchant_admin" {
			if bindErr := iamClient.BindUserToOrganizationWithToken(userToken, iamUser.IAMSub, tenantID, iamRole, middleware.GetUserID(c.Request.Context())); bindErr != nil {
				log.Printf("[MerchantUpdateRole] BindUser failed for %s: %v", iamUser.IAMSub, bindErr)
			}
		} else {
			if demoteErr := iamClient.UpdateUserRoleInOrgWithToken(userToken, tenantID, iamUser.IAMSub, iamRole); demoteErr != nil {
				log.Printf("[MerchantUpdateRole] UpdateRole failed for %s: %v", iamUser.IAMSub, demoteErr)
			}
		}

		nsID := middleware.GetNamespaceID(c.Request.Context())
		if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
			for _, t := range templates {
				if t.Code == templateCode {
					if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, iamUser.IAMSub, tenantID, t.Code); err != nil {
						log.Printf("[MerchantUpdateRole] AssignRoleTemplate failed: %v", err)
					}
					break
				}
			}
		} else {
			log.Printf("[MerchantUpdateRole] ListRoleTemplates failed: %v", err)
		}
	}

	result := db.Model(&models.MerchantMember{}).
		Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", tenantID, merchantID, userID).
		Update("role", input.Role)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update member role: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"merchant_id": merchantID,
			"user_id":     userID,
			"new_role":    input.Role,
		},
	})
}

// RemoveMember DELETE /admin/merchants/:id/members/:uid - Remove member from merchant
func (h *MerchantMemberHandler) RemoveMember(c *gin.Context) {
	merchantID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	if !hasMerchantAccess(db, tenantID, merchantID, c) {
		return
	}

	iamClient := services.NewIAMClient()
	operatorID := middleware.GetUserID(c.Request.Context())
	memberToken := services.ExtractUserToken(c)
	if err := iamClient.UnbindUserFromOrganizationWithToken(memberToken, userID, tenantID, operatorID); err != nil {
		log.Printf("[MerchantRemoveMember] IAM UnbindUser failed for user %s from org %s: %v", userID, tenantID, err)
	}

	result := db.Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", tenantID, merchantID, userID).
		Delete(&models.MerchantMember{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to remove member: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	// Local cache hygiene: if the user has no remaining merchant memberships
	// in this tenant, soft-delete the cached users row.
	var remaining int64
	db.Model(&models.MerchantMember{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Count(&remaining)
	if remaining == 0 {
		now := time.Now()
		if err := db.Model(&models.User{}).
			Where("id = ? AND tenant_id = ?", userID, tenantID).
			Update("deleted_at", now).Error; err != nil {
			log.Printf("[MerchantRemoveMember] Failed to soft-delete cached user %s: %v", userID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Member removed successfully",
	})
}

// hasMerchantAccess checks the merchant belongs to the requesting tenant.
func hasMerchantAccess(db *gorm.DB, tenantID, merchantID string, c *gin.Context) bool {
	var count int64
	result := db.Model(&models.Merchant{}).
		Where("id = ? AND tenant_id = ?", merchantID, tenantID).
		Count(&count)

	if result.Error != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return false
	}
	return true
}
