package handlers

import (
	"fmt"
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

// #2114 邀请与成员管理层级：申请 / 审批 / 邀请。
// 依赖 beaconiam#491（各级解绑 / customer 挂载 / bind 改角色 / 删用户收紧 / 登录上下文统一）。

const inviteTTL2114 = 72 * time.Hour // 3 天

func findUserByIdentifier(db *gorm.DB, identifier string) *models.User {
	if strings.TrimSpace(identifier) == "" {
		return nil
	}
	var u models.User
	if err := db.Where("phone = ? OR email = ?", identifier, identifier).First(&u).Error; err == nil {
		return &u
	}
	return nil
}

func localUserIDOrNil(c *gin.Context, db *gorm.DB) *string {
	if uid, err := middleware.LocalUserID(c.Request.Context(), db); err == nil && uid != "" {
		return &uid
	}
	return nil
}

// hasLocalTenantMembership 检查本地缓存表（site_members / merchant_members /
// technician_profiles）是否仍有该用户的成员行。**仅用于本地缓存行的软删决策**
// （§8.1：users 软删交叉污染）——不用于「是否本商户成员」业务判定（那以
// isIAMTenantMember 的 IAM 关系为准，计划 §2.2）。
func hasLocalTenantMembership(db *gorm.DB, tenantID, userID string) bool {
	var n int64
	db.Model(&models.SiteMember{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Count(&n)
	if n > 0 {
		return true
	}
	db.Model(&models.MerchantMember{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Count(&n)
	if n > 0 {
		return true
	}
	db.Model(&models.TechnicianProfile{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Count(&n)
	return n > 0
}

// siteAncestorOrgs 返回网点组织（C）自身的祖先组织集合：{C, …, 商户根 B}，
// **排除** namespace 平台根（is_primary，所有用户级联持有 A:member，参与交集会全员命中）。
// #2114：判定「是否本商户成员」以 IAM 关系为准（计划 §2.2），本地表仅展示缓存。
func siteAncestorOrgs(iamClient *services.IAMClient, siteOrgID string) (map[string]bool, error) {
	out := map[string]bool{}
	cur := siteOrgID
	for i := 0; i < 16 && cur != ""; i++ {
		org, err := iamClient.GetOrganization(cur)
		if err != nil {
			return nil, err
		}
		if org.IsPrimary {
			break
		}
		out[cur] = true
		if org.ParentID == nil || *org.ParentID == "" {
			break
		}
		cur = *org.ParentID
	}
	return out, nil
}

// resolveIAMSub 将本地 users.id 解析为 IAM sub（#2114：管理员代建用户 local id == IAM id，
// 自注册用户不同）。查不到时兜底返回原值。
func resolveIAMSub(db *gorm.DB, localUserID string) string {
	var u models.User
	if err := db.Select("iam_sub").Where("id = ?", localUserID).First(&u).Error; err == nil && u.IAMSub != "" {
		return u.IAMSub
	}
	return localUserID
}

// hasPendingInvite（#2114 审计修复）：同 (tenant, site, 对象) 是否已有待处理邀请/申请。
// siteID 空串表示商户/平台级（site_id IS NULL）。
func hasPendingInvite(db *gorm.DB, tenantID, siteID, identifier string, inviteeUserID *string) bool {
	q := db.Model(&models.StaffInvite{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{"pending_approval", "pending"})
	if siteID == "" {
		q = q.Where("site_id IS NULL")
	} else {
		q = q.Where("site_id = ?", siteID)
	}
	if inviteeUserID != nil && *inviteeUserID != "" {
		q = q.Where("invitee_user_id = ? OR invitee_identifier = ?", *inviteeUserID, identifier)
	} else {
		q = q.Where("invitee_identifier = ?", identifier)
	}
	var n int64
	q.Count(&n)
	return n > 0
}

// ApplySiteMembership POST /api/sites/:id/members/apply（#2114）
// 网点管理员为**非本商户成员**（P2 已注册 / P3 未注册）提交加入申请，交商户管理员审批。
func ApplySiteMembership(c *gin.Context) {
	siteID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var req struct {
		Identifier string `json:"identifier"`
		Role       string `json:"role"`
		Note       string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少被邀请人手机号/邮箱"})
		return
	}

	var site models.Site
	if err := db.Where("id = ? AND tenant_id = ?", siteID, tenantID).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "网点不存在"})
		return
	}
	if site.OrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "网点未绑定组织，无法申请"})
		return
	}

	invitee := findUserByIdentifier(db, req.Identifier)

	// 去重：同网点 + 同标识 的待审批/待接受 唯一
	var dup int64
	dq := db.Model(&models.StaffInvite{}).
		Where("tenant_id = ? AND site_id = ? AND status IN ?", tenantID, siteID, []string{"pending_approval", "pending"})
	if invitee != nil {
		dq = dq.Where("invitee_user_id = ? OR invitee_identifier = ?", invitee.ID, req.Identifier)
	} else {
		dq = dq.Where("invitee_identifier = ?", req.Identifier)
	}
	dq.Count(&dup)
	if dup > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该用户已有待处理的邀请申请"})
		return
	}

	inv := models.StaffInvite{
		TenantID:          tenantID,
		OrgID:             site.OrgID,
		SiteID:            &siteID,
		Kind:              "site_member",
		Role:              normalizeRole(req.Role),
		Status:            "pending_approval",
		Code:              newInviteCode(),
		ExpiresAt:         time.Now().Add(inviteTTL2114),
		RequestedBy:       localUserIDOrNil(c, db),
		RequestNote:       req.Note,
		InviteeIdentifier: req.Identifier,
	}
	if invitee != nil {
		inv.InviteeUserID = &invitee.ID
		inv.InviteeName = invitee.Name
	}
	if err := db.Create(&inv).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "提交申请失败"})
		return
	}

	content := fmt.Sprintf("网点「%s」申请邀请 %s 加入（角色：%s）", site.Name, req.Identifier, inv.Role)
	actionData := fmt.Sprintf(`{"invite_id":%q}`, inv.ID)
	services.NotifyMerchantAdmins(db, tenantID, "invite_application", "新的成员邀请申请",
		content, inv.ID, "staff_invite", "invite_manage", &actionData)

	c.JSON(http.StatusCreated, gin.H{"code": 20100, "data": gin.H{"invite_id": inv.ID, "status": inv.Status}})
}

