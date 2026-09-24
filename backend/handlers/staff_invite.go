package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const inviteTTL = 7 * 24 * time.Hour

func newInviteCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) // 16 hex chars
}

// CreateSiteInvite POST /api/sites/:id/invites — 管理员签发邀请码（#2031 邀请制自助加入）。
// 管理员只发码；成员身份由本人登录后接受，从而满足一人一号 + 禁止管理员代挂。
func CreateSiteInvite(c *gin.Context) {
	siteID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var req struct {
		Role      string `json:"role"`
		ExpiresIn int    `json:"expires_in_hours"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Role == "" {
		req.Role = "site_member"
	}

	var site models.Site
	if err := db.Where("id = ? AND tenant_id = ?", siteID, tenantID).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "网点不存在"})
		return
	}
	if site.OrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "网点未绑定组织，无法邀请"})
		return
	}

	ttl := inviteTTL
	if req.ExpiresIn > 0 {
		ttl = time.Duration(req.ExpiresIn) * time.Hour
	}

	var createdBy *string
	if uid := middleware.GetUserID(ctx); uid != "" {
		createdBy = &uid
	}

	invite := models.StaffInvite{
		TenantID:  tenantID,
		OrgID:     site.OrgID,
		SiteID:    siteID,
		Role:      normalizeRole(req.Role),
		Code:      newInviteCode(),
		CreatedBy: createdBy,
		ExpiresAt: time.Now().Add(ttl),
		Status:    "pending",
	}
	if err := db.Create(&invite).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "生成邀请码失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id":         invite.ID,
			"code":       invite.Code,
			"role":       invite.Role,
			"site_id":    siteID,
			"site_name":  site.Name,
			"expires_at": invite.ExpiresAt,
		},
	})
}

// ListSiteInvites GET /api/sites/:id/invites — 待接受邀请列表（管理员）。
func ListSiteInvites(c *gin.Context) {
	siteID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var invites []models.StaffInvite
	db.Where("tenant_id = ? AND site_id = ? AND status = ? AND expires_at > ?", tenantID, siteID, "pending", time.Now()).
		Order("created_at DESC").Find(&invites)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"invites": invites}})
}

// AcceptInvite POST /api/user/accept-invite — 本人用邀请码加入（userOptionalAuth）。
// 关系绑定到**调用者自己的账户**（一人一号），管理员不得代挂。
func AcceptInvite(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if middleware.GetUserID(ctx) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}
	// #2052/#1742：JWT sub 是 IAM 侧 id，须解析为本地 users.id 再与 invitee_user_id 比对
	localUserID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "账户解析失败，请稍后重试"})
		return
	}
	if localUserID == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少邀请码"})
		return
	}

	var invite models.StaffInvite
	if err := db.Where("code = ?", req.Code).First(&invite).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "邀请码无效"})
		return
	}
	if invite.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "邀请码已被使用或已失效"})
		return
	}
	if time.Now().After(invite.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"code": 41000, "message": "邀请码已过期"})
		return
	}

	var user models.User
	if err := db.Where("id = ?", localUserID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}
	if user.IAMSub == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "账户未与 IAM 同步，请重新登录后再试"})
		return
	}

	// Idempotent: already a member of this site → mark accepted and return.
	var existing int64
	db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", invite.TenantID, invite.SiteID, user.ID).
		Count(&existing)
	if existing == 0 && invite.SiteID != "" {
		iamRole := toIAMRole(invite.Role)
		if err := services.NewIAMClient().BindUserToOrganization(user.IAMSub, invite.OrgID, iamRole, user.IAMSub); err != nil {
			log.Printf("[AcceptInvite] IAM bind failed user=%s org=%s: %v", user.ID, invite.OrgID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "加入失败，请稍后重试"})
			return
		}
		if err := db.Create(&models.SiteMember{
			TenantID: invite.TenantID,
			SiteID:   invite.SiteID,
			UserID:   user.ID,
			Role:     invite.Role,
			Roles:    []string{invite.Role}, // #2034
		}).Error; err != nil {
			log.Printf("[AcceptInvite] local site_member create failed user=%s site=%s: %v", user.ID, invite.SiteID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "加入失败，请稍后重试"})
			return
		}
	}

	now := time.Now()
	db.Model(&models.StaffInvite{}).Where("id = ?", invite.ID).Updates(map[string]interface{}{
		"status":      "accepted",
		"accepted_by": user.ID,
		"accepted_at": now,
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"site_id": invite.SiteID, "role": invite.Role, "is_member": true},
	})
}

// bindInvitee 将 user 绑定到 invite 对应网点（IAM BindUserToOrganization + 本地 site_members），幂等。
// 本人账户绑定（一人一号），管理员不得代挂（#2031/#2052）。
func bindInvitee(db *gorm.DB, invite *models.StaffInvite, user *models.User) error {
	var existing int64
	db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", invite.TenantID, invite.SiteID, user.ID).
		Count(&existing)
	if existing > 0 || invite.SiteID == "" {
		return nil // 已是成员 → 幂等
	}
	iamRole := toIAMRole(invite.Role)
	if err := services.NewIAMClient().BindUserToOrganization(user.IAMSub, invite.OrgID, iamRole, user.IAMSub); err != nil {
		return fmt.Errorf("iam bind: %w", err)
	}
	if err := db.Create(&models.SiteMember{
		TenantID: invite.TenantID,
		SiteID:   invite.SiteID,
		UserID:   user.ID,
		Role:     invite.Role,
		Roles:    []string{invite.Role}, // #2034
	}).Error; err != nil {
		return fmt.Errorf("site_member create: %w", err)
	}
	return nil
}

// loadInvitationForUser 加载邀请并校验「处理人 = 被邀请人」+ 状态/有效期（#2052）。
func loadInvitationForUser(db *gorm.DB, id, localUserID string, c *gin.Context) (*models.StaffInvite, bool) {
	var invite models.StaffInvite
	if err := db.Where("id = ?", id).First(&invite).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "邀请不存在"})
		return nil, false
	}
	if invite.InviteeUserID == nil || *invite.InviteeUserID != localUserID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无权处理该邀请"})
		return nil, false
	}
	if invite.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "邀请已处理或已失效"})
		return nil, false
	}
	if time.Now().After(invite.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"code": 41000, "message": "邀请已过期"})
		return nil, false
	}
	return &invite, true
}

// SendStaffInvite POST /api/sites/:id/invites/send — #2052 撞库邀请：
// 管理员对**既有用户**发起加入邀请（不发码），被邀请人在「系统消息」接受/拒绝。
func SendStaffInvite(c *gin.Context) {
	siteID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var req struct {
		Identifier string `json:"identifier"` // 手机号或邮箱
		Role       string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少手机号/邮箱"})
		return
	}
	identifier := strings.TrimSpace(req.Identifier)
	if req.Role == "" {
		req.Role = "site_member"
	}
	role := normalizeRole(req.Role)

	var site models.Site
	if err := db.Where("id = ? AND tenant_id = ?", siteID, tenantID).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "网点不存在"})
		return
	}
	if site.OrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "网点未绑定组织，无法邀请"})
		return
	}

	var invitee models.User
	if err := db.Where("phone = ? OR email = ?", identifier, identifier).First(&invitee).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "未找到该手机号/邮箱对应的账户"})
		return
	}

	var existing int64
	db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, invitee.ID).Count(&existing)
	if existing > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该用户已是本网点成员"})
		return
	}

	var createdBy *string
	if uid := middleware.GetUserID(ctx); uid != "" {
		createdBy = &uid
	}

	invite := models.StaffInvite{
		TenantID:      tenantID,
		OrgID:         site.OrgID,
		SiteID:        siteID,
		InviteeUserID: &invitee.ID,
		Role:          role,
		Code:          newInviteCode(),
		CreatedBy:     createdBy,
		ExpiresAt:     time.Now().Add(inviteTTL),
		Status:        "pending",
	}
	if err := db.Create(&invite).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "发送邀请失败"})
		return
	}

	services.Notify(db, tenantID, invitee.ID, "staff_invite",
		"网点加入邀请",
		fmt.Sprintf("邀请你加入网点「%s」（角色：%s），是否接受？", site.Name, role),
		invite.ID, "staff_invite", "staff_invite")

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"invite_id":    invite.ID,
			"invitee_name": invitee.Name,
			"site_id":      siteID,
			"role":         role,
		},
	})
}

// AcceptInvitation POST /api/user/invitations/:id/accept — 被邀请人本人接受（#2052）。
func AcceptInvitation(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if middleware.GetUserID(ctx) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}
	// #2052/#1742：JWT sub 是 IAM 侧 id，须解析为本地 users.id 再与 invitee_user_id 比对
	localUserID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "账户解析失败，请稍后重试"})
		return
	}
	if localUserID == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}

	invite, ok := loadInvitationForUser(db, c.Param("id"), localUserID, c)
	if !ok {
		return
	}

	var user models.User
	if err := db.Where("id = ?", localUserID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}
	if user.IAMSub == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "账户未与 IAM 同步，请重新登录后再试"})
		return
	}
	if err := bindInvitee(db, invite, &user); err != nil {
		log.Printf("[AcceptInvitation] bind failed user=%s site=%s: %v", user.ID, invite.SiteID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "加入失败，请稍后重试"})
		return
	}

	now := time.Now()
	db.Model(&models.StaffInvite{}).Where("id = ?", invite.ID).Updates(map[string]interface{}{
		"status":      "accepted",
		"accepted_by": user.ID,
		"accepted_at": now,
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"site_id": invite.SiteID, "role": invite.Role, "is_member": true},
	})
}

// RejectInvitation POST /api/user/invitations/:id/reject — 被邀请人本人拒绝（#2052）。
func RejectInvitation(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if middleware.GetUserID(ctx) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}
	// #2052/#1742：JWT sub 是 IAM 侧 id，须解析为本地 users.id 再与 invitee_user_id 比对
	localUserID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "账户解析失败，请稍后重试"})
		return
	}
	if localUserID == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}

	invite, ok := loadInvitationForUser(db, c.Param("id"), localUserID, c)
	if !ok {
		return
	}

	db.Model(&models.StaffInvite{}).Where("id = ?", invite.ID).Update("status", "rejected")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": "rejected"}})
}
