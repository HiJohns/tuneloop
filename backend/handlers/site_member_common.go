package handlers

// #1938: 站点 / 中转网点共享的成员添加核心。
// 自 SiteMemberHandler.AddMember 提取，保持既有行为不变，供中转中心复用。
// 契约：user_id(+role) | user_ids:[{user_id,role}] |
//       new_users:[{username,name,email,phone,role,password,auto_generate,force_password_change}] + skip_activation。

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

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
	// #2366 D1/D6：密码模式（缺省 = 邀请 / 旧 skip_activation 行为）。
	// password 与 auto_generate 互斥；force_password_change 缺省 false（保持旧行为）。
	Password            string `json:"password"`
	AutoGenerate        bool   `json:"auto_generate"`
	ForcePasswordChange bool   `json:"force_password_change"`
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

// newUserCreateReq 归一化 new_users 条目并构造 IAM 创建请求（#2366 D1/D6）。
// 网点成员（addMembersCore）与商户直属成员（merchant_member.AddMember）共用本函数，
// 避免「手动密码 / 自动生成 / 邀请」规则在两处漂移。
//
// 规则：
//  1. username 空且 email 空 → 400（无可用的登录标识）；
//  2. username 空且 email 非空 → username 显式取 email（旧行为，不再静默回退）；
//  3. password 与 auto_generate 同时出现 → 400（二选一）；
//  4. 手动密码 / 无邮箱自动生成 / 旧 skip_activation → 直接激活并一次性回显密码；
//  5. 有邮箱 + 自动生成 → 仍走邀请（D2：IAM 邮件流程，本任务不改变该分支）；
//  6. 无密码模式且无邮箱 → 400（邀请无投递通道）。
//
// 返回值 identifier 为实际登录名，用于 initial_passwords[].identifier 回显。
func newUserCreateReq(c *gin.Context, nu addMemberUser, reason, operatorID string, skipActivation bool) (*services.CreateUserRequest, string, *addMemberFail) {
	username := strings.TrimSpace(nu.Username)
	email := strings.TrimSpace(nu.Email)

	if username == "" {
		if email == "" {
			return nil, "", &addMemberFail{http.StatusBadRequest, gin.H{
				"code":    40001,
				"message": "username is required when email is empty",
			}}
		}
		username = email
	}

	if nu.Password != "" && nu.AutoGenerate {
		return nil, "", &addMemberFail{http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "password and auto_generate are mutually exclusive",
		}}
	}

	direct := false // true = 直接激活并一次性回显初始密码
	password := ""  // 仅 direct 时使用
	switch {
	case nu.Password != "":
		if err := validatePassword(nu.Password); err != nil {
			return nil, "", &addMemberFail{http.StatusBadRequest, gin.H{
				"code":    40001,
				"message": err.Error(),
			}}
		}
		direct, password = true, nu.Password
	case nu.AutoGenerate && email == "":
		direct, password = true, generatePassword()
	case skipActivation: // 保留旧 skip_activation 语义：服务端生成密码并一次性下发
		direct, password = true, generatePassword()
	case email == "":
		return nil, "", &addMemberFail{http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "email is required unless password or auto_generate is provided",
		}}
	}

	createReq := &services.CreateUserRequest{
		Username:            username,
		Name:                strings.TrimSpace(nu.Name),
		Email:               email,
		Phone:               strings.TrimSpace(nu.Phone),
		Reason:              reason,
		OperatorID:          operatorID,
		SkipActivation:      direct,
		ForcePasswordChange: nu.ForcePasswordChange,
	}
	if direct {
		createReq.Password = password
		createReq.SendNotificationEmail = true
		createReq.NotificationLang = middleware.GetCulture(c)
	} else {
		callbackURL := os.Getenv("EXTERNAL_WEB_URL")
		if callbackURL == "" {
			callbackURL = fmt.Sprintf("http://%s", c.Request.Host)
		}
		createReq.CallbackURL = callbackURL
	}
	return createReq, username, nil
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
			createReq, identifier, fail := newUserCreateReq(c, nu, "网点成员 - "+site.Name, operatorID, input.SkipActivation)
			if fail != nil {
				return res, fail
			}
			if createReq.Password != "" {
				log.Printf("[AddMember] direct activation with generated/initial password for %s", identifier)
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
				log.Printf("[AddMember] Failed to create user %s: %v", createReq.Email, err)
				res.BindErrors = append(res.BindErrors, gin.H{
					"email":      createReq.Email,
					"identifier": identifier,
					"error":      err.Error(),
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

			if createReq.Password != "" {
				// #2366：identifier = 实际登录名；email 保留兼容旧前端。
				res.InitialPasswords = append(res.InitialPasswords, gin.H{
					"identifier": identifier,
					"email":      createReq.Email,
					"password":   createReq.Password,
				})
			}

			localUser := models.User{
				ID:       userResult.UserID,
				IAMSub:   userResult.UserID,
				TenantID: tenantID,
				OrgID:    site.OrgID,
				Name:     createReq.Name,
				Email:    createReq.Email,
				Phone:    createReq.Phone,
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

	// #2114 P1 门控辅助：判定「是否本商户成员」以 IAM 关系为权威（计划 §2.2；本地表仅展示缓存）。
	// 祖先链（C→…→B，排除平台根 is_primary）整链只解析一次，避免循环内重复 IAM 调用。
	var ancestorOrgs map[string]bool
	var ancestorErr error
	loadAncestors := func() (map[string]bool, error) {
		if ancestorOrgs == nil && ancestorErr == nil {
			ancestorOrgs, ancestorErr = siteAncestorOrgs(iamClient, site.OrgID)
		}
		return ancestorOrgs, ancestorErr
	}
	p1IsTenantMember := func(userID string) (bool, error) {
		userOrgs, err := iamClient.GetUserOrgIDs(resolveIAMSub(db, userID))
		if err != nil {
			return false, err
		}
		ancestors, err := loadAncestors()
		if err != nil {
			return false, err
		}
		for _, o := range userOrgs {
			if ancestors[o] {
				return true, nil
			}
		}
		return false, nil
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

		// #2114 P1：网点级角色只能直接添加「已是本商户成员」的用户（P2/P3 → 申请）。
		// 判定以 IAM B 关系 active 为权威（计划 §2.2；本地表仅展示缓存）。
		// IAM 校验失败 → 显式 500（不静默吞错、不静默回退本地缓存）。
		if !adminLevel {
			isMember, err := p1IsTenantMember(userID)
			if err != nil {
				log.Printf("[AddMember] IAM membership check failed for user %s: %v", userID, err)
				return res, &addMemberFail{http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "无法校验成员关系，请重试",
				}}
			}
			if !isMember {
				return res, needApplyFail()
			}
		}

		// #2428 D3：跨租户禁止 direct bind；返回 40911 引导邀请。
		var targetUser models.User
		if err := db.Where("id = ? AND deleted_at IS NULL", userID).First(&targetUser).Error; err == nil && targetUser.TenantID != tenantID {
			return res, &addMemberFail{http.StatusConflict, gin.H{
				"code":    40911,
				"message": "该用户属于其他商户，请发送加入邀请，由本人确认加入",
				"data":    gin.H{"need_invite": true},
			}}
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
