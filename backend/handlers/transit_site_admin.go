package handlers

// #1935: 平台级「中转网点」管理（Platform-direct transit sites）
// — 归属平台顶层组织（创建者上下文的租户），与商户 SiteManagement 租户作用域分离。
// — 成员管理复用 site_members，角色收敛为中转网点管理员/成员两类。
// — 被 TransitRoute 引用的中转网点禁止停用/删除（需先解绑路由）。

import (
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const transitSiteType = "transit"

// ListAdminTransitSites returns platform-owned transit sites with route usage counts.
// (#1935 audit Bug6: 停用站点不再被过滤，前端状态列可展示「停用」)
func ListAdminTransitSites(c *gin.Context) {
	db := database.GetDB()
	var sites []models.Site
	if err := db.Where("type = ?", transitSiteType).
		Order("created_at DESC").Find(&sites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list transit sites"})
		return
	}

	type item struct {
		models.Site
		RouteCount  int64 `json:"route_count"`
		MemberCount int64 `json:"member_count"`
	}
	list := make([]item, 0, len(sites))
	for _, s := range sites {
		var routeCount, memberCount int64
		db.Model(&models.TransitRoute{}).Where("transit_site_id = ?", s.ID).Count(&routeCount)
		db.Model(&models.SiteMember{}).Where("site_id = ?", s.ID).Count(&memberCount)
		list = append(list, item{Site: s, RouteCount: routeCount, MemberCount: memberCount})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list}})
}

// ListControlledSites returns candidate controlled sites for transit route
// creation (#1936 audit Bug1: no GET /api/sites endpoint existed, leaving the
// frontend route-creation dropdown permanently empty) — sites owned by
// controlled-type merchants, excluding transit-type sites.
func ListControlledSites(c *gin.Context) {
	db := database.GetDB()
	var sites []models.Site
	if err := db.
		Joins("JOIN merchants m ON m.tenant_id = sites.tenant_id AND m.merchant_type = ?", models.MerchantTypeControlled).
		Where("sites.type <> ? AND sites.status = ?", transitSiteType, "active").
		Order("sites.created_at DESC").
		Find(&sites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list controlled sites"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": sites}})
}

// CreateAdminTransitSite creates a platform-direct transit site.
func CreateAdminTransitSite(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Address     string `json:"address" binding:"required"`
		Phone       string `json:"phone" binding:"required"`
		ContactName string `json:"contact_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "name/address/phone 为必填"})
		return
	}

	db := database.GetDB()
	tenantID := middleware.GetTenantID(c.Request.Context())

	// #1935: 中转网点直辖平台顶层组织 —— OrgID 填平台租户 ID（顶层）
	// #1935 audit Bug4: contact_name 持久化（此前被静默丢弃）
	site := models.Site{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		OrgID:       tenantID,
		Name:        req.Name,
		Address:     req.Address,
		Phone:       req.Phone,
		ContactName: req.ContactName,
		Type:        transitSiteType,
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.Create(&site).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create transit site"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": site})
}

// UpdateAdminTransitSite edits a transit site (name/address/phone/contact).
func UpdateAdminTransitSite(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name        *string `json:"name"`
		Address     *string `json:"address"`
		Phone       *string `json:"phone"`
		ContactName *string `json:"contact_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request"})
		return
	}
	if (req.Address != nil && *req.Address == "") || (req.Phone != nil && *req.Phone == "") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "address/phone 不可为空"})
		return
	}

	db := database.GetDB()
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.ContactName != nil {
		updates["contact_name"] = *req.ContactName
	}
	if err := db.Model(&models.Site{}).Where("id = ? AND type = ?", id, transitSiteType).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update transit site"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

// DeleteAdminTransitSite deactivates a transit site — rejected when routes reference it.
func DeleteAdminTransitSite(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	var routeCount int64
	db.Model(&models.TransitRoute{}).Where("transit_site_id = ?", id).Count(&routeCount)
	if routeCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "该中转网点仍被路由引用，请先解除相关路由（#1935 完整性约束）",
		})
		return
	}
	if err := db.Model(&models.Site{}).Where("id = ? AND type = ?", id, transitSiteType).
		Update("status", "inactive").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to deactivate"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "deactivated"})
}

// ListTransitSiteMembers returns site members of a transit site (admin/member roles only).
func ListTransitSiteMembers(c *gin.Context) {
	siteID := c.Param("id")
	db := database.GetDB()
	// #1935 audit Bug6: 仅作用于中转类型站点
	var site models.Site
	if err := db.Where("id = ? AND type = ?", siteID, transitSiteType).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "transit site not found"})
		return
	}
	var members []models.SiteMember
	if err := db.Where("site_id = ?", siteID).Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list members"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": members}})
}

// assignTransitRoleTemplate best-effort assigns the role template that feeds
// JWT roles/sys_perm — audit #1935 Bug A（红线）：IAM 错误不得静默吞没，失败
// 必须透出到响应体（role_errors，与 site_member.go 同构），由调用方随响应返回。
func assignTransitRoleTemplate(c *gin.Context, iamClient *services.IAMClient, userToken, userID, orgID, templateCode string) []gin.H {
	roleErrors := []gin.H{}
	nsID := middleware.GetNamespaceID(c.Request.Context())
	templates, err := iamClient.ListRoleTemplates(nsID)
	if err != nil {
		log.Printf("[TransitSiteMember] ListRoleTemplates failed: %v", err)
		return []gin.H{{"error": "failed to list role templates: " + err.Error()}}
	}
	for _, t := range templates {
		if t.Code == templateCode {
			if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, userID, orgID, t.Code); err != nil {
				log.Printf("[TransitSiteMember] AssignRoleTemplate failed for user %s code %s: %v", userID, templateCode, err)
				roleErrors = append(roleErrors, gin.H{
					"user_id":       userID,
					"template_code": templateCode,
					"error":         err.Error(),
				})
			}
			break
		}
	}
	return roleErrors
}

