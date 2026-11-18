package handlers

// #2386（#2384 前置）：全体成员聚合视图（只读）。
//
// 数据来源：merchant_members（直属/负责人）、site_members（网点，roles jsonb）、
// users.role；去重键 = user_id。权限：
//   - 商户管理员：直属 + 本商户全部网点；
//   - 网点管理员/员工：`scope=all` 自动降级为本人所在网点；
//   - 系统/平台管理员：只读（本接口无任何写动作）；
//   - 跨商户不可见。
//
// 本接口不改变既有 GET /api/users（ListStaff）行为（#2372/#2373 依赖）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type StaffMembersHandler struct{}

func NewStaffMembersHandler() *StaffMembersHandler { return &StaffMembersHandler{} }

type staffMemberSiteRef struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type staffMemberRow struct {
	UserID       string               `json:"user_id"`
	Username     string               `json:"username"`
	Name         string               `json:"name"`
	Phone        string               `json:"phone"`
	Email        string               `json:"email"`
	Status       string               `json:"status"`
	Role         string               `json:"role"`
	MerchantRole string               `json:"merchant_role"`
	Identities   []string             `json:"identities"`
	Sites        []staffMemberSiteRef `json:"sites"`
	IsDirect     bool                 `json:"is_direct"`
	JoinedAt     time.Time            `json:"joined_at"`
}

