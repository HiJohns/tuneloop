package handlers

// #1938: 站点 / 中转网点共享的成员添加核心。
// 自 SiteMemberHandler.AddMember 提取，保持既有行为不变，供中转中心复用。
// 契约：user_id(+role) | user_ids:[{user_id,role}] | new_users:[{username,name,email,phone,role}] + skip_activation。

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type addMemberUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
}

type addMemberInput struct {
	UserID         string                   `json:"user_id"`
	Role           string                   `json:"role"`
	UserIDs        []map[string]interface{} `json:"user_ids"`
	SkipActivation bool                     `json:"skip_activation"`
	NewUsers       []addMemberUser          `json:"new_users"`
}

type addMemberResult struct {
	DirectlyAdded    []gin.H
	BindErrors       []gin.H
	InitialPasswords []gin.H
	RoleErrors       []gin.H
}

type addMemberFail struct {
	Status int
	Body   gin.H
}

// addMembersCore 执行成员添加（新建用户 → IAM 绑定 → 角色模板 → 本地缓存）。
// 返回 fail 非 nil 时，调用方直接 `c.JSON(fail.Status, fail.Body)`。
func addMembersCore(c *gin.Context, db *gorm.DB, site models.Site, tenantID, siteID string, input addMemberInput) (addMemberResult, *addMemberFail) {
	res := addMemberResult{}

	// Determine user list to process
	var usersToProcess []map[string]interface{}
	if input.UserID != "" && len(input.UserIDs) == 0 {
		usersToProcess = []map[string]interface{}{{"user_id": input.UserID, "role": input.Role}}
	} else if len(input.UserIDs) > 0 {
		usersToProcess = input.UserIDs
	} else if len(input.NewUsers) == 0 {
		return res, &addMemberFail{http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Either user_id (old format), user_ids (new format), or new_users must be provided",
		}}
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)
	operatorID := middleware.GetUserID(c.Request.Context())

	// Process new_users: create or get users first, then add to processing list
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
				Username:       nuUsername,
				Name:           nu.Name,
				Email:          nu.Email,
				Phone:          nu.Phone,
				Reason:         "网点成员 - " + site.Name,
				OperatorID:     operatorID,
				SkipActivation: input.SkipActivation,
			}
			createReq.Password = generatePassword()
			createReq.SendNotificationEmail = input.SkipActivation
			if input.SkipActivation {
				createReq.NotificationLang = middleware.GetCulture(c)
				log.Printf("[AddMember] skip_activation=true, generated password for %s", nu.Email)
			} else {
				createReq.CallbackURL = callbackURL
			}
			userResult, err := iamClient.CreateOrGetUser(userToken, createReq)
			if err != nil {
				var conflictErr *services.UsernameConflictError
				if errors.As(err, &conflictErr) {
					var memberCount int64
					db.Model(&models.SiteMember{}).
						Where("user_id = ? AND tenant_id = ?", conflictErr.UserID, tenantID).
						Count(&memberCount)

					type siteInfo struct {
						SiteName string `json:"site_name"`
						Role     string `json:"role"`
					}
					var sites []siteInfo
					db.Table("site_members").
						Select("sites.name AS site_name, site_members.role").
						Joins("JOIN sites ON sites.id = site_members.site_id").
						Where("site_members.user_id = ? AND site_members.tenant_id = ?", conflictErr.UserID, tenantID).
						Scan(&sites)

					return res, &addMemberFail{http.StatusConflict, gin.H{
						"code": 40901,
						"data": gin.H{
							"conflicts": []gin.H{{
								"user_id":       conflictErr.UserID,
								"name":          conflictErr.Name,
								"email":         conflictErr.Email,
								"phone":         conflictErr.Phone,
								"username":      conflictErr.Username,
								"same_merchant": memberCount > 0,
								"orgs":          sites,
								"error":         conflictErr.Error(),
							}},
						},
					}}
				}
				log.Printf("[AddMember] Failed to create user %s: %v", nu.Email, err)
				res.BindErrors = append(res.BindErrors, gin.H{
					"email": nu.Email,
					"error": err.Error(),
				})
				continue
			}

			if userResult.Conflict {
				log.Printf("[AddMember] User already exists in IAM: %s, proceeding with binding", userResult.UserID)

				var existingLocal models.User
				if err := db.Where("iam_sub = ?", userResult.UserID).First(&existingLocal).Error; err != nil {
					existingUser := userResult.ExistingUsers[0]
					localUser := models.User{
						ID:       userResult.UserID,
						IAMSub:   userResult.UserID,
						TenantID: tenantID,
						OrgID:    site.OrgID,
						Name:     existingUser.Name,
						Email:    existingUser.Email,
						Phone:    nu.Phone,
						Role:     "site_member",
						Status:   "active",
					}
					if err := db.Create(&localUser).Error; err != nil {
						log.Printf("[AddMember] Failed to create local user for existing IAM user %s: %v", userResult.UserID, err)
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
						log.Printf("[AddMember] Failed to restore local user cache %s: %v", existingLocal.ID, err)
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
				res.InitialPasswords = append(res.InitialPasswords, gin.H{
					"email":    nu.Email,
					"password": createReq.Password,
				})
			}

			localUser := models.User{
				ID:       userResult.UserID,
				IAMSub:   userResult.UserID,
				TenantID: tenantID,
				OrgID:    site.OrgID,
				Name:     nu.Name,
				Email:    nu.Email,
				Phone:    nu.Phone,
				Role:     "site_member",
				Status:   "active",
			}
			if err := db.Create(&localUser).Error; err != nil {
				log.Printf("[AddMember] Failed to create local user %s: %v", userResult.UserID, err)
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
		db.Model(&models.SiteMember{}).
			Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, userID).
			Count(&count)
		if count > 0 {
			continue
		}

		normalizedRole := normalizeRole(role)
		iamRole := toIAMRole(normalizedRole)
		templateCode := normalizedRole

		if site.OrgID != "" {
			if err := iamClient.BindUserToOrganizationWithToken(userToken, userID, site.OrgID, iamRole, operatorID); err != nil {
				log.Printf("[AddMember] IAM BindUser failed for user %s to org %s: %v", userID, site.OrgID, err)
				res.BindErrors = append(res.BindErrors, gin.H{
					"user_id": userID,
					"error":   err.Error(),
				})
				continue
			}
			nsID := middleware.GetNamespaceID(c.Request.Context())
			if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
				for _, t := range templates {
					if t.Code == templateCode {
						if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, userID, site.OrgID, t.Code); err != nil {
							log.Printf("[AddMember] AssignRoleTemplate failed for user %s code %s: %v", userID, templateCode, err)
							res.RoleErrors = append(res.RoleErrors, gin.H{
								"user_id":       userID,
								"template_code": templateCode,
								"error":         err.Error(),
							})
						}
						break
					}
				}
			} else {
				log.Printf("[AddMember] ListRoleTemplates failed: %v", err)
				res.RoleErrors = append(res.RoleErrors, gin.H{
					"error": "failed to list role templates: " + err.Error(),
				})
			}
		}

		member := models.SiteMember{
			TenantID: tenantID,
			SiteID:   siteID,
			UserID:   userID,
			Role:     role,
		}

		result := db.Create(&member)
		if result.Error != nil {
			return res, &addMemberFail{http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": fmt.Sprintf("Failed to add member %s: %s", userID, result.Error.Error()),
			}}
		}

		res.DirectlyAdded = append(res.DirectlyAdded, gin.H{
			"user_id": userID,
			"role":    role,
		})
	}

	return res, nil
}
