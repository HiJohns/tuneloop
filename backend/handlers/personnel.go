package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// #2065 人员管理三视图（角色感知）：
//   - merchant_admin：本商户全员 = 各网点成员 ∪ 维修师傅（直属商户，不挂网点）
//   - site_admin：仅本网点成员（不含维修师傅）
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

	var rows []personnelRow
	switch businessRole {
	case middleware.BusinessRoleMerchantAdmin:
		if tid == "" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
			return
		}
		rows = personnelMerchantView(db, tid, keyword, siteFilter)
	case middleware.BusinessRoleSiteAdmin:
		if oid == "" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
			return
		}
		rows = personnelSiteView(db, oid, keyword)
	case middleware.BusinessRoleSystemAdmin:
		if oid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "operator has no organization"})
			return
		}
		rows = personnelSystemView(db, oid, keyword)
	default:
		c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "access denied"})
		return
	}

	// 分页（页面既有 page/page_size 契约；行数有限，内存切片即可）
	total := len(rows)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
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
func personnelMerchantView(db *gorm.DB, tid, keyword, siteFilter string) []personnelRow {
	var users []models.User
	q := db.Where("tenant_id = ? AND deleted_at IS NULL", tid)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil
	}
	var sms []siteMembershipRow
	smq := db.Table("site_members AS sm").
		Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("s.tenant_id = ? AND sm.status = 'active'", tid)
	if siteFilter != "" {
		smq = smq.Where("sm.site_id = ?", siteFilter)
	}
	smq.Scan(&sms)
	// #2065 审计 H1：指定网点时仅列该网点成员（师傅直属商户、不属任何网点 → 排除）
	var techIDs []string
	if siteFilter == "" {
		db.Model(&models.TechnicianProfile{}).Where("tenant_id = ?", tid).Pluck("user_id", &techIDs)
	}
	return assemblePersonnel(users, sms, techIDs)
}

// personnelSiteView 仅本网点成员（不含维修师傅）。
func personnelSiteView(db *gorm.DB, siteID, keyword string) []personnelRow {
	var sms []siteMembershipRow
	db.Table("site_members AS sm").
		Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.site_id = ? AND sm.status = 'active'", siteID).
		Scan(&sms)

	ids := make([]string, 0, len(sms))
	for _, m := range sms {
		ids = append(ids, m.UserID)
	}
	if len(ids) == 0 {
		return nil
	}
	var users []models.User
	q := db.Where("id IN ? AND deleted_at IS NULL", ids)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR phone ILIKE ? OR email ILIKE ?", like, like, like)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil
	}
	return assemblePersonnel(users, sms, nil)
}

// personnelSystemView 平台员工（根组织 users，与 platform_staff.go 同源）∪ 中转网点成员。
// 注意：平台员工通常无 site_members 记录，不能走 assemblePersonnel 的「无归属即过滤」逻辑。
func personnelSystemView(db *gorm.DB, rootOrgID, keyword string) []personnelRow {
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
	if err := q.Find(&staff).Error; err != nil {
		return nil
	}
	for _, u := range staff {
		seen[u.ID] = len(rows)
		rows = append(rows, personnelRow{
			ID: u.ID, UserID: u.ID, Name: u.Name, Phone: u.Phone, Email: u.Email,
			Position: u.Position, Role: u.Role, Status: u.Status, IAMSub: u.IAMSub,
			SiteName: "平台",
		})
	}

	// 2) 中转网点成员（sites.type='transit'，归属根组织）
	var sms []siteMembershipRow
	db.Table("site_members AS sm").
		Select("sm.user_id, sm.site_id, s.name AS site_name, sm.role").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("s.type = ? AND (s.tenant_id = ? OR s.org_id = ?) AND sm.status = 'active'", "transit", rootOrgID, rootOrgID).
		Scan(&sms)
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
		if err := mq.Find(&members).Error; err != nil {
			return nil
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
				SiteName: strings.Join(siteNames, ", "),
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
	return rows
}

// assemblePersonnel 将用户 × 网点归属 × 师傅身份组装为一行一人（聚合网点名）。
func assemblePersonnel(users []models.User, sms []siteMembershipRow, techIDs []string) []personnelRow {
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
		if len(memberships) == 0 && !isTech {
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
		})
	}
	return rows
}
