package handlers

// #1938: 站点 / 中转网点共享的成员添加核心。
// 自 SiteMemberHandler.AddMember 提取，保持既有行为不变，供中转中心复用。
// 契约：user_id(+role) | user_ids:[{user_id,role}] |
//       new_users:[{username,name,email,phone,role,password,auto_generate,force_password_change}] + skip_activation。

import (
	"context"
	"encoding/json"
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
	"gorm.io/gorm/clause"
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

// mergeSiteMemberRoles 计算改主角色后的角色全集（#2482）。
//
// `site_members.roles` 是身份全集（主角色 + 其它身份，如「员工 + 维修师傅」），
// 而授权门禁（hasSiteRole）以 `roles` 为准。改主角色时若整体替换为单值，会在
// 本地抹掉其它身份，而 IAM 侧旧功能角色模板并不会被移除（无 unassign API），
// 造成「本地拒维修 / IAM 仍授权」的双源不一致。
//
// 语义：去空白 + 去重保序；保留其它角色；旧主角色**原位**替换为 newPrimary；
// 结果必含 newPrimary；newPrimary 为空时原样返回（防御，不制造空主角色）。
func mergeSiteMemberRoles(existing []string, oldPrimary, newPrimary string) []string {
	oldPrimary = strings.TrimSpace(oldPrimary)
	newPrimary = strings.TrimSpace(newPrimary)

	normalized := make([]string, 0, len(existing)+1)
	seen := make(map[string]struct{}, len(existing)+1)
	for _, role := range existing {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	if newPrimary == "" {
		return normalized
	}

	replaced := false
	for i, role := range normalized {
		if oldPrimary != "" && role == oldPrimary {
			normalized[i] = newPrimary
			replaced = true
			break
		}
	}
	if replaced {
		// 原位替换后可能产生重复（newPrimary 原本已在集合内）→ 去重保序。
		out := make([]string, 0, len(normalized))
		seen = make(map[string]struct{}, len(normalized))
		for _, role := range normalized {
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			out = append(out, role)
		}
		return out
	}
	if _, ok := seen[newPrimary]; ok {
		return normalized
	}
	return append(normalized, newPrimary)
}

// siteMemberRolesExpr 把角色集合序列化为 jsonb 表达式（#2475）。
//
// GORM 的 map Update 绕过 `serializer:json`，直接传 Go `[]string` 会被渲染成
// SQL 数组字面量 `('site_member')`，对 jsonb 列报 SQLSTATE 22P02。所有 map
// 写入 site_members.roles 的路径必须复用本 helper（与 site_member.go 的
// UpdateMemberRole 同口径），不得各自拼 SQL。
//
// 语义：去空白 + 去重保序；nil/空集合 → `[]`（roles 列 NOT NULL）。
func siteMemberRolesExpr(roles []string) (clause.Expr, error) {
	normalized := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return clause.Expr{}, fmt.Errorf("encode site_member roles: %w", err)
	}
	return gorm.Expr("?::jsonb", string(encoded)), nil
}

// ============================================================================
// #2505 S1：IAM 功能角色「集合语义」地基
//
// 根因：#2482 之后本地 `site_members.roles` 已经保留并存身份，但 IAM 侧仍只调用
// `AssignRoleTemplateToUserWithToken`（ADD-only），降级（site_admin→site_member）
// 后旧模板留在 `user_org_relations.functional_roles` 里 → 权限回收失效。
//
// 修复：GET current → 计算目标集合 → PUT 集合覆盖。目标集合 = 本 org 合并后的
// 受管角色 ∪ 新主角色，并**保留非受管模板**（customer / namespace_admin / 自定义），
// 避免集合覆盖误删顾客上下文（D2）。
// ============================================================================

// managedFunctionalRoleCodes 是 tuneloop 受管的功能角色模板码（D3）。
// 只有这些码会被集合覆盖「回收」；其它码（customer / namespace_admin / 自定义）
// 一律保留。
var managedFunctionalRoleCodes = []string{
	"site_admin",
	"site_member",
	"merchant_staff",
	"repair_technician",
	"merchant_admin",
}

// isManagedFunctionalRoleCode 判定模板码是否属于 tuneloop 受管集合。
func isManagedFunctionalRoleCode(code string) bool {
	code = strings.TrimSpace(code)
	for _, managed := range managedFunctionalRoleCodes {
		if code == managed {
			return true
		}
	}
	return false
}

// normalizeFunctionalRoleCodes 去空白 + 去重保序（空串丢弃）。
func normalizeFunctionalRoleCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

// functionalRoleTargetSet 计算 PUT functional-roles 的目标集合。
//
// 规则（D2）：
//  1. desired（新目标，如合并后的本地角色集）在前，去空白/去重保序；
//  2. 随后追加 current 中**非受管**且未出现过的码（customer / namespace_admin /
//     自定义模板无条件保留，不受模板列表影响）。
//
// 受管码只以 desired 为准 → 降级即回收；非受管码只增不减 → 不破坏顾客上下文。
func functionalRoleTargetSet(current, desired []string) []string {
	target := normalizeFunctionalRoleCodes(desired)
	seen := make(map[string]struct{}, len(target))
	for _, code := range target {
		seen[code] = struct{}{}
	}
	for _, code := range normalizeFunctionalRoleCodes(current) {
		if isManagedFunctionalRoleCode(code) {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		target = append(target, code)
	}
	return target
}

// functionalRoleTargetSetWithout 从 current 精确移除指定码（保序、去重）。
// S2（#2507）换管理员降级等路径复用；本单只提供纯函数与被测语义。
func functionalRoleTargetSetWithout(current []string, remove ...string) []string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, code := range remove {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		removeSet[code] = struct{}{}
	}
	out := make([]string, 0, len(current))
	for _, code := range normalizeFunctionalRoleCodes(current) {
		if _, drop := removeSet[code]; drop {
			continue
		}
		out = append(out, code)
	}
	return out
}

// functionalRoleSetEqual 忽略顺序比较两个角色集合。
func functionalRoleSetEqual(a, b []string) bool {
	as := normalizeFunctionalRoleCodes(a)
	bs := normalizeFunctionalRoleCodes(b)
	if len(as) != len(bs) {
		return false
	}
	seen := make(map[string]struct{}, len(as))
	for _, code := range as {
		seen[code] = struct{}{}
	}
	for _, code := range bs {
		if _, ok := seen[code]; !ok {
			return false
		}
	}
	return true
}

// functionalRoleErrorCode 为 role_errors 条目挑一个「标识码」：
// 目标集合首位（通常是新主角色），空集合时用固定占位，保证前端可展示。
func functionalRoleErrorCode(desired []string) string {
	if codes := normalizeFunctionalRoleCodes(desired); len(codes) > 0 {
		return codes[0]
	}
	return "functional_roles"
}

// syncFunctionalRoles 把 IAM 功能角色集合同步为 `desired`（集合覆盖）。
//
// 流程（D1/D4/D7/D10）：
//
//	a) GET current —— 失败则**不**发 PUT（无法保真非受管码），以 role_errors 暴露；
//	b) nsID 非空时拉模板列表并按 `is_active` 过滤**受管**失效码（剔除并记
//	   role_errors，避免把失效码发给对端触发整组 400 invalid_role）；
//	   非受管码不受模板列表影响；列表拉取失败/nsID 空 → 跳过过滤；
//	c) 目标集合与 current 集合相等（忽略顺序）→ 跳过 PUT；
//	d) PUT —— 失败以 role_errors 暴露。
//
// 返回值为 role_errors 条目（空切片 = 全部成功）。
func syncFunctionalRoles(ctx context.Context, iamClient *services.IAMClient, userToken, iamSub, orgID string, desired []string) []gin.H {
	roleErrors := make([]gin.H, 0)
	desired = normalizeFunctionalRoleCodes(desired)

	if iamClient == nil {
		return append(roleErrors, gin.H{"code": functionalRoleErrorCode(desired), "error": "sync functional roles: IAM client is nil"})
	}
	if strings.TrimSpace(iamSub) == "" {
		return append(roleErrors, gin.H{"code": functionalRoleErrorCode(desired), "error": "sync functional roles: user has no iam_sub"})
	}
	if strings.TrimSpace(orgID) == "" {
		return append(roleErrors, gin.H{"code": functionalRoleErrorCode(desired), "error": "sync functional roles: org_id is empty"})
	}

	current, err := iamClient.GetUserFunctionalRolesWithToken(userToken, iamSub, orgID)
	if err != nil {
		// D4：读不到当前集合就写 = 可能删掉并存/非受管角色 → 直接短路。
		log.Printf("[syncFunctionalRoles] read IAM functional roles failed user=%s org=%s: %v", iamSub, orgID, err)
		return append(roleErrors, gin.H{"code": functionalRoleErrorCode(desired), "error": "read IAM functional roles: " + err.Error()})
	}

	filteredCurrent := normalizeFunctionalRoleCodes(current)
	var activeTemplates map[string]struct{}
	if nsID := strings.TrimSpace(middleware.GetNamespaceID(ctx)); nsID != "" {
		if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
			activeTemplates = make(map[string]struct{}, len(templates))
			for _, tmpl := range templates {
				if tmpl.IsActive != nil && !*tmpl.IsActive {
					continue
				}
				activeTemplates[strings.TrimSpace(tmpl.Code)] = struct{}{}
			}
			kept := make([]string, 0, len(filteredCurrent))
			for _, code := range filteredCurrent {
				if !isManagedFunctionalRoleCode(code) {
					kept = append(kept, code)
					continue
				}
				if _, ok := activeTemplates[code]; !ok {
					log.Printf("[syncFunctionalRoles] dropping inactive managed role template user=%s org=%s code=%s", iamSub, orgID, code)
					roleErrors = append(roleErrors, gin.H{
						"code":  code,
						"error": "role template is inactive or unavailable: " + code,
					})
					continue
				}
				kept = append(kept, code)
			}
			filteredCurrent = kept
		} else {
			log.Printf("[syncFunctionalRoles] list role templates failed ns=%s (skip filtering): %v", nsID, err)
		}
	}

	target := functionalRoleTargetSet(filteredCurrent, desired)
	if activeTemplates != nil {
		// 目标侧的受管码同样不能被失效模板带进 PUT（beaconiam 会整组 400 invalid_role）。
		kept := make([]string, 0, len(target))
		for _, code := range target {
			if isManagedFunctionalRoleCode(code) {
				if _, ok := activeTemplates[code]; !ok {
					log.Printf("[syncFunctionalRoles] dropping inactive desired role template user=%s org=%s code=%s", iamSub, orgID, code)
					roleErrors = append(roleErrors, gin.H{
						"code":  code,
						"error": "role template is inactive or unavailable: " + code,
					})
					continue
				}
			}
			kept = append(kept, code)
		}
		target = kept
	}
	if functionalRoleSetEqual(target, current) {
		return roleErrors
	}

	if err := iamClient.SetUserFunctionalRolesWithToken(userToken, iamSub, orgID, target); err != nil {
		log.Printf("[syncFunctionalRoles] set IAM functional roles failed user=%s org=%s roles=%v: %v", iamSub, orgID, target, err)
		return append(roleErrors, gin.H{"code": functionalRoleErrorCode(desired), "error": "set IAM functional roles: " + err.Error()})
	}
	return roleErrors
}