// GetMyMerchant GET /api/admin/my-merchant（#2114）
// 当前商户管理员所属商户上下文（直属成员管理页邀请/创建用）。
func GetMyMerchant(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	orgID := middleware.GetOrgID(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var m models.Merchant
	if err := db.Where("org_id = ? OR tenant_id = ?", orgID, tenantID).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "未找到商户上下文"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id": m.ID, "name": m.Name, "org_id": m.OrgID,
	}})
}

// ListMerchantInvites GET /api/admin/invites（#2114）— 邀请管理列表（商户维度）。
func ListMerchantInvites(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	q := db.Model(&models.StaffInvite{}).Where("tenant_id = ?", tenantID)
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if s := c.Query("site_id"); s != "" {
		q = q.Where("site_id = ?", s)
	}
	// #2157：服务端分页（page/page_size + total）
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)
	var total int64
	q.Count(&total)
	var invites []models.StaffInvite
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&invites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	siteNames := map[string]string{}
	out := make([]gin.H, 0, len(invites))
	for _, inv := range invites {
		siteName := ""
		if inv.SiteID != nil {
			if n, ok := siteNames[*inv.SiteID]; ok {
				siteName = n
			} else {
				var s models.Site
				if err := db.Select("name").First(&s, "id = ?", *inv.SiteID).Error; err == nil {
					siteName = s.Name
					siteNames[*inv.SiteID] = s.Name
				}
			}
		}
		out = append(out, gin.H{
			"id":                 inv.ID,
			"kind":               inv.Kind,
			"site_id":            inv.SiteID,
			"site_name":          siteName,
			"invitee_name":       inv.InviteeName,
			"invitee_identifier": inv.InviteeIdentifier,
			"role":               inv.Role,
			"status":             inv.Status,
			"request_note":       inv.RequestNote,
			"reject_reason":      inv.RejectReason,
			"created_at":         inv.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"invites": out, "total": total, "page": page, "page_size": pageSize}})
}

// MerchantInvitesPendingCount GET /api/admin/invites/pending-count（#2114）— 待审批数。
func MerchantInvitesPendingCount(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var n int64
	if err := db.Model(&models.StaffInvite{}).
		Where("tenant_id = ? AND status = ?", tenantID, "pending_approval").
		Count(&n).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"count": n}})
}

// ApproveInvite POST /api/admin/invites/:id/approve（#2114）— 商户管理员同意申请 → 发邀请。
func ApproveInvite(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	id := c.Param("id")

	var inv models.StaffInvite
	if err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&inv).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "邀请不存在"})
		return
	}
	if inv.Status != "pending_approval" {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "该申请已被处理"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":      "pending",
		"approved_by": localUserIDOrNil(c, db),
		"approved_at": now,
		"expires_at":  now.Add(inviteTTL2114),
	}
	if inv.Code == "" {
		updates["code"] = newInviteCode()
	}
	if err := db.Model(&models.StaffInvite{}).Where("id = ?", inv.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "审批失败"})
		return
	}

	if inv.InviteeUserID != nil {
		services.Notify(db, tenantID, *inv.InviteeUserID, "staff_invite", "加入邀请",
			"你收到一个加入邀请，是否接受？", inv.ID, "staff_invite", "staff_invite")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": "pending"}})
}