type staffMemberBaseRow struct {
	ID        string    `gorm:"column:id"`
	Username  string    `gorm:"column:username"`
	Name      string    `gorm:"column:name"`
	Phone     string    `gorm:"column:phone"`
	Email     string    `gorm:"column:email"`
	Status    string    `gorm:"column:status"`
	Role      string    `gorm:"column:role"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type staffMemberDirectRow struct {
	UserID    string    `gorm:"column:user_id"`
	Role      string    `gorm:"column:role"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type staffMemberSiteRow struct {
	UserID    string    `gorm:"column:user_id"`
	SiteID    string    `gorm:"column:site_id"`
	SiteName  string    `gorm:"column:site_name"`
	Role      string    `gorm:"column:role"`
	Roles     string    `gorm:"column:roles"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func identityRoles(filter string) []string {
	switch filter {
	case "admin":
		return []string{"site_admin", "merchant_admin", "admin", "ADMIN"}
	case "staff":
		return []string{"site_member", "merchant_staff", "staff", "STAFF", "WORKER"}
	case "technician":
		return []string{"repair_technician"}
	default:
		return nil
	}
}

// List handles GET /api/staff/members (read-only aggregate view).
func (h *StaffMembersHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	role := middleware.GetBusinessRole(ctx)
	tid := middleware.GetTenantID(ctx)
	oid := middleware.GetOrgID(ctx)
	callerID := middleware.GetUserID(ctx)

	// #2386 F1：Issue 契约默认 direct（只列本商户直属）。
	scope := c.DefaultQuery("scope", "direct")
	if scope != "all" && scope != "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "scope must be all|direct"})
		return
	}
	identity := c.Query("identity")
	if identity != "" && len(identityRoles(identity)) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "identity must be admin|staff|technician"})
		return
	}
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)

	// 权限范围
	var anchors []string
	var scopedSiteIDs []string
	switch role {
	case middleware.BusinessRoleMerchantAdmin:
		anchor := tid
		if anchor == "" {
			anchor = oid
		}
		anchors = append(anchors, anchor)
		if m, ok := resolveOperatorMerchant2247(merchantConfigDB(ctx), oid, tid); ok && m.OrgID != "" && m.OrgID != anchor {
			anchors = append(anchors, m.OrgID)
		}
	case middleware.BusinessRoleSiteAdmin, middleware.BusinessRoleSiteMember:
		if callerID == "" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "no caller identity"})
			return
		}
		db.Table("site_members").Where("user_id = ?", callerID).Pluck("site_id", &scopedSiteIDs)
		if len(scopedSiteIDs) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "你不在任何网点中"})
			return
		}
	case middleware.BusinessRoleSystemAdmin, middleware.BusinessRolePlatformStaff:
		// 只读、全量可见（A2：跨商户明细需显式 merchant_id，本期不做筛选）
	default:
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "仅管理端可查看成员视图"})
		return
	}

	// #2386 F4：直属 JOIN 必须按邀请商户锚定 tenant，避免「B 商户直属」混入 A 的 direct。
	mmJoin := "LEFT JOIN merchant_members mm ON mm.user_id = users.id"
	joinArgs := []interface{}{}
	if len(anchors) > 0 {
		mmJoin += " AND mm.tenant_id IN ?"
		joinArgs = append(joinArgs, anchors)
	}
	q := db.Table("users").
		Joins(mmJoin, joinArgs...).
		Joins("LEFT JOIN site_members sm ON sm.user_id = users.id")
	switch role {
	case middleware.BusinessRoleMerchantAdmin:
		q = q.Where("(mm.tenant_id IN ? OR sm.tenant_id IN ?)", anchors, anchors)
	case middleware.BusinessRoleSiteAdmin, middleware.BusinessRoleSiteMember:
		q = q.Where("sm.site_id IN ?", scopedSiteIDs)
	}
	if scope == "direct" {
		q = q.Where("mm.user_id IS NOT NULL")
	} else {
		q = q.Where("(mm.user_id IS NOT NULL OR sm.user_id IS NOT NULL)")
	}
	if siteID := strings.TrimSpace(c.Query("site_id")); siteID != "" {
		q = q.Where("sm.site_id = ?", siteID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("users.status = ?", status)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("(users.username ILIKE ? OR users.name ILIKE ? OR users.email ILIKE ? OR users.phone ILIKE ?)", like, like, like, like)
	}
	if roles := identityRoles(identity); len(roles) > 0 {
		// #2386 F3：site_members.roles 是 jsonb 多角色，必须参与筛选（与 identities 展示一致）。
		// 用 jsonb_exists_any（?| 的函数形式）避免 GORM 把操作符里的 ? 当成占位符。
		q = q.Where("(mm.role IN ? OR sm.role IN ? OR users.role IN ? OR jsonb_exists_any(sm.roles, ?::text[]))",
			roles, roles, roles, pq.Array(roles))
	}

	var total int64
	if err := q.Distinct("users.id").Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to count members"})
		return
	}

	var base []staffMemberBaseRow
	if err := q.Distinct().
		Select("users.id, users.username, users.name, users.phone, users.email, users.status, users.role, users.created_at").
		Order("users.created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&base).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list members"})
		return
	}
	ids := make([]string, 0, len(base))
	for _, u := range base {
		ids = append(ids, u.ID)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "success", "data": gin.H{"list": []staffMemberRow{}, "total": total, "page": page, "page_size": pageSize}})
		return
	}

	// 批量取关系（两条查询 + JOIN 站点，无逐用户查询）
	var directs []staffMemberDirectRow
	dq := db.Table("merchant_members").Select("user_id, role, created_at").Where("user_id IN ?", ids)
	if len(anchors) > 0 {
		dq = dq.Where("tenant_id IN ?", anchors)
	}
	if err := dq.Scan(&directs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to load direct memberships"})
		return
	}

	var siteRows []staffMemberSiteRow
	sq := db.Table("site_members sm").
		Select("sm.user_id, sm.site_id, sm.role, sm.roles, sm.created_at, s.name AS site_name").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id IN ?", ids)
	if len(anchors) > 0 {
		sq = sq.Where("sm.tenant_id IN ?", anchors)
	}
	if len(scopedSiteIDs) > 0 {
		sq = sq.Where("sm.site_id IN ?", scopedSiteIDs)
	}
	if siteID := strings.TrimSpace(c.Query("site_id")); siteID != "" {
		sq = sq.Where("sm.site_id = ?", siteID)
	}
	if err := sq.Scan(&siteRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to load site memberships"})
		return
	}

	directByUser := map[string]staffMemberDirectRow{}
	for _, d := range directs {
		directByUser[d.UserID] = d
	}
	sitesByUser := map[string][]staffMemberSiteRow{}
	for _, s := range siteRows {
		sitesByUser[s.UserID] = append(sitesByUser[s.UserID], s)
	}

	list := make([]staffMemberRow, 0, len(base))
	for _, u := range base {
		row := staffMemberRow{
			UserID: u.ID, Username: u.Username, Name: u.Name, Phone: u.Phone,
			Email: u.Email, Status: u.Status, Role: u.Role, JoinedAt: u.CreatedAt,
		}
		seenIdentity := map[string]bool{}
		addIdentity := func(v string) {
			v = strings.TrimSpace(v)
			if v == "" || seenIdentity[v] {
				return
			}
			seenIdentity[v] = true
			row.Identities = append(row.Identities, v)
		}
		if d, ok := directByUser[u.ID]; ok {
			row.IsDirect = true
			row.MerchantRole = d.Role
			addIdentity(d.Role)
			if !d.CreatedAt.IsZero() && d.CreatedAt.Before(row.JoinedAt) {
				row.JoinedAt = d.CreatedAt
			}
		}
		siteSeen := map[string]int{}
		for _, s := range sitesByUser[u.ID] {
			roles := parseSiteMemberRoles(s.Roles)
			if len(roles) == 0 && s.Role != "" {
				roles = []string{s.Role}
			}
			for _, r := range roles {
				addIdentity(r)
			}
			if idx, ok := siteSeen[s.SiteID]; ok {
				row.Sites[idx].Roles = mergeStrings(row.Sites[idx].Roles, roles)
			} else {
				siteSeen[s.SiteID] = len(row.Sites)
				row.Sites = append(row.Sites, staffMemberSiteRef{ID: s.SiteID, Name: s.SiteName, Roles: roles})
			}
			if !s.CreatedAt.IsZero() && s.CreatedAt.Before(row.JoinedAt) {
				row.JoinedAt = s.CreatedAt
			}
		}
		addIdentity(u.Role)
		list = append(list, row)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "success", "data": gin.H{
		"list": list, "total": total, "page": page, "page_size": pageSize,
	}})
}

func parseSiteMemberRoles(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var roles []string
	if err := json.Unmarshal([]byte(raw), &roles); err != nil {
		return nil
	}
	return roles
}

func mergeStrings(base, extra []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range append(append([]string{}, base...), extra...) {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