// AddTransitSiteMember adds a member (role restricted to site_admin / site_member)
// with IAM org binding — #1935 audit Bug1: 本地 site_members 仅为缓存，账户/权限操作
// 必须以 IAM 为准（AGENTS §685）；IAM 绑定失败必须返回错误，不得静默写本地缓存。
func AddTransitSiteMember(c *gin.Context) {
	siteID := c.Param("id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Role   string `json:"role" binding:"required,oneof=site_admin site_member"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_id/role 为必填（role: site_admin|site_member）"})
		return
	}

	db := database.GetDB()
	var site models.Site
	if err := db.Where("id = ? AND type = ?", siteID, transitSiteType).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "transit site not found"})
		return
	}

	var count int64
	db.Model(&models.SiteMember{}).Where("site_id = ? AND user_id = ?", siteID, req.UserID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "该用户已是本中转网点成员"})
		return
	}

	// IAM bind first — local cache only after success
	var roleErrors []gin.H
	if site.OrgID != "" {
		iamClient := services.NewIAMClient()
		userToken := services.ExtractUserToken(c)
		operatorID := middleware.GetUserID(c.Request.Context())
		if err := iamClient.BindUserToOrganizationWithToken(userToken, req.UserID, site.OrgID, toIAMRole(req.Role), operatorID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50000, "message": "IAM 绑定失败: " + err.Error()})
			return
		}
		roleErrors = assignTransitRoleTemplate(c, iamClient, userToken, req.UserID, site.OrgID, req.Role)
	}

	member := models.SiteMember{
		ID:       uuid.New().String(),
		SiteID:   siteID,
		UserID:   req.UserID,
		TenantID: site.TenantID,
		Role:     req.Role,
	}
	if err := db.Create(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to add member"})
		return
	}
	resp := gin.H{"code": 20000, "data": member}
	if len(roleErrors) > 0 {
		// audit #1935 Bug A（红线）：IAM 角色模板失败必须透出，不得静默 200
		resp["role_errors"] = roleErrors
	}
	c.JSON(http.StatusOK, resp)
}

// UpdateTransitSiteMemberRole changes a member's role (#1935 audit Bug5:
// 计划「增删改查」缺「改」) — IAM role update first, then the local cache row.
func UpdateTransitSiteMemberRole(c *gin.Context) {
	siteID := c.Param("id")
	memberID := c.Param("member_id")
	var req struct {
		Role string `json:"role" binding:"required,oneof=site_admin site_member"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "role 为必填（site_admin|site_member）"})
		return
	}

	db := database.GetDB()
	var site models.Site
	if err := db.Where("id = ? AND type = ?", siteID, transitSiteType).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "transit site not found"})
		return
	}
	var member models.SiteMember
	if err := db.Where("id = ? AND site_id = ?", memberID, siteID).First(&member).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "member not found"})
		return
	}

	var roleErrors []gin.H
	if site.OrgID != "" && member.UserID != "" {
		iamClient := services.NewIAMClient()
		userToken := services.ExtractUserToken(c)
		if err := iamClient.UpdateUserRoleInOrgWithToken(userToken, site.OrgID, member.UserID, toIAMRole(req.Role)); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50000, "message": "IAM 角色更新失败: " + err.Error()})
			return
		}
		roleErrors = assignTransitRoleTemplate(c, iamClient, userToken, member.UserID, site.OrgID, req.Role)
	}

	if err := db.Model(&member).Update("role", req.Role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update member role"})
		return
	}
	member.Role = req.Role
	resp := gin.H{"code": 20000, "message": "updated", "data": member}
	if len(roleErrors) > 0 {
		// audit #1935 Bug A（红线）：IAM 角色模板失败必须透出，不得静默 200
		resp["role_errors"] = roleErrors
	}
	c.JSON(http.StatusOK, resp)
}

// RemoveTransitSiteMember removes a member from a transit site.
// #1935 audit Bug2: 删除条件必须限定站点归属（此前仅按 member_id 全表删除，
// 可越站删除任意网点成员）；unbind 失败按 site_member.go 同构策略 log 后继续。
func RemoveTransitSiteMember(c *gin.Context) {
	siteID := c.Param("id")
	memberID := c.Param("member_id")
	db := database.GetDB()

	var site models.Site
	if err := db.Where("id = ? AND type = ?", siteID, transitSiteType).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "transit site not found"})
		return
	}
	var member models.SiteMember
	if err := db.Where("id = ? AND site_id = ?", memberID, siteID).First(&member).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "member not found"})
		return
	}

	if site.OrgID != "" && member.UserID != "" {
		iamClient := services.NewIAMClient()
		memberToken := services.ExtractUserToken(c)
		operatorID := middleware.GetUserID(c.Request.Context())
		if err := iamClient.UnbindUserFromOrganizationWithToken(memberToken, member.UserID, site.OrgID, operatorID); err != nil {
			log.Printf("[RemoveTransitSiteMember] IAM UnbindUser failed for user %s from org %s: %v", member.UserID, site.OrgID, err)
		}
	}

	if err := db.Where("id = ? AND site_id = ?", memberID, siteID).Delete(&models.SiteMember{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to remove member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "removed"})
}
