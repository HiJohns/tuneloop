package handlers

// #1935: 平台级「中转网点」管理（Platform-direct transit sites）
// — 归属平台顶层组织（创建者上下文的租户），与商户 SiteManagement 租户作用域分离。
// — 成员管理复用 site_members，角色收敛为中转网点管理员/成员两类。
// — 被 TransitRoute 引用的中转网点禁止停用/删除（需先解绑路由）。

import (
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const transitSiteType = "transit"

// ListAdminTransitSites returns platform-owned transit sites with route usage counts.
func ListAdminTransitSites(c *gin.Context) {
	db := database.GetDB()
	var sites []models.Site
	if err := db.Where("type = ? AND status = ?", transitSiteType, "active").
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
	site := models.Site{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     tenantID,
		Name:      req.Name,
		Address:   req.Address,
		Phone:     req.Phone,
		Type:      transitSiteType,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
	var members []models.SiteMember
	if err := db.Where("site_id = ?", siteID).Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list members"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": members}})
}

// AddTransitSiteMember adds a member (role restricted to site_admin / site_member).
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
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": member})
}

// RemoveTransitSiteMember removes a member from a transit site.
func RemoveTransitSiteMember(c *gin.Context) {
	memberID := c.Param("member_id")
	db := database.GetDB()
	if err := db.Where("id = ?", memberID).Delete(&models.SiteMember{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to remove member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "removed"})
}
