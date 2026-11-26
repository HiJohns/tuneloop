package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// #2065 人员管理三视图（角色感知）：
//   - merchant_admin：本商户全员 = 各网点成员 ∪ 维修师傅（直属商户，不挂网点）
//   - site_admin：本网点（**组织域**：JWT `oid` = `sites.org_id`）成员，不含维修师傅
//   - system_admin：平台员工（根组织 users）∪ 中转网点成员
//
// 字段：姓名/电话/所属网点/职位（+ email/status/iam_sub 供既有行操作复用）。
type PersonnelHandler struct{}

func NewPersonnelHandler() *PersonnelHandler { return &PersonnelHandler{} }

type personnelRow struct {
	ID           string `json:"id"` // = UserID（兼容既有行操作对 record.id 的依赖）
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Position     string `json:"position"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	IAMSub       string `json:"iam_sub"`
	SiteID       string `json:"site_id"`
	SiteName     string `json:"site_name"`
	IsTechnician bool   `json:"is_technician"`
	Bio          string `json:"bio"`    // #2073: 全员富文本简介
	Avatar       string `json:"avatar"` // #2073: 头像（users.avatar_url）
	// #2526: 本地归属（users.tenant_id / users.org_id）缺失（空或 nil UUID）时为 true，
	// 该行按 membership 展示但归属未回填，PC 列表须显示「归属缺失」标记。
	// 由 assemblePersonnel（商户/网点视图）与 personnelSystemView（系统视图）同源填充。
	AttributionMissing bool `json:"attribution_missing"`
}

// List GET /api/admin/personnel
func (h *PersonnelHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	businessRole := middleware.GetBusinessRole(ctx)
	tid := middleware.GetTenantID(ctx)
	oid := middleware.GetOrgID(ctx)
	// #2065 审计 H1：页面既有搜索表单发送 name/site_id（旧 /staff 契约），必须消费；
	// search 保留作通用口径（name 优先）。
	keyword := strings.TrimSpace(c.Query("name"))
	if keyword == "" {
		keyword = strings.TrimSpace(c.Query("search"))
	}
	siteFilter := strings.TrimSpace(c.Query("site_id"))
	// #2427 D3：site_id 非法（如 "null"/"abc"）必须 40002 + WARN，
	// 不得把非法值直接带进 SQL（PG 22P02）后静默返回 0 行。
	if siteFilter != "" {
		if _, err := uuid.Parse(siteFilter); err != nil {
			log.Printf("[Personnel] WARN: invalid site_id=%q from user=%s: %v", siteFilter, middleware.GetUserID(ctx), err)
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid site_id: must be a UUID"})
			return
		}
	}
	directOnly := c.Query("direct") == "true" // #2067: 只看直属员工（商户直属+师傅）

	var rows []personnelRow
	var err error
	switch businessRole {
	case middleware.BusinessRoleMerchantAdmin:
		if tid == "" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
			return
		}
		rows, err = personnelMerchantView(db, tid, keyword, siteFilter, directOnly)
	case middleware.BusinessRoleSiteAdmin:
		if oid == "" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
			return
		}
		// #2508: oid 是组织 ID（sites.org_id），与 site_id（sites.id）不同域；
		// site_id 过滤与本组织求交，必须在视图内消费（D2）。
		rows, err = personnelSiteView(db, oid, keyword, siteFilter)
	case middleware.BusinessRoleSystemAdmin:
		if oid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "operator has no organization"})
			return
		}
		rows, err = personnelSystemView(db, oid, keyword)
	default:
		c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
		return
	}
	// #2427 D3：视图查询失败必须显式返回，不得静默当作「无数据」（空列表会把故障伪装成 0 条）。
	if err != nil {
		log.Printf("[Personnel] ERROR: personnel list query failed (role=%s, tid=%s, oid=%s, site_id=%q): %v",
			businessRole, tid, oid, siteFilter, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query personnel: " + err.Error()})
		return
	}

	// 分页（页面既有 page/page_size 契约；行数有限，内存切片即可）
	total := len(rows)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	pageSize = clampPageSize(pageSize, 20, maxPageSize)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": rows[start:end], "total": total}})
}

type siteMembershipRow struct {
	UserID   string
	SiteID   string
	SiteName string
	Role     string
}

// personnelMerchantView 本商户全员：tenant 下所有网点成员 ∪ 维修师傅。
// personnelMerchantView 本商户全员。**membership 为权威**（#2067）：
// 归属由 site_members.sites.tenant_id / merchant_members.tenant_id / technician_profiles.tenant_id 决定，
// users 行按 id 取回（local users.tenant_id 不可靠，如商户管理员自身行为全零 uuid）。
func personnelMerchantView(db *gorm.DB, tid, keyword, siteFilter string, directOnly bool) ([]personnelRow, error) {
	// #2067 + #2261：成员归属已由 sites / merchant_members / technician_profiles
	// 的 tenant_id 显式过滤；本地 users.tenant_id 不可靠（零租户用户），
	// 必须关闭自动租户范围，否则 users 查询会把零租户成员漏掉。
	scopeCtx := db.Statement.Context
	if scopeCtx == nil {
		scopeCtx = context.Background()
	}
	db = db.WithContext(database.ExplicitTenantScopeOnly(scopeCtx))
	if directOnly {
		siteFilter = "" // #2067: direct 优先（页面可能同时带网点选择）
	}
	// 1) 网点成员（非 direct、且未指定网点/指定网点过滤）
	var sms []siteMembershipRow
	if !directOnly {
		smq := db.Table("site_members AS sm").
			Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
			Joins("JOIN sites s ON s.id = sm.site_id").
			Where("s.tenant_id = ? AND sm.status = 'active'", tid)
		if siteFilter != "" {
			smq = smq.Where("sm.site_id = ?", siteFilter)
		}
		// #2427 R3：Scan 错误必须返回（此前静默 → 0 行，故障伪装成「无数据」）。
		if err := smq.Scan(&sms).Error; err != nil {
			return nil, fmt.Errorf("site_members scan failed: %w", err)
		}
	}
	// 2) 直属员工（merchant_members）；指定网点时不列直属
	mmRoles := map[string]string{}
	if siteFilter == "" {
		var mms []struct {
			UserID string
			Role   string
		}
		if err := db.Table("merchant_members").Select("user_id, role").
			Where("tenant_id = ? AND status = 'active'", tid).Scan(&mms).Error; err != nil {
			return nil, fmt.Errorf("merchant_members scan failed: %w", err)
		}
		for _, m := range mms {
			mmRoles[m.UserID] = m.Role
		}
	}
	// 3) 维修师傅（直属商户）；指定网点时排除（不属网点）
	var techIDs []string
	if siteFilter == "" {
		if err := db.Model(&models.TechnicianProfile{}).Where("tenant_id = ?", tid).
			Pluck("user_id", &techIDs).Error; err != nil {
			return nil, fmt.Errorf("technician_profiles query failed: %w", err)
		}
	}

	// 4) 汇总 user ids（membership 权威）→ 按 id 取 users
	idSet := map[string]bool{}
	for _, m := range sms {
		idSet[m.UserID] = true
	}
	for id := range mmRoles {
		idSet[id] = true
	}
	for _, id := range techIDs {
		idSet[id] = true
	}
	if len(idSet) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	var users []models.User
	q := db.Where("id IN ? AND deleted_at IS NULL", ids)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
	}
	// #2072: 新建置顶（此前无 ORDER BY，行序不稳定致新建用户不在当前页）
	q = q.Order("created_at DESC")
	if err := q.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("users query failed: %w", err)
	}
	return assemblePersonnel(users, sms, techIDs, mmRoles), nil
}

// personnelSiteView 仅本网点成员（不含维修师傅）。
//
// #2508：JWT `oid` 是**组织 ID**（`sites.org_id`），而 `site_members.site_id`
// 存的是 `sites.id`——两者不同域。必须经 `sites` 关联按组织过滤；直接把 `oid`
// 当 `site_id` 比较会让 site_admin 的人员列表恒为空（静默 0 条，无报错）。
func personnelSiteView(db *gorm.DB, orgID, keyword, siteFilter string) ([]personnelRow, error) {
	// #2067 / D4：membership 为权威（users.tenant_id 不可靠，如零租户成员），
	// 隔离由 `sites.org_id = oid` 与成员关系承担，故显式关闭自动租户范围。
	scopeCtx := db.Statement.Context
	if scopeCtx == nil {
		scopeCtx = context.Background()
	}
	db = db.WithContext(database.ExplicitTenantScopeOnly(scopeCtx))

	var sms []siteMembershipRow
	smq := db.Table("site_members AS sm").
		Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("s.org_id = ? AND sm.status = 'active'", orgID)
	// D2：site_id 与本组织求交——跨组织站点 ID 得到空集（不报错、不泄漏）。
	if siteFilter != "" {
		smq = smq.Where("sm.site_id = ?", siteFilter)
	}
	if err := smq.Scan(&sms).Error; err != nil {
		return nil, fmt.Errorf("site_members scan failed (org=%s): %w", orgID, err)
	}
	if len(sms) == 0 {
		// #2427 D3 / #2508 B5：「恒空」必须留可观测痕迹，否则 ID 域/配置问题
		// 会再次以「空列表、无报错」的形态逃过巡检。
		log.Printf("[Personnel] WARN: site view resolved 0 active members for org=%s (site_id=%q, keyword=%q)",
			orgID, siteFilter, keyword)
	}

	ids := make([]string, 0, len(sms))
	for _, m := range sms {
		ids = append(ids, m.UserID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var users []models.User
	q := db.Where("id IN ? AND deleted_at IS NULL", ids)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
	}
	// #2072: 新建置顶
	q = q.Order("created_at DESC")
	if err := q.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("users query failed: %w", err)
	}
	return assemblePersonnel(users, sms, nil, nil), nil
}

// personnelSystemView 平台员工（根组织 users，与 platform_staff.go 同源）∪ 中转网点成员。
// 注意：平台员工通常无 site_members 记录，不能走 assemblePersonnel 的「无归属即过滤」逻辑。
func personnelSystemView(db *gorm.DB, rootOrgID, keyword string) ([]personnelRow, error) {
	rows := []personnelRow{}
	seen := map[string]int{}

	// 1) 平台员工（无网点归属 → site_name=平台）
	var staff []models.User
	q := db.Where("org_id = ? AND LOWER(role) IN ? AND deleted_at IS NULL",
		rootOrgID, []string{"staff", "namespace_admin", "sys_admin"})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
	}
	q = q.Order("created_at DESC") // #2072: 新建置顶
	if err := q.Find(&staff).Error; err != nil {
		return nil, fmt.Errorf("platform staff query failed: %w", err)
	}
	for _, u := range staff {
		seen[u.ID] = len(rows)
		rows = append(rows, personnelRow{
			ID: u.ID, UserID: u.ID, Name: u.Name, Phone: u.Phone, Email: u.Email,
			Position: u.Position, Role: u.Role, Status: u.Status, IAMSub: u.IAMSub,
			SiteName: "平台", Bio: u.Bio, Avatar: u.AvatarURL,
			AttributionMissing: isNilAttribution(u.TenantID) || isNilAttribution(u.OrgID),
		})
	}

	// 2) 中转网点成员（sites.type='transit'，归属根组织）
	var sms []siteMembershipRow
	if err := db.Table("site_members AS sm").
		Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("s.type = ? AND (s.tenant_id = ? OR s.org_id = ?) AND sm.status = 'active'", "transit", rootOrgID, rootOrgID).
		Scan(&sms).Error; err != nil {
		return nil, fmt.Errorf("transit site_members scan failed: %w", err)
	}
	ids := make([]string, 0, len(sms))
	for _, m := range sms {
		ids = append(ids, m.UserID)
	}
	if len(ids) > 0 {
		var members []models.User
		mq := db.Where("id IN ? AND deleted_at IS NULL", ids)
		if keyword != "" {
			like := "%" + keyword + "%"
			mq = mq.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
		}
		mq = mq.Order("created_at DESC") // #2072: 新建置顶
		if err := mq.Find(&members).Error; err != nil {
			return nil, fmt.Errorf("transit members query failed: %w", err)
		}
		byUser := map[string][]siteMembershipRow{}
		for _, m := range sms {
			byUser[m.UserID] = append(byUser[m.UserID], m)
		}
		for _, u := range members {
			siteNames := []string{}
			seenSite := map[string]bool{}
			roleNames := []string{}
			for _, m := range byUser[u.ID] {
				if m.SiteName != "" && !seenSite[m.SiteName] {
					siteNames = append(siteNames, m.SiteName)
					seenSite[m.SiteName] = true
				}
				if m.Role != "" {
					roleNames = append(roleNames, m.Role)
				}
			}
			row := personnelRow{
				ID: u.ID, UserID: u.ID, Name: u.Name, Phone: u.Phone, Email: u.Email,
				Position: u.Position, Role: u.Role, Status: u.Status, IAMSub: u.IAMSub,
				SiteName: strings.Join(siteNames, ", "), Bio: u.Bio, Avatar: u.AvatarURL,
				AttributionMissing: isNilAttribution(u.TenantID) || isNilAttribution(u.OrgID),
			}
			if len(roleNames) > 0 {
				row.Role = strings.Join(roleNames, ",")
			}
			if idx, ok := seen[u.ID]; ok {
				rows[idx] = row // 同时是平台员工 → 以中转网点归属为准
				continue
			}
			seen[u.ID] = len(rows)
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// assemblePersonnel 将用户 × 网点归属 × 师傅身份组装为一行一人（聚合网点名）。
func assemblePersonnel(users []models.User, sms []siteMembershipRow, techIDs []string, mmRoles map[string]string) []personnelRow {
	byUser := map[string][]siteMembershipRow{}
	for _, m := range sms {
		byUser[m.UserID] = append(byUser[m.UserID], m)
	}
	techSet := map[string]bool{}
	for _, id := range techIDs {
		techSet[id] = true
	}

	rows := make([]personnelRow, 0, len(users))
	for _, u := range users {
		memberships := byUser[u.ID]
		isTech := techSet[u.ID]
		mmRole, isDirect := "", false
		if mmRoles != nil {
			mmRole, isDirect = mmRoles[u.ID]
		}
		if len(memberships) == 0 && !isTech && !isDirect {
			continue // 非员工（纯顾客）不进入人员管理视图
		}
		siteNames := make([]string, 0, len(memberships))
		siteIDs := make([]string, 0, len(memberships))
		roles := make([]string, 0, len(memberships))
		seenSite := map[string]bool{}
		for _, m := range memberships {
			if m.SiteName != "" && !seenSite[m.SiteName] {
				siteNames = append(siteNames, m.SiteName)
				seenSite[m.SiteName] = true
			}
			if len(siteIDs) == 0 {
				siteIDs = append(siteIDs, m.SiteID)
			}
			if m.Role != "" {
				roles = append(roles, m.Role)
			}
		}
		siteName := strings.Join(siteNames, ", ")
		position := strings.TrimSpace(u.Position)
		if isTech {
			if siteName == "" {
				siteName = "直属商户"
			}
			if position == "" {
				position = "维修师傅"
			}
		} else if isDirect {
			// #2067: 商户直属员工（merchant_members，无网点）
			if siteName == "" {
				siteName = "直属商户"
			}
			if position == "" {
				position = mmRole
			}
		}
		role := u.Role
		if len(roles) > 0 {
			role = strings.Join(roles, ",")
		}
		siteID := ""
		if len(siteIDs) > 0 {
			siteID = siteIDs[0]
		}
		rows = append(rows, personnelRow{
			ID: u.ID, UserID: u.ID, Name: u.Name, Phone: u.Phone, Email: u.Email,
			Position: position, Role: role, Status: u.Status, IAMSub: u.IAMSub,
			SiteID: siteID, SiteName: siteName, IsTechnician: isTech,
			Bio: u.Bio, Avatar: u.AvatarURL,
			AttributionMissing: isNilAttribution(u.TenantID) || isNilAttribution(u.OrgID),
		})
	}
	return rows
}

// #2526：员工 by-id 动作（详情/保存/激活）的可见性判定。
//
// 背景：人员列表三视图以 membership 为权威（#2065/#2067），但 by-id 动作曾按
// `users.tenant_id = ?` 过滤 → 零租户（nil UUID）存量行「列表可见、点开 404」。
// 这里把判定收口为唯一实现，列表与详情不再各写一套：
//
//	可见 = membership 口径 ∪（旧 tenant/org 口径 ∧ 非顾客角色）
//
// 纯顾客（`lower(role) ∈ {"", "user"}`）不再因 `users.tenant_id` 命中被当员工打开
// （数据保护 B10）；不可见与不存在对调用方统一映射为 404，不泄漏存在性。

// nilUUIDLiteral 是 PostgreSQL/SQL 里表示「无归属」的 nil UUID。
const nilUUIDLiteral = "00000000-0000-0000-0000-000000000000"

// staffUserLookup 区分「不存在」与「存在但不可见」，两者对外都是 404。
type staffUserLookup int

const (
	staffUserFound staffUserLookup = iota
	staffUserMissing
	staffUserInvisible
)

// isNilAttribution 报告字符串形式的归属 ID 是否为空/nil UUID。
func isNilAttribution(v string) bool {
	t := strings.TrimSpace(v)
	return t == "" || t == nilUUIDLiteral
}

// isCustomerUserRole 报告 users.role 是否属于顾客（含空值，空值不可当员工）。
func isCustomerUserRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "user":
		return true
	default:
		return false
	}
}

// loadVisibleStaffUser 取行（不按租户过滤）后再做可见性判定。
// 定位与授权分离：先按 id 取行，再交给 personnelUserVisible 判定能否访问（B3）。
func loadVisibleStaffUser(db *gorm.DB, ctx context.Context, userID string) (*models.User, staffUserLookup, error) {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, staffUserMissing, nil
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		// 非 UUID 在主键查询里必然无行（PG 会报 22P02，不能当 500 泄露实现细节）。
		return nil, staffUserMissing, nil
	}
	var user models.User
	q := db.WithContext(database.ExplicitTenantScopeOnly(ctx)).
		Where("id = ? AND deleted_at IS NULL", trimmed)
	if err := q.First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, staffUserMissing, nil
		}
		return nil, staffUserMissing, fmt.Errorf("query user %s: %w", trimmed, err)
	}
	visible, err := personnelUserVisible(db, ctx, &user)
	if err != nil {
		return nil, staffUserMissing, err
	}
	if !visible {
		return &user, staffUserInvisible, nil
	}
	return &user, staffUserFound, nil
}

// personnelUserVisible 是员工 by-id 动作的唯一可见性判定（列表口径 + 角色感知旧口径）。
func personnelUserVisible(db *gorm.DB, ctx context.Context, u *models.User) (bool, error) {
	if u == nil {
		return false, nil
	}
	businessRole := middleware.GetBusinessRole(ctx)
	tid := middleware.GetTenantID(ctx)
	oid := middleware.GetOrgID(ctx)
	customer := isCustomerUserRole(u.Role)

	switch businessRole {
	case middleware.BusinessRoleMerchantAdmin:
		if tid == "" {
			return false, nil
		}
		ok, err := hasMerchantMembership(db, ctx, u.ID, tid)
		if err != nil || ok {
			return ok, err
		}
		// 旧口径：同商户本地行，但只对员工角色生效（顾客不因 tenant 命中变员工）。
		return !customer && u.TenantID == tid, nil

	case middleware.BusinessRoleSiteAdmin:
		if oid == "" {
			return false, nil
		}
		ok, err := hasSiteMembershipByOrg(db, ctx, u.ID, oid)
		if err != nil || ok {
			return ok, err
		}
		// #2508：site_admin 的旧口径是「tenant 命中 + org 在可见组织内」，仍限员工角色。
		if customer {
			return false, nil
		}
		return legacyTenantOrgVisible(ctx, u), nil

	case middleware.BusinessRoleSystemAdmin:
		if oid == "" {
			return false, nil
		}
		// 平台员工（与 personnelSystemView 同源）
		switch strings.ToLower(strings.TrimSpace(u.Role)) {
		case "staff", "namespace_admin", "sys_admin":
			if u.OrgID == oid {
				return true, nil
			}
		}
		if ok, err := hasTransitSiteMembership(db, ctx, u.ID, oid); err != nil || ok {
			return ok, err
		}
		// 带租户 claim 的 NAMESPACE_ADMIN 等仍按旧口径放行，避免收窄既有访问。
		return tid != "" && u.TenantID == tid, nil

	default:
		return legacyTenantOrgVisible(ctx, u), nil
	}
}

// legacyTenantOrgVisible 复刻修复前的旧口径：`users.tenant_id = tid` + ApplyOrgScope。
func legacyTenantOrgVisible(ctx context.Context, u *models.User) bool {
	tid := middleware.GetTenantID(ctx)
	if tid == "" || u.TenantID != tid {
		return false
	}
	orgIDs, err := middleware.GetVisibleOrgIDs(ctx)
	if err != nil || orgIDs == nil {
		// ApplyOrgScope 在无法解析可见组织时退化为「不加 org 约束」。
		return true
	}
	for _, id := range orgIDs {
		if id == u.OrgID {
			return true
		}
	}
	return false
}

// hasMerchantMembership 报告用户是否以 active 成员身份归属本商户
// （网点成员 / 商户直属员工 / 维修师傅，与 personnelMerchantView 同源）。
func hasMerchantMembership(db *gorm.DB, ctx context.Context, userID, tenantID string) (bool, error) {
	scoped := db.WithContext(database.ExplicitTenantScopeOnly(ctx))
	var count int64
	if err := scoped.Table("site_members AS sm").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id = ? AND sm.status = 'active' AND s.tenant_id = ?", userID, tenantID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("site membership check failed (user=%s tenant=%s): %w", userID, tenantID, err)
	}
	if count > 0 {
		return true, nil
	}
	if err := scoped.Table("merchant_members").
		Where("user_id = ? AND tenant_id = ? AND status = 'active'", userID, tenantID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("merchant membership check failed (user=%s tenant=%s): %w", userID, tenantID, err)
	}
	if count > 0 {
		return true, nil
	}
	if err := scoped.Model(&models.TechnicianProfile{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("technician profile check failed (user=%s tenant=%s): %w", userID, tenantID, err)
	}
	return count > 0, nil
}

// hasSiteMembershipByOrg 报告用户是否以 active 成员身份归属该组织（#2508 口径：sites.org_id）。
func hasSiteMembershipByOrg(db *gorm.DB, ctx context.Context, userID, orgID string) (bool, error) {
	scoped := db.WithContext(database.ExplicitTenantScopeOnly(ctx))
	var count int64
	if err := scoped.Table("site_members AS sm").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id = ? AND sm.status = 'active' AND s.org_id = ?", userID, orgID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("site membership check failed (user=%s org=%s): %w", userID, orgID, err)
	}
	return count > 0, nil
}

// hasTransitSiteMembership 报告用户是否属于该组织下的中转网点（与 personnelSystemView 同源）。
func hasTransitSiteMembership(db *gorm.DB, ctx context.Context, userID, orgID string) (bool, error) {
	scoped := db.WithContext(database.ExplicitTenantScopeOnly(ctx))
	var count int64
	if err := scoped.Table("site_members AS sm").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id = ? AND sm.status = 'active' AND s.type = ? AND (s.tenant_id = ? OR s.org_id = ?)",
			userID, "transit", orgID, orgID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("transit membership check failed (user=%s org=%s): %w", userID, orgID, err)
	}
	return count > 0, nil
}
