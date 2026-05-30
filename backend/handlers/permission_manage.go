package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

type PermissionManageHandler struct {
	db           *gorm.DB
	iamClient    *services.IAMClient
	permRegistry *services.PermissionRegistry
}

func NewPermissionManageHandler(db *gorm.DB, iamClient *services.IAMClient, permRegistry *services.PermissionRegistry) *PermissionManageHandler {
	return &PermissionManageHandler{db: db, iamClient: iamClient, permRegistry: permRegistry}
}

type MemberPermissionResp struct {
	UserID       string   `json:"user_id"`
	Name         string   `json:"name"`
	SiteID       string   `json:"site_id"`
	SiteName     string   `json:"site_name"`
	RoleCode     string   `json:"role_code"`
	RoleName     string   `json:"role_name"`
	CusPermCodes []string `json:"cus_perm_codes"`
}

func (h *PermissionManageHandler) ListUsers(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "no tenant in context"})
		return
	}

	type MemberRow struct {
		UserID   string         `gorm:"column:user_id"`
		Name     string         `gorm:"column:name"`
		SiteID   string         `gorm:"column:site_id"`
		SiteName string         `gorm:"column:site_name"`
		Role     string         `gorm:"column:role"`
		Codes    sql.NullString `gorm:"column:cus_perm_codes"`
	}

	var rows []MemberRow
	err := h.db.Raw(`
		SELECT sm.user_id, u.name, sm.site_id, s.name as site_name, sm.role, sm.cus_perm_codes
		FROM site_members sm
		JOIN users u ON u.iam_sub = sm.user_id::text
		JOIN sites s ON s.id = sm.site_id
		WHERE s.tenant_id = ?
		ORDER BY s.name, u.name
	`, tenantID).Scan(&rows).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list users: " + err.Error()})
		return
	}

	result := make([]MemberPermissionResp, len(rows))
	for i, m := range rows {
		roleTpl, _ := services.GetRoleTemplate(m.Role)
		var codes []string
		if m.Codes.Valid {
			codes = parsePQArray(m.Codes.String)
		}
		result[i] = MemberPermissionResp{
			UserID:       m.UserID,
			Name:         m.Name,
			SiteID:       m.SiteID,
			SiteName:     m.SiteName,
			RoleCode:     m.Role,
			RoleName:     roleTpl.Name,
			CusPermCodes: codes,
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

type setUserPermReq struct {
	CusPermCodes []string `json:"cus_perm_codes" binding:"required"`
}

func (h *PermissionManageHandler) SetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	orgID := middleware.GetOrgID(c.Request.Context())
	adminCusPerm := middleware.GetCusPerm(c.Request.Context())

	var req setUserPermReq
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
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "cannot grant permission you don't have: " + code})
			return
		}
	}

	cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(req.CusPermCodes, h.permRegistry.GetCusPermBit)
	if err := h.iamClient.SetUserCustomerPermissions(orgID, userID, cusPerm, cusPermExt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to set permissions: " + err.Error()})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	if err := h.db.Exec("UPDATE site_members SET cus_perm_codes = ? FROM sites WHERE site_members.site_id = sites.id AND sites.tenant_id = ? AND site_members.user_id = ?", pq.Array(req.CusPermCodes), tenantID, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update local cache: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "permissions updated, will take effect on next login"})
}

type setUserRoleReq struct {
	RoleCode string `json:"role_code" binding:"required"`
}

func (h *PermissionManageHandler) SetUserRole(c *gin.Context) {
	userID := c.Param("id")
	orgID := middleware.GetOrgID(c.Request.Context())
	log.Printf("[SetUserRole] Request: userID=%s orgID=%s", userID, orgID)

	// Override orgID with the target user's actual org from users table
	var targetUser models.User
	if err := h.db.Where("id = ? AND deleted_at IS NULL", userID).First(&targetUser).Error; err == nil && targetUser.OrgID != "" {
		orgID = targetUser.OrgID
		log.Printf("[SetUserRole] orgID overridden by users.OrgID: %s", orgID)
	}

	var req setUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid request: " + err.Error()})
		return
	}
	log.Printf("[SetUserRole] request body: role_code=%s", req.RoleCode)

	// Look up IAMSub from local users table (IAM expects the IAM user ID, not local UUID)
	iamUserID := userID
	var iamUser models.User
	if err := h.db.Where("id = ? AND deleted_at IS NULL", userID).First(&iamUser).Error; err == nil && iamUser.IAMSub != "" {
		iamUserID = iamUser.IAMSub
		log.Printf("[SetUserRole] IAMSub found by users.id: iamUserID=%s", iamUserID)
	} else {
		// Fallback: the URL param "id" might be the IAMSub itself, try searching by iam_sub
		var userBySub models.User
		if err := h.db.Where("iam_sub = ? AND deleted_at IS NULL", userID).First(&userBySub).Error; err == nil {
			iamUserID = userBySub.IAMSub
			log.Printf("[SetUserRole] IAMSub found by users.iam_sub: iamUserID=%s", iamUserID)
		} else {
			// Fallback: look up via site_members → users join
			type userIDResult struct{ UserID string }
			var uidResult userIDResult
			if err := h.db.Table("site_members").
				Select("users.iam_sub").
				Joins("JOIN users ON users.id = site_members.user_id").
				Where("site_members.user_id = ? AND users.iam_sub IS NOT NULL AND users.iam_sub != ''", userID).
				First(&uidResult).Error; err == nil && uidResult.UserID != "" {
				iamUserID = uidResult.UserID
				log.Printf("[SetUserRole] IAMSub found via site_members join: iamUserID=%s", iamUserID)
			} else {
				log.Printf("[SetUserRole] WARNING: IAMSub not found for userID=%s, using local UUID", userID)
			}
		}
	}

	userToken := services.ExtractUserToken(c)
	log.Printf("[SetUserRole] Calling IAM: token_len=%d org=%s user=%s role=%s", len(userToken), orgID, iamUserID, toIAMRole(req.RoleCode))

	iamRole := toIAMRole(req.RoleCode)
	if len(userToken) == 0 {
		log.Printf("[SetUserRole] ERROR: empty user token")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50002, "message": "user token not found"})
		return
	}

	if err := h.iamClient.UpdateUserRoleInOrgWithToken(userToken, orgID, iamUserID, iamRole); err != nil {
		log.Printf("[SetUserRole] UpdateUserRoleInOrgWithToken failed: userID=%s iamUserID=%s orgID=%s role=%s err=%v", userID, iamUserID, orgID, iamRole, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update role: " + err.Error()})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	if err := h.db.Exec("UPDATE site_members SET role = ? FROM sites WHERE site_members.site_id = sites.id AND sites.tenant_id = ? AND site_members.user_id = ?", req.RoleCode, tenantID, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update local cache: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "role updated, will take effect on next login"})
}

func parsePQArray(s string) []string {
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func toIAMRole(tuneloopCode string) string {
	switch tuneloopCode {
	case "merchant_admin":
		return "OWNER"
	case "site_admin":
		return "ADMIN"
	case "site_member":
		return "STAFF"
	case "worker":
		return "WORKER"
	default:
		return tuneloopCode
	}
}
