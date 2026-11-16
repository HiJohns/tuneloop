package handlers

// #2394（父 #2385）：成员移除审批流后端。
//
// 业务规则（#2385 D3）：网点管理员不能直接开除成员，必须提交移除申请，
// 商户管理员批准后由服务端执行解绑（IAM 关系 + 本地成员行）；拒绝/撤回
// 只改状态，不触碰成员关系。
//
// 状态机：pending_approval → approved → executed（失败 → failed）
//
//	└→ rejected
//	└→ cancelled（仅申请人）
import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	removalStatusPending  = "pending_approval"
	removalStatusApproved = "approved"
	removalStatusRejected = "rejected"
	removalStatusCancel   = "cancelled"
	removalStatusExecuted = "executed"
	removalStatusFailed   = "failed"

	removalReasonMaxLen = 200
)

type MemberRemovalRequestHandler struct{}

func NewMemberRemovalRequestHandler() *MemberRemovalRequestHandler {
	return &MemberRemovalRequestHandler{}
}

// requireMerchantAdmin2394 仅允许商户管理员访问审批入口（系统管理员另行授权时再扩展）。
// 网点管理员即便 IAM 角色是 ADMIN 也不能审批（审批权在商户层）。
func requireMerchantAdmin2394(c *gin.Context) bool {
	if middleware.GetBusinessRole(c.Request.Context()) != middleware.BusinessRoleMerchantAdmin {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "仅商户管理员可处理成员移除申请"})
		return false
	}
	return true
}

// resolveSiteMerchant2394 用 site 的归属锚（site.TenantID = 商户 org，见 #2266）
// 解析商户；merchantConfigDB 跳过自动 tenant scope（merchants.tenant_id 是命名空间 org）。
func resolveSiteMerchant2394(c *gin.Context, site models.Site) (models.Merchant, bool) {
	ctx := c.Request.Context()
	return resolveOperatorMerchant2247(merchantConfigDB(ctx), site.TenantID, middleware.GetTenantID(ctx))
}

// CreateSiteMemberRemovalRequest POST /api/sites/:id/members/:user_id/removal-requests
// 网点管理员提交移除申请（pending_approval），并通知本商户管理员。
func (h *MemberRemovalRequestHandler) CreateSiteMemberRemovalRequest(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	siteID := c.Param("id")
	targetUserID := c.Param("user_id")
	if targetUserID == "" {
		targetUserID = c.Param("uid")
	}

	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason is required"})
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason is required"})
		return
	}
	if utf8.RuneCountInString(reason) > removalReasonMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason must be at most 200 characters"})
		return
	}
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_id is required"})
		return
	}

	var site models.Site
	if err := db.Where("id = ? AND tenant_id = ?", siteID, tenantID).First(&site).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Site not found"})
		return
	}
	merchant, ok := resolveSiteMerchant2394(c, site)
	if !ok || merchant.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "merchant not found for site"})
		return
	}

	var member models.SiteMember
	if err := db.Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, targetUserID).
		First(&member).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Member not found"})
		return
	}

	requesterID, err := middleware.LocalUserID(ctx, db)
	if err != nil || requesterID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无法识别申请人（本地用户不存在）"})
		return
	}

	// 预检重复（DB 侧另有 partial unique index 兜底并发）
	var pending int64
	if err := db.Model(&models.MemberRemovalRequest{}).
		Where("tenant_id = ? AND site_id = ? AND target_user_id = ? AND status = ?", tenantID, siteID, targetUserID, removalStatusPending).
		Count(&pending).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败：" + err.Error()})
		return
	}
	if pending > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该成员已有待审批的移除申请"})
		return
	}

	siteIDCopy := siteID
	req := models.MemberRemovalRequest{
		TenantID:     tenantID,
		MerchantID:   merchant.ID,
		SiteID:       &siteIDCopy,
		TargetUserID: targetUserID,
		RequesterID:  requesterID,
		Reason:       reason,
		Status:       removalStatusPending,
	}
	if err := db.Create(&req).Error; err != nil {
		if isDuplicateKey2394(err) {
			c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "该成员已有待审批的移除申请"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "提交移除申请失败：" + err.Error()})
		return
	}

	actionData := fmt.Sprintf(`{"removal_request_id":%q}`, req.ID)
	services.NotifyMerchantAdmins(db, tenantID, "removal_request", "新的成员移除申请",
		fmt.Sprintf("网点「%s」申请移除成员，原因：%s", site.Name, reason),
		req.ID, "member_removal_request", "removal_manage", &actionData)

	writeAuditLog(db, tenantID, middleware.GetUserID(ctx), "member_removal_request", "member_removal_request", req.ID,
		map[string]interface{}{
			"merchant_id":    req.MerchantID,
			"site_id":        siteID,
			"target_user_id": targetUserID,
			"requester_id":   requesterID,
			"reason":         reason,
		})

	c.JSON(http.StatusCreated, gin.H{"code": 20100, "data": gin.H{
		"request_id": req.ID,
		"status":     req.Status,
	}})
}