// RejectInvite POST /api/admin/invites/:id/reject（#2114）— 商户管理员拒绝申请。
func RejectInvite(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	id := c.Param("id")

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	var inv models.StaffInvite
	if err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&inv).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "邀请不存在"})
		return
	}
	if inv.Status != "pending_approval" {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "该申请已被处理"})
		return
	}

	now := time.Now()
	if err := db.Model(&models.StaffInvite{}).Where("id = ?", inv.ID).Updates(map[string]interface{}{
		"status":        "rejected",
		"reject_reason": req.Reason,
		"approved_by":   localUserIDOrNil(c, db),
		"approved_at":   now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "审批失败"})
		return
	}

	if inv.RequestedBy != nil {
		services.Notify(db, tenantID, *inv.RequestedBy, "invite_rejected", "邀请申请被拒绝",
			"你的成员邀请申请未获批准"+"："+req.Reason, inv.ID, "staff_invite", "info")
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": "rejected"}})
}

// InviteToMerchant POST /api/admin/merchants/:id/invites（#2114）
// 商户管理员直接邀请用户加入商户（merchant_member）或作为商户直属员工（merchant_staff）。
func InviteToMerchant(c *gin.Context) {
	merchantID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		Identifier string `json:"identifier"`
		Kind       string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少被邀请人手机号/邮箱"})
		return
	}

	// 商户组织（target org）：merchants.org_id
	var merchant models.Merchant
	if err := db.Where("id = ? AND tenant_id = ?", merchantID, tenantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "商户不存在"})
		return
	}
	if merchant.OrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "商户未绑定组织，无法邀请"})
		return
	}

	kind := req.Kind
	role := "member"
	switch kind {
	case "merchant_staff":
		role = "merchant_staff" // #2129：直属员工专用角色（含 repair:read）
	default:
		kind = "merchant_member"
	}

	invitee := findUserByIdentifier(db, req.Identifier)
	// #2114 审计修复：直邀也须去重（此前无查重，重复邀请同一人）
	var inviteeID *string
	if invitee != nil {
		inviteeID = &invitee.ID
	}
	if hasPendingInvite(db, tenantID, "", req.Identifier, inviteeID) {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该用户已有待处理的邀请"})
		return
	}

	inv := models.StaffInvite{
		TenantID:          tenantID,
		OrgID:             merchant.OrgID,
		SiteID:            nil, // 商户级
		Kind:              kind,
		Role:              role,
		Status:            "pending",
		Code:              newInviteCode(),
		ExpiresAt:         time.Now().Add(inviteTTL2114),
		CreatedBy:         localUserIDOrNil(c, db),
		InviteeIdentifier: req.Identifier,
	}
	if invitee != nil {
		inv.InviteeUserID = &invitee.ID
		inv.InviteeName = invitee.Name
	}
	if err := db.Create(&inv).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "邀请失败"})
		return
	}
	if invitee != nil {
		services.Notify(db, tenantID, invitee.ID, "staff_invite", "加入商户邀请",
			fmt.Sprintf("邀请你加入「%s」，是否接受？", merchant.Name), inv.ID, "staff_invite", "staff_invite")
	}

	c.JSON(http.StatusCreated, gin.H{"code": 20100, "data": gin.H{
		"invite_id": inv.ID, "code": inv.Code, "kind": kind, "role": role, "status": inv.Status,
	}})
}

// InvitePlatformStaff POST /api/admin/platform-staff/invites（#2114）
// 平台管理员（system_admin）直接邀请既有用户成为平台直属成员。
func InvitePlatformStaff(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	rootOrgID := middleware.GetOrgID(ctx)
	if rootOrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "无法解析平台根组织"})
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少被邀请人手机号/邮箱"})
		return
	}

	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000" // 平台级：无租户，使用零 UUID 满足非空约束
	}

	invitee := findUserByIdentifier(db, req.Identifier)
	// #2114 审计修复：平台直邀去重
	var inviteeID *string
	if invitee != nil {
		inviteeID = &invitee.ID
	}
	if hasPendingInvite(db, tenantID, "", req.Identifier, inviteeID) {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该用户已有待处理的邀请"})
		return
	}
	inv := models.StaffInvite{
		TenantID:          tenantID,
		OrgID:             rootOrgID,
		SiteID:            nil,
		Kind:              "platform_staff",
		Role:              "site_member", // 结构角色 STAFF（O5 复用现有角色）
		Status:            "pending",
		Code:              newInviteCode(),
		ExpiresAt:         time.Now().Add(inviteTTL2114),
		CreatedBy:         localUserIDOrNil(c, db),
		InviteeIdentifier: req.Identifier,
	}
	if invitee != nil {
		inv.InviteeUserID = &invitee.ID
		inv.InviteeName = invitee.Name
	}
	if err := db.Create(&inv).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "邀请失败"})
		return
	}
	if invitee != nil {
		services.Notify(db, tenantID, invitee.ID, "staff_invite", "平台邀请",
			"邀请你加入平台，是否接受？", inv.ID, "staff_invite", "staff_invite")
	}

	c.JSON(http.StatusCreated, gin.H{"code": 20100, "data": gin.H{
		"invite_id": inv.ID, "code": inv.Code, "status": inv.Status,
	}})
}
