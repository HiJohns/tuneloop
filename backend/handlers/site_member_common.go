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

// errAlreadyRegistered is returned when adding a member whose phone/email
// already belongs to an account. #2025 D2 / #2028 Step 2（一人一号）：不允许
// 管理员把该身份挂接到既有账户（防误绑），改由本人登录后自助加入。
const errAlreadyRegistered = "该手机号/邮箱已在本平台注册，不能由管理员直接添加；请让本人登录后自助加入（作为员工加入 / 注册为顾客）"

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

	// #2114 P1/P2/P3 门控：网点级角色不得直接添加「非本商户成员」（P2 已注册 / P3 未注册），
	// 需经商户管理员审批（提交邀请申请）。商户管理员 / 系统管理员 / 平台员工不受限。
	callerRole := middleware.GetBusinessRole(c.Request.Context())
	adminLevel := callerRole == middleware.BusinessRoleMerchantAdmin ||
		callerRole == middleware.BusinessRoleSystemAdmin ||
		callerRole == middleware.BusinessRolePlatformStaff ||
		site.Type == "transit" // #1938 中转网点为管理侧维护，不适用 #2114 门控
	needApplyFail := func() *addMemberFail {
		return &addMemberFail{http.StatusForbidden, gin.H{
			"code":    40311,
			"message": "您没有权限邀请该用户，需商户管理员同意，是否提出申请？",
			"data":    gin.H{"need_apply": true},
		}}
	}

	// P3：网点级角色不得直接创建新账户加入网点（须由商户管理员创建/邀请）
	if len(input.NewUsers) > 0 && !adminLevel {
		return res, needApplyFail()
	}

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
					// #2025 D2 / #2028 Step 2（一人一号）：该手机号/邮箱已有账户。
					// 不允许管理员从冲突清单里挑一个既有用户代为挂接（误绑风险），
					// 也不做静默复用。由本人登录后自助加入（作为员工加入 / 注册为顾客）。
					return res, &addMemberFail{http.StatusConflict, gin.H{
						"code":    40902,
						"message": errAlreadyRegistered,
						"data":    gin.H{"registered": true},
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
				// #2025 D2 / #2028 Step 2：既有账户不得由管理员挂接（一人一号）。
				// 原实现会直接用冲突用户 + 绑定本地缓存，属「管理员代挂」，已移除。
				return res, &addMemberFail{http.StatusConflict, gin.H{
					"code":    40902,
					"message": errAlreadyRegistered,
					"data":    gin.H{"registered": true},
				}}
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

		// #2114 P1：网点级角色只能直接添加「已是本商户成员」的用户（P2/P3 → 申请）
		if !adminLevel && !isTenantMember(db, tenantID, userID) {
			return res, needApplyFail()
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
			Roles:    []string{normalizedRole}, // #2034 多重角色（集）
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