// ListRemovalRequests GET /api/admin/removal-requests?status=&page=&page_size=
// 商户管理员查看本商户范围的移除申请（服务端分页）。
func (h *MemberRemovalRequestHandler) ListRemovalRequests(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireMerchantAdmin2394(c) {
		return
	}
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	q := db.Model(&models.MemberRemovalRequest{}).Where("tenant_id = ?", tenantID)
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		q = q.Where("status = ?", s)
	}
	if s := strings.TrimSpace(c.Query("site_id")); s != "" {
		q = q.Where("site_id = ?", s)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败：" + err.Error()})
		return
	}

	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)

	var rows []models.MemberRemovalRequest
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败：" + err.Error()})
		return
	}

	siteNames := map[string]string{}
	userNames := map[string]string{}
	resolveName := func(id string) string {
		if n, ok := userNames[id]; ok {
			return n
		}
		var u models.User
		name := ""
		if err := db.Select("name").Where("id = ?", id).First(&u).Error; err == nil {
			name = u.Name
		}
		userNames[id] = name
		return name
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		siteName := ""
		if r.SiteID != nil {
			if n, ok := siteNames[*r.SiteID]; ok {
				siteName = n
			} else {
				var s models.Site
				if err := db.Select("name").Where("id = ?", *r.SiteID).First(&s).Error; err == nil {
					siteName = s.Name
				}
				siteNames[*r.SiteID] = siteName
			}
		}
		out = append(out, gin.H{
			"id":             r.ID,
			"merchant_id":    r.MerchantID,
			"site_id":        r.SiteID,
			"site_name":      siteName,
			"target_user_id": r.TargetUserID,
			"target_name":    resolveName(r.TargetUserID),
			"requester_id":   r.RequesterID,
			"requester_name": resolveName(r.RequesterID),
			"reason":         r.Reason,
			"status":         r.Status,
			"reject_reason":  r.RejectReason,
			"decided_by":     r.DecidedBy,
			"decided_at":     r.DecidedAt,
			"created_at":     r.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"requests":  out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}})
}

// ApproveRemovalRequest POST /api/admin/removal-requests/:id/approve
// 商户管理员批准：解绑 IAM 关系 → 删除本地成员行 → 状态 executed（失败 failed，不静默）。
func (h *MemberRemovalRequestHandler) ApproveRemovalRequest(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireMerchantAdmin2394(c) {
		return
	}
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	requestID := c.Param("id")

	var req models.MemberRemovalRequest
	if err := db.Where("id = ? AND tenant_id = ?", requestID, tenantID).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "移除申请不存在"})
		return
	}
	if req.Status != removalStatusPending {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "该申请已被处理"})
		return
	}

	deciderID, err := middleware.LocalUserID(ctx, db)
	if err != nil || deciderID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无法识别审批人（本地用户不存在）"})
		return
	}

	unbindOrg, err := removalUnbindOrg2394(ctx, db, tenantID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": err.Error()})
		return
	}

	iamClient := services.NewIAMClient()
	operatorID := middleware.GetUserID(ctx)
	userToken := services.ExtractUserToken(c)
	iamSub := resolveIAMSub(db, req.TargetUserID)
	if err := iamClient.UnbindUserFromOrganizationWithToken(userToken, iamSub, unbindOrg, operatorID); err != nil {
		now := time.Now()
		db.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
			"status":     removalStatusFailed,
			"decided_by": deciderID,
			"decided_at": now,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "解除成员失败（IAM）：" + err.Error()})
		return
	}

	now := time.Now()
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).
			Update("status", removalStatusApproved).Error; err != nil {
			return err
		}
		if req.SiteID != nil {
			if err := tx.Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, *req.SiteID, req.TargetUserID).
				Delete(&models.SiteMember{}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", tenantID, req.MerchantID, req.TargetUserID).
				Delete(&models.MerchantMember{}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
			"status":     removalStatusExecuted,
			"decided_by": deciderID,
			"decided_at": now,
		}).Error
	})
	if txErr != nil {
		// IAM 已解绑但本地事务失败：状态置 failed 并显式返回，交由运维按审计记录补偿。
		db.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
			"status":     removalStatusFailed,
			"decided_by": deciderID,
			"decided_at": now,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "执行移除失败（本地事务）：" + txErr.Error()})
		return
	}

	services.Notify(db, tenantID, req.RequesterID, "removal_approved", "成员移除申请已通过",
		"你提交的成员移除申请已获商户管理员批准并执行", req.ID, "member_removal_request", "info")

	auditUser := middleware.GetUserID(ctx)
	writeAuditLog(db, tenantID, auditUser, "member_removal_approved", "member_removal_request", req.ID,
		map[string]interface{}{
			"merchant_id":    req.MerchantID,
			"target_user_id": req.TargetUserID,
			"requester_id":   req.RequesterID,
			"unbind_org_id":  unbindOrg,
		})
	writeAuditLog(db, tenantID, auditUser, "member_removal_executed", "member_removal_request", req.ID,
		map[string]interface{}{
			"merchant_id":    req.MerchantID,
			"site_id":        req.SiteID,
			"target_user_id": req.TargetUserID,
			"iam_sub":        iamSub,
			"unbind_org_id":  unbindOrg,
			"decided_by":     deciderID,
		})

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": removalStatusExecuted}})
}

