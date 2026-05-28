package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"
)

type RoleManageHandler struct {
	db           *gorm.DB
	iamClient    *services.IAMClient
	permRegistry *services.PermissionRegistry
}

func NewRoleManageHandler(db *gorm.DB, iamClient *services.IAMClient, permRegistry *services.PermissionRegistry) *RoleManageHandler {
	return &RoleManageHandler{db: db, iamClient: iamClient, permRegistry: permRegistry}
}

type LocalRole struct {
	ID            string   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID      string   `gorm:"type:uuid;not null" json:"tenant_id"`
	IAMTemplateID string   `gorm:"type:varchar(100)" json:"iam_template_id"`
	Name          string   `gorm:"type:varchar(100);not null" json:"name"`
	Code          string   `gorm:"type:varchar(50);not null" json:"code"`
	CusPermCodes  pq.StringArray `gorm:"type:text[];default:'{}'" json:"cus_perm_codes"`
	IsSystem      bool     `gorm:"default:false" json:"is_system"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type RoleResp struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Code          string   `json:"code"`
	CusPermCodes  []string `json:"cus_perm_codes"`
	IsSystem      bool     `json:"is_system"`
	PermissionCount int    `json:"permission_count"`
}

func (h *RoleManageHandler) ListRoles(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "no tenant in context"})
		return
	}

	var localRoles []LocalRole
	if err := h.db.Table("roles").Where("tenant_id = ?", tenantID).Find(&localRoles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list roles: " + err.Error()})
		return
	}

	roleMap := make(map[string]RoleResp)
	for _, r := range localRoles {
		roleMap[r.Code] = RoleResp{
			ID:              r.ID,
			Name:            r.Name,
			Code:            r.Code,
			CusPermCodes:    r.CusPermCodes,
			IsSystem:        r.IsSystem,
			PermissionCount: len(r.CusPermCodes),
		}
	}

	for code, tpl := range services.AllRoleTemplates {
		if _, exists := roleMap[code]; !exists {
			roleMap[code] = RoleResp{
				Name:            tpl.Name,
				Code:            code,
				CusPermCodes:    tpl.CusPermCodes,
				IsSystem:        true,
				PermissionCount: len(tpl.CusPermCodes),
			}
		}
	}

	result := make([]RoleResp, 0, len(roleMap))
	for _, r := range roleMap {
		result = append(result, r)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

type createRoleReq struct {
	Name         string   `json:"name" binding:"required"`
	Code         string   `json:"code" binding:"required"`
	CusPermCodes []string `json:"cus_perm_codes" binding:"required"`
}

func (h *RoleManageHandler) CreateRole(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	nsID := middleware.GetNamespaceID(c.Request.Context())
	adminCusPerm := middleware.GetCusPerm(c.Request.Context())

	var req createRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid request: " + err.Error()})
		return
	}

	for _, code := range req.CusPermCodes {
		bit := h.permRegistry.GetCusPermBit(code)
		if bit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid permission code: " + code})
			return
		}
		if adminCusPerm&(1<<bit) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "cannot assign permission you don't have: " + code})
			return
		}
	}

	cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(req.CusPermCodes, h.permRegistry.GetCusPermBit)
	templateID, err := h.iamClient.CreateRoleTemplate(nsID, req.Code, req.Name, cusPerm, cusPermExt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create role in IAM: " + err.Error()})
		return
	}

	role := LocalRole{
		TenantID:      tenantID,
		IAMTemplateID: templateID,
		Name:          req.Name,
		Code:          req.Code,
		CusPermCodes:  req.CusPermCodes,
		IsSystem:      false,
	}
	if err := h.db.Create(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save role locally: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "role created", "data": gin.H{"id": role.ID}})
}

type updateRoleReq struct {
	Name         *string  `json:"name"`
	CusPermCodes []string `json:"cus_perm_codes" binding:"required"`
}

func (h *RoleManageHandler) UpdateRole(c *gin.Context) {
	roleID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())
	nsID := middleware.GetNamespaceID(c.Request.Context())
	adminCusPerm := middleware.GetCusPerm(c.Request.Context())

	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid request: " + err.Error()})
		return
	}

	var role LocalRole
	if err := h.db.Where("id = ? AND tenant_id = ?", roleID, tenantID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "role not found"})
		return
	}

	for _, code := range req.CusPermCodes {
		bit := h.permRegistry.GetCusPermBit(code)
		if bit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid permission code: " + code})
			return
		}
		if adminCusPerm&(1<<bit) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "cannot assign permission you don't have: " + code})
			return
		}
	}

	cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(req.CusPermCodes, h.permRegistry.GetCusPermBit)
	if role.IAMTemplateID != "" {
		if err := h.iamClient.SyncRoleTemplateCusPerm(nsID, role.Code, cusPerm, cusPermExt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to sync permissions to IAM: " + err.Error()})
			return
		}
	}

	updates := map[string]interface{}{
		"cus_perm_codes": pq.Array(req.CusPermCodes),
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if err := h.db.Model(&role).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update role: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "role updated"})
}

func (h *RoleManageHandler) DeleteRole(c *gin.Context) {
	roleID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	var role LocalRole
	if err := h.db.Where("id = ? AND tenant_id = ?", roleID, tenantID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "role not found"})
		return
	}

	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "cannot delete system role"})
		return
	}

	var memberCount int64
	h.db.Model(&struct{}{}).Table("site_members").
		Joins("JOIN sites ON sites.id = site_members.site_id").
		Where("sites.tenant_id = ? AND site_members.role = ?", tenantID, role.Code).
		Count(&memberCount)
	if memberCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "role is assigned to members, reassign them first"})
		return
	}

	if err := h.db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to delete role: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "role deleted"})
}
