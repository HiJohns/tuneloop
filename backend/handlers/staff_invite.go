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

// ListMyPendingInvitations GET /api/user/invitations/pending — 我的待接受「定向」邀请（#2216）。
// 匹配口径与 member_invite_2114 一致：invitee_user_id = 我，或（invitee_user_id IS NULL
// 且 invitee_identifier ∈ {我的手机, 我的邮箱}）。不含邀请码；未登录/解析失败返回空列表。
func ListMyPendingInvitations(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	emptyList := gin.H{"code": 20000, "data": gin.H{"list": []gin.H{}}}

	if middleware.GetUserID(ctx) == "" {
		c.JSON(http.StatusOK, emptyList)
		return
	}
	localUserID, err := middleware.LocalUserID(ctx, db)
	if err != nil || localUserID == "" {
		c.JSON(http.StatusOK, emptyList)
		return
	}
	var me models.User
	if err := db.Select("id", "phone", "email").Where("id = ?", localUserID).First(&me).Error; err != nil {
		c.JSON(http.StatusOK, emptyList)
		return
	}

	identifiers := []string{}
	for _, v := range []string{me.Phone, me.Email} {
		if strings.TrimSpace(v) != "" {
			identifiers = append(identifiers, v)
		}
	}

	q := db.Model(&models.StaffInvite{}).
		Where("status = ? AND expires_at > ?", "pending", time.Now())
	if len(identifiers) > 0 {
		q = q.Where("invitee_user_id = ? OR (invitee_user_id IS NULL AND invitee_identifier IN ?)", localUserID, identifiers)
	} else {
		q = q.Where("invitee_user_id = ?", localUserID)
	}

	var invites []models.StaffInvite
	if err := q.Order("created_at DESC").Find(&invites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	roleLabel := map[string]string{
		"site_member":     "网点员工",
		"merchant_member": "商户成员",
		"merchant_staff":  "商户员工",
		"platform_staff":  "平台员工",
	}
	siteNames := map[string]string{}
	out := make([]gin.H, 0, len(invites))
	for _, inv := range invites {
		name := ""
		if inv.SiteID != nil {
			if n, ok := siteNames[*inv.SiteID]; ok {
				name = n
			} else {
				var s models.Site
				if err := db.Select("name").First(&s, "id = ?", *inv.SiteID).Error; err == nil {
					name = s.Name
					siteNames[*inv.SiteID] = s.Name
				}
			}
		} else {
			// 商户/平台级（site_id 为 NULL）：按 OrgID 解析商户名。
			var mrec models.Merchant
			if err := db.Select("name").First(&mrec, "org_id = ?", inv.OrgID).Error; err == nil {
				name = mrec.Name
			}
		}
		label := roleLabel[inv.Role]
		if label == "" {
			label = inv.Role
		}
		out = append(out, gin.H{
			"id":         inv.ID,
			"kind":       inv.Kind,
			"site_id":    inv.SiteID,
			"name":       name,
			"role":       inv.Role,
			"role_label": label,
			"created_at": inv.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": out}})
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
	if existing == 0 && invite.SiteID != nil {
		iamRole := toIAMRole(invite.Role)
		if err := services.NewIAMClient().BindUserToOrganization(user.IAMSub, invite.OrgID, iamRole, user.IAMSub); err != nil {
			log.Printf("[AcceptInvite] IAM bind failed user=%s org=%s: %v", user.ID, invite.OrgID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "加入失败，请稍后重试"})
			return
		}
		if err := db.Create(&models.SiteMember{
			TenantID: invite.TenantID,
			SiteID:   *invite.SiteID,
			UserID:   user.ID,
			Role:     invite.Role,
			Roles:    []string{invite.Role}, // #2034
		}).Error; err != nil {
			log.Printf("[AcceptInvite] local site_member create failed user=%s site=%s: %v", user.ID, strOrEmpty(invite.SiteID), err)
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
	if existing > 0 || invite.SiteID == nil {
		return nil // 已是成员 → 幂等
	}
	iamRole := toIAMRole(invite.Role)
	if err := services.NewIAMClient().BindUserToOrganization(user.IAMSub, invite.OrgID, iamRole, user.IAMSub); err != nil {
		return fmt.Errorf("iam bind: %w", err)
	}
	if err := db.Create(&models.SiteMember{
		TenantID: invite.TenantID,
		SiteID:   *invite.SiteID,
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
	// #2216: 匹配口径与「我的待接受邀请」查询/ member_invite_2114 一致——
	// 指向我（invitee_user_id）或（未认领且 identifier ∈ {我的手机, 我的邮箱}）。
	match := invite.InviteeUserID != nil && *invite.InviteeUserID == localUserID
	if !match && invite.InviteeUserID == nil && invite.InviteeIdentifier != "" {
		var me models.User
		if err := db.Select("phone", "email").Where("id = ?", localUserID).First(&me).Error; err == nil {
			match = invite.InviteeIdentifier == me.Phone || (me.Email != "" && invite.InviteeIdentifier == me.Email)
		}
	}
	if !match {
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
		SiteID:        &siteID,
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

	// #2216: identifier 定向邀请在接受时认领（invitee_user_id ← 本人），与一键接受口径一致。
	if invite.InviteeUserID == nil {
		db.Model(&models.StaffInvite{}).Where("id = ?", invite.ID).Update("invitee_user_id", localUserID)
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
		log.Printf("[AcceptInvitation] bind failed user=%s site=%s: %v", user.ID, strOrEmpty(invite.SiteID), err)
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

// strOrEmpty 安全打印可空 site_id（#2114：商户/平台级邀请 site_id 为 NULL）
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