// RejectRemovalRequest POST /api/admin/removal-requests/:id/reject
// 商户管理员拒绝（原因必填），通知申请人。
func (h *MemberRemovalRequestHandler) RejectRemovalRequest(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireMerchantAdmin2394(c) {
		return
	}
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	requestID := c.Param("id")

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason is required"})
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > removalReasonMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason is required and must be at most 200 characters"})
		return
	}

	var req models.MemberRemovalRequest
	if err := db.Where("id = ? AND tenant_id = ?", requestID, tenantID).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "移除申请不存在"})
		return
	}
	if req.Status != removalStatusPending {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "该申请已被处理"})
		return
	}

	deciderID, err := middleware.LocalUserID(ctx, db)
	if err != nil || deciderID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无法识别审批人（本地用户不存在）"})
		return
	}

	now := time.Now()
	if err := db.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
		"status":        removalStatusRejected,
		"reject_reason": reason,
		"decided_by":    deciderID,
		"decided_at":    now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "拒绝申请失败：" + err.Error()})
		return
	}

	services.Notify(db, tenantID, req.RequesterID, "removal_rejected", "成员移除申请被拒绝",
		"你提交的成员移除申请未获批准："+reason, req.ID, "member_removal_request", "info")

	writeAuditLog(db, tenantID, middleware.GetUserID(ctx), "member_removal_rejected", "member_removal_request", req.ID,
		map[string]interface{}{
			"merchant_id":    req.MerchantID,
			"target_user_id": req.TargetUserID,
			"requester_id":   req.RequesterID,
			"reject_reason":  reason,
			"decided_by":     deciderID,
		})

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": removalStatusRejected}})
}

// CancelRemovalRequest POST /api/admin/removal-requests/:id/cancel
// 仅申请人本人可撤回仍处于 pending_approval 的申请。
func (h *MemberRemovalRequestHandler) CancelRemovalRequest(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	requestID := c.Param("id")

	currentUserID, err := middleware.LocalUserID(ctx, db)
	if err != nil || currentUserID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无法识别当前用户"})
		return
	}

	var req models.MemberRemovalRequest
	if err := db.Where("id = ? AND tenant_id = ?", requestID, tenantID).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40404, "message": "移除申请不存在"})
		return
	}
	if req.RequesterID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "仅申请人本人可撤回申请"})
		return
	}
	if req.Status != removalStatusPending {
		c.JSON(http.StatusConflict, gin.H{"code": 40910, "message": "该申请已被处理"})
		return
	}

	if err := db.Model(&models.MemberRemovalRequest{}).Where("id = ?", req.ID).
		Update("status", removalStatusCancel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "撤回申请失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": removalStatusCancel}})
}

// removalUnbindOrg2394 返回执行解绑的 IAM org：
// 网点申请 → 网点 org（成员在该 org 下的关系）；商户直属申请 → 商户 org。
func removalUnbindOrg2394(ctx context.Context, db *gorm.DB, tenantID string, req models.MemberRemovalRequest) (string, error) {
	if req.SiteID != nil {
		var site models.Site
		if err := db.Where("id = ? AND tenant_id = ?", *req.SiteID, tenantID).First(&site).Error; err != nil {
			return "", fmt.Errorf("site not found for removal request")
		}
		if site.OrgID == "" {
			return "", fmt.Errorf("site org is missing")
		}
		return site.OrgID, nil
	}
	var merchant models.Merchant
	if err := merchantConfigDB(ctx).Where("id = ?", req.MerchantID).First(&merchant).Error; err != nil {
		return "", fmt.Errorf("merchant not found for removal request")
	}
	if merchant.OrgID == "" {
		return "", fmt.Errorf("merchant org is missing")
	}
	return merchant.OrgID, nil
}

func isDuplicateKey2394(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "23505")
}
