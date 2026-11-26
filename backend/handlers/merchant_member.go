package handlers

import (
	"context"
	"errors"
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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MerchantMemberHandler struct{}

func NewMerchantMemberHandler() *MerchantMemberHandler {
	return &MerchantMemberHandler{}
}

// ListMembers GET /admin/merchants/:id/members - List merchant members
func (h *MerchantMemberHandler) ListMembers(c *gin.Context) {
	merchantID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// #2274：读取锚 = 商户 org（与 #2250 写入锚一致）
	merchant, ok := loadMerchantForAccess(db, tenantID, merchantID, c)
	if !ok {
		return
	}
	anchor := merchant.OrgID
	if anchor == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "商户 org 缺失"})
		return
	}

	// #2164: 服务端分页 + total
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)

	// #2546 D1/D3：默认口径仍是「商户管理员」（既有调用方行为不变）；
	// include=all 返回本商户全部 active 成员，供「转让所有权」候选使用。
	includeAll := strings.EqualFold(strings.TrimSpace(c.Query("include")), "all")
	// #2546 D1b：admin_uid 读取侧归一化到本地 users.id（历史行写的是 IAM 用户 id）。
	ownerUID, ownerAnchorPresent, ownerErr := merchantOwnerLocalUID(c.Request.Context(), db, merchant)
	if ownerErr != nil {
		// #2592：诊断只进日志，5xx 文案必须是固定字面量。
		log.Printf("[MerchantMemberList] owner resolution failed merchant=%s: %v", merchant.ID, ownerErr)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "读取商户所有人信息失败，请稍后重试"})
		return
	}
	// #2546 D14：转让门控真值 = 本商户 admin_uid 是否等于当前登录账号（同一本地 id 空间）。
	// 平台级 users.is_owner（JWT is_owner）与组织 OWNER 无关，禁止作为门控。
	viewerUID, viewerErr := viewerLocalUserID(c, db)
	if viewerErr != nil {
		log.Printf("[MerchantMemberList] viewer resolution failed merchant=%s: %v", merchant.ID, viewerErr)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "读取当前登录账号失败，请稍后重试"})
		return
	}
	viewerIsOwner := ownerUID != "" && viewerUID == ownerUID

	applyFilters := func(q *gorm.DB) *gorm.DB {
		q = q.Where("merchant_members.merchant_id = ? AND merchant_members.tenant_id = ?", merchantID, anchor)
		if includeAll {
			return q.Where("(merchant_members.status = ? OR merchant_members.status = ? OR merchant_members.status IS NULL)", "active", "")
		}
		return q.Where("merchant_members.role = ?", "merchant_admin")
	}

	// #2274：显式锚查询 → 关闭自动租户范围（平台管理员 tid 为共享 tenant）
	qdb := db.WithContext(database.IdentityCtx(c.Request.Context()))
	var total int64
	applyFilters(qdb.Table("merchant_members").
		Joins("JOIN users ON users.id = merchant_members.user_id")).
		Count(&total)

	var members []struct {
		UserID    string    `json:"user_id"`
		UserName  string    `json:"user_name"`
		UserEmail string    `json:"user_email"`
		Role      string    `json:"role"`
		Status    string    `json:"status"`
		IsOwner   bool      `json:"is_owner"`
		CreatedAt time.Time `json:"created_at"`
	}

	applyFilters(qdb.Table("merchant_members").
		Select("merchant_members.user_id, users.name as user_name, users.email as user_email, merchant_members.role, merchant_members.status, merchant_members.created_at").
		Joins("JOIN users ON users.id = merchant_members.user_id")).
		Order("merchant_members.created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&members)

	// #2546 D1：is_owner 只是响应级计算字段（不落库），本地权威是 merchants.admin_uid。
	for i := range members {
		members[i].IsOwner = ownerUID != "" && members[i].UserID == ownerUID
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":      members,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"owner_uid": ownerUID,
			// #2546 D1b：admin_uid 是否有值（区分「未锚定所有人」与「成员记录缺失」）。
			"owner_anchor_present": ownerAnchorPresent,
			// #2546 D14：前端「转让所有权」入口的唯一门控真值（本地 users.id 空间）。
			"viewer_uid":      viewerUID,
			"viewer_is_owner": viewerIsOwner,
		},
	})
}

// AddMember POST /admin/merchants/:id/members - Add member to merchant.
// The IAM binding org for a merchant is its own tenant_id.
func (h *MerchantMemberHandler) AddMember(c *gin.Context) {
	merchantID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	if !hasMerchantAccess(db, tenantID, merchantID, c) {
		return
	}

	var input struct {
		UserID         string                   `json:"user_id"`
		Role           string                   `json:"role"`
		UserIDs        []map[string]interface{} `json:"user_ids"`
		SkipActivation bool                     `json:"skip_activation"`
		// #2366：与网点成员共用 addMemberUser（密码模式字段同步，避免规则漂移）。
		NewUsers []addMemberUser `json:"new_users"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	var usersToProcess []map[string]interface{}

	if input.UserID != "" && len(input.UserIDs) == 0 {
		usersToProcess = []map[string]interface{}{
			{"user_id": input.UserID, "role": input.Role},
		}
	} else if len(input.UserIDs) > 0 {
		usersToProcess = input.UserIDs
	} else if len(input.NewUsers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Either user_id, user_ids, or new_users must be provided",
		})
		return
	}

	// #2250/#2274：商户归属锚 = org（显式范围查询，避免共享 tenant 下 404）
	merchant, ok := loadMerchantForAccess(db, tenantID, merchantID, c)
	if !ok {
		return
	}
	merchantAnchor := merchant.OrgID
	if merchantAnchor == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "商户 org 缺失"})
		return
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)
	operatorID := middleware.GetUserID(c.Request.Context())

	var directlyAdded []gin.H
	var bindErrors []gin.H
	var initialPasswords []gin.H
	var roleErrors []gin.H

	if len(input.NewUsers) > 0 {
		for _, nu := range input.NewUsers {
			// #2366：统一走共享构造函数（与网点成员同一套密码模式规则）。
			createReq, identifier, fail := newUserCreateReq(c, nu, "商户成员 - "+merchant.Name, operatorID, input.SkipActivation)
			if fail != nil {
				c.JSON(fail.Status, fail.Body)
				return
			}
			if createReq.Password != "" {
				log.Printf("[MerchantAddMember] direct activation with generated/initial password for %s", identifier)
			}
			userResult, err := iamClient.CreateOrGetUser(userToken, createReq)
			if err != nil {
				var conflictErr *services.UsernameConflictError
				if errors.As(err, &conflictErr) {
					var memberCount int64
					db.Model(&models.MerchantMember{}).
						Where("user_id = ? AND tenant_id = ?", conflictErr.UserID, merchantAnchor).
						Count(&memberCount)

					type merchantInfo struct {
						MerchantName string `json:"merchant_name"`
						Role         string `json:"role"`
					}
					var merchants []merchantInfo
					db.Table("merchant_members").
						Select("merchants.name AS merchant_name, merchant_members.role").
						Joins("JOIN merchants ON merchants.id = merchant_members.merchant_id").
						Where("merchant_members.user_id = ? AND merchant_members.tenant_id = ?", conflictErr.UserID, merchantAnchor).
						Scan(&merchants)

					c.JSON(http.StatusConflict, gin.H{
						"code": 40901,
						"data": gin.H{
							"conflicts": []gin.H{{
								"user_id":         conflictErr.UserID,
								"name":            conflictErr.Name,
								"email_masked":    services.MaskEmail(conflictErr.Email),
								"phone_masked":    services.MaskPhone(conflictErr.Phone),
								"username_masked": services.MaskUsername(conflictErr.Username),
								"matched_fields":  services.MatchIdentifier(conflictErr.Phone, conflictErr.Email, conflictErr.Username, "", conflictErr.Email, ""),
								"same_merchant":   memberCount > 0,
								"merchants":       merchants,
								"error":           conflictErr.Error(),
							}},
						},
					})
					return
				}
				log.Printf("[MerchantAddMember] Failed to create user %s: %v", createReq.Email, err)
				bindErrors = append(bindErrors, gin.H{
					"email":      createReq.Email,
					"identifier": identifier,
					"error":      err.Error(),
				})
				continue
			}
			if userResult.Conflict {
				log.Printf("[MerchantAddMember] User already exists in IAM: %s, proceeding with binding", userResult.UserID)

				var existingLocal models.User
				if err := db.Where("iam_sub = ?", userResult.UserID).First(&existingLocal).Error; err != nil {
					existingUser := userResult.ExistingUsers[0]
					localUser := models.User{
						ID:       userResult.UserID,
						IAMSub:   userResult.UserID,
						TenantID: merchantAnchor,
						OrgID:    merchantAnchor,
						Name:     existingUser.Name,
						Email:    existingUser.Email,
						Phone:    createReq.Phone,
						Role:     "site_member",
						Status:   "active",
					}
					if err := db.Create(&localUser).Error; err != nil {
						log.Printf("[MerchantAddMember] Failed to create local user for existing IAM user %s: %v", userResult.UserID, err)
					}
				} else {
					existingUser := userResult.ExistingUsers[0]
					updates := map[string]interface{}{
						"deleted_at": nil,
						"name":       existingUser.Name,
						"email":      existingUser.Email,
						"status":     "active",
					}
					if existingLocal.Phone == "" {
						updates["phone"] = createReq.Phone
					}
					if err := db.Model(&models.User{}).
						Where("id = ? AND tenant_id = ?", existingLocal.ID, tenantID).
						Updates(updates).Error; err != nil {
						log.Printf("[MerchantAddMember] Failed to restore local user cache %s: %v", existingLocal.ID, err)
					}
				}

				role := nu.Role
				if role == "" {
					role = "site_member"
				}
				usersToProcess = append(usersToProcess, map[string]interface{}{
					"user_id": userResult.UserID,
					"role":    role,
				})
				continue
			}

			if createReq.Password != "" {
				// #2366：identifier = 实际登录名；email 保留兼容旧前端。
				initialPasswords = append(initialPasswords, gin.H{
					"identifier": identifier,
					"email":      createReq.Email,
					"password":   createReq.Password,
				})
			}

			localUser := models.User{
				ID:       userResult.UserID,
				IAMSub:   userResult.UserID,
				TenantID: merchantAnchor,
				OrgID:    merchantAnchor,
				Name:     createReq.Name,
				Email:    createReq.Email,
				Phone:    createReq.Phone,
				Role:     "site_member",
				Status:   "active",
			}
			if err := db.Create(&localUser).Error; err != nil {
				log.Printf("[MerchantAddMember] Failed to create local user %s: %v", userResult.UserID, err)
			}
			role := nu.Role
			if role == "" {
				role = "site_member"
			}
			usersToProcess = append(usersToProcess, map[string]interface{}{
				"user_id": userResult.UserID,
				"role":    role,
			})
		}
	}

	for _, userEntry := range usersToProcess {
		userID, ok := userEntry["user_id"].(string)
		if !ok || userID == "" {
			continue
		}

		role := input.Role
		if r, ok := userEntry["role"].(string); ok && r != "" {
			role = r
		}

		// #2546 D2：所有权只能转让，不能追加 —— 显式 OWNER 请求直接拒绝（不调 IAM、
		// 不写本地），否则 IAM 侧必然 400（组织已有 OWNER）。
		if isOwnerRoleRequest(role) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "所有人不能通过添加成员设置，请使用转让所有权",
			})
			return
		}

		var count int64
		db.Model(&models.MerchantMember{}).
			Where("merchant_id = ? AND user_id = ?", merchantID, userID).
			Count(&count)
		if count > 0 {
			continue
		}

		// #2428 D3：跨租户禁止 direct bind；只能走加入邀请。
		var targetUser models.User
		if err := db.Where("id = ? AND deleted_at IS NULL", userID).First(&targetUser).Error; err == nil && targetUser.TenantID != tenantID {
			c.JSON(http.StatusConflict, gin.H{
				"code":    40911,
				"message": "该用户属于其他商户，请发送加入邀请，由本人确认加入",
				"data":    gin.H{"need_invite": true},
			})
			return
		}

		normalizedRole := normalizeRole(role)
		iamRole := merchantMemberIAMRole(normalizedRole)
		templateCode := normalizedRole

		// Bind to the merchant's own org (merchant.OrgID) — NOT the namespace
		// tenant id. site_member.go binds site members to site.OrgID; the
		// equivalent here is merchant.OrgID, so IAM org bindings stay
		// consistent with existing members (#1718/#1715 backfill).
		bindOrg := merchant.OrgID
		if bindOrg == "" {
			bindOrg = tenantID
		}
		if err := iamClient.BindUserToOrganizationWithToken(userToken, userID, bindOrg, iamRole, operatorID); err != nil {
			log.Printf("[MerchantAddMember] IAM BindUser failed for user %s to org %s: %v", userID, bindOrg, err)
			bindErrors = append(bindErrors, gin.H{
				"user_id": userID,
				"error":   err.Error(),
			})
			continue
		}
		nsID := middleware.GetNamespaceID(c.Request.Context())
		if templates, err := iamClient.ListRoleTemplates(nsID); err == nil {
			for _, t := range templates {
				if t.Code == templateCode {
					if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, userID, bindOrg, t.Code); err != nil {
						log.Printf("[MerchantAddMember] AssignRoleTemplate failed for user %s code %s: %v", userID, templateCode, err)
						roleErrors = append(roleErrors, gin.H{
							"user_id":       userID,
							"template_code": templateCode,
							"error":         err.Error(),
						})
					}
					break
				}
			}
		} else {
			log.Printf("[MerchantAddMember] ListRoleTemplates failed: %v", err)
			roleErrors = append(roleErrors, gin.H{
				"error": "failed to list role templates: " + err.Error(),
			})
		}

		member := models.MerchantMember{
			TenantID:   merchantAnchor,
			MerchantID: merchantID,
			UserID:     userID,
			Role:       role,
		}
		result := db.Create(&member)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": fmt.Sprintf("Failed to add member %s: %s", userID, result.Error.Error()),
			})
			return
		}

		directlyAdded = append(directlyAdded, gin.H{
			"user_id": userID,
			"role":    role,
		})
	}

	if len(directlyAdded) == 0 && len(bindErrors) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "No valid users to add",
		})
		return
	}

	responseData := gin.H{
		"merchant_id":    merchantID,
		"directly_added": directlyAdded,
	}
	if len(bindErrors) > 0 {
		responseData["bind_errors"] = bindErrors
	}
	if len(initialPasswords) > 0 {
		responseData["initial_passwords"] = initialPasswords
	}
	if len(roleErrors) > 0 {
		responseData["role_errors"] = roleErrors
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": responseData,
	})
}

// UpdateMemberRole PUT /admin/merchants/:id/members/:uid - Update member role
func (h *MerchantMemberHandler) UpdateMemberRole(c *gin.Context) {
	merchantID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	var input struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	// #2250（返工）：成员与 IAM 组织统一锚定商户 org（共享 tenant 不得当组织用）
	merchant, ok := loadMerchantForAccess(db, tenantID, merchantID, c)
	if !ok {
		return
	}
	anchor := merchant.OrgID
	if anchor == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "merchant org anchor is missing"})
		return
	}

	// #2546 D2：所有权只能转让 —— 显式 OWNER 请求、以及改当前所有人角色都必须拒绝，
	// 且不调 IAM（IAM 侧会 400/403，本地不得先行写入）。
	if isOwnerRoleRequest(input.Role) || isOwnerRoleRequest(normalizeRole(input.Role)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "所有人不能通过改角色设置，请使用转让所有权",
		})
		return
	}
	// #2546 D1b/D14：owner 比较统一在本地 users.id 空间（历史 admin_uid 可能是 IAM id）。
	ownerUID, _, ownerErr := merchantOwnerLocalUID(c.Request.Context(), db, merchant)
	if ownerErr != nil {
		// #2592：诊断只进日志，5xx 文案必须是固定字面量。
		log.Printf("[MerchantMemberUpdateRole] owner resolution failed merchant=%s user=%s: %v", merchant.ID, userID, ownerErr)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "读取商户所有人信息失败，请稍后重试"})
		return
	}
	if ownerUID != "" && userID == ownerUID {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "所有人不能改角色，请使用转让所有权",
		})
		return
	}

	var iamUser models.User
	if err := db.Where("id = ?", userID).First(&iamUser).Error; err == nil && iamUser.IAMSub != "" {
		normalizedRole := normalizeRole(input.Role)
		iamRole := merchantMemberIAMRole(normalizedRole)
		templateCode := normalizedRole

		iamClient := services.NewIAMClient()
		userToken := services.ExtractUserToken(c)

		// #2250（返工）：IAM 调用失败必须显式返回（不得本地改角色而 IAM 漂移）
		if normalizedRole == "merchant_admin" {
			if bindErr := iamClient.BindUserToOrganizationWithToken(userToken, iamUser.IAMSub, anchor, iamRole, middleware.GetUserID(c.Request.Context())); bindErr != nil {
				log.Printf("[MerchantUpdateRole] BindUser failed for %s: %v", iamUser.IAMSub, bindErr)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "更新成员角色失败，请稍后重试"})
				return
			}
		} else {
			if demoteErr := iamClient.UpdateUserRoleInOrgWithToken(userToken, anchor, iamUser.IAMSub, iamRole); demoteErr != nil {
				log.Printf("[MerchantUpdateRole] UpdateRole failed for %s: %v", iamUser.IAMSub, demoteErr)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "更新成员角色失败，请稍后重试"})
				return
			}
		}

		nsID := middleware.GetNamespaceID(c.Request.Context())
		templates, err := iamClient.ListRoleTemplates(nsID)
		if err != nil {
			log.Printf("[MerchantUpdateRole] ListRoleTemplates failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "更新成员角色失败，请稍后再试"})
			return
		}
		for _, t := range templates {
			if t.Code == templateCode {
				if err := iamClient.AssignRoleTemplateToUserWithToken(userToken, iamUser.IAMSub, anchor, t.Code); err != nil {
					log.Printf("[MerchantUpdateRole] AssignRoleTemplate failed: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "更新成员角色失败，请稍后再试"})
					return
				}
				break
			}
		}
	}

	result := db.Model(&models.MerchantMember{}).
		Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", anchor, merchantID, userID).
		Update("role", input.Role)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update member role: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"merchant_id": merchantID,
			"user_id":     userID,
			"new_role":    input.Role,
		},
	})
}

// RemoveMember DELETE /admin/merchants/:id/members/:uid - Remove member from merchant
func (h *MerchantMemberHandler) RemoveMember(c *gin.Context) {
	merchantID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// #2250（返工）：成员行与 IAM 解绑统一锚定商户 org（与 AddMember/门控同口径）
	merchant, ok := loadMerchantForAccess(db, tenantID, merchantID, c)
	if !ok {
		return
	}
	anchor := merchant.OrgID
	if anchor == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "merchant org anchor is missing"})
		return
	}

	// Last-admin protection (#1718): a merchant must keep at least one
	// responsible person (merchant_admin). Refuse removal when the target is a
	// merchant_admin and it is the last one — server-side authority, the
	// frontend disable is only UX.
	var target models.MerchantMember
	if err := db.Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", anchor, merchantID, userID).
		First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Member not found"})
		return
	}
	// #2546：所有人（OWNER）不能移除，只能转让所有权 —— 排在既有「最后负责人」
	// 兜底之前，且不调 IAM（IAM 侧同样会 403）。
	// #2546 D1b/D14：owner 比较统一在本地 users.id 空间（历史 admin_uid 可能是 IAM id）。
	ownerUID, _, ownerErr := merchantOwnerLocalUID(c.Request.Context(), db, merchant)
	if ownerErr != nil {
		// #2592：诊断只进日志，5xx 文案必须是固定字面量。
		log.Printf("[MerchantMemberRemove] owner resolution failed merchant=%s user=%s: %v", merchant.ID, userID, ownerErr)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "读取商户所有人信息失败，请稍后重试"})
		return
	}
	if ownerUID != "" && userID == ownerUID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "所有人不能移除，只能转让所有权"})
		return
	}
	if target.Role == "merchant_admin" {
		var adminCount int64
		db.Model(&models.MerchantMember{}).
			Where("tenant_id = ? AND merchant_id = ? AND role = ?", anchor, merchantID, "merchant_admin").
			Count(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40009, "message": "不能删除最后一个负责人"})
			return
		}
	}

	// #2114：解绑商户关系用 merchant.OrgID（≠ tenant_id，实测二者不同），
	// 传 IAM sub（非本地 id），失败显式返回（不静默）。绑定侧同样用 merchant.OrgID。
	iamClient := services.NewIAMClient()
	operatorID := middleware.GetUserID(c.Request.Context())
	memberToken := services.ExtractUserToken(c)
	iamSub := resolveIAMSub(db, userID)
	if err := iamClient.UnbindUserFromOrganizationWithToken(memberToken, iamSub, anchor, operatorID); err != nil {
		log.Printf("[MerchantMemberRemove] IAM unbind failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "解除成员失败，请稍后再试"})
		return
	}

	result := db.Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", anchor, merchantID, userID).
		Delete(&models.MerchantMember{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to remove member: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	// #2114：仅当该用户在本商户「无任何成员身份」（网点/商户/师傅）时才软删缓存 users 行。
	if !hasLocalTenantMembership(db, anchor, userID) {
		now := time.Now()
		if err := db.Model(&models.User{}).
			Where("id = ? AND tenant_id = ?", userID, anchor).
			Update("deleted_at", now).Error; err != nil {
			log.Printf("[MerchantRemoveMember] Failed to soft-delete cached user %s: %v", userID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Member removed successfully",
	})
}

// nilUUID2546 is the all-zero UUID Postgres/GORM uses for "no user".
const nilUUID2546 = "00000000-0000-0000-0000-000000000000"

// merchantOwnerUID returns the merchant's single OWNER pointer as stored in
// merchants.admin_uid (#2546 D1). An empty or all-zero UUID means the merchant
// is not anchored locally (see #2553 for stored-data reconciliation), and
// callers must not guess an owner.
//
// The returned value is in the *stored* ID space; use merchantOwnerLocalUID for
// anything that compares against local users.id (D1b/D14).
func merchantOwnerUID(merchant models.Merchant) string {
	uid := strings.TrimSpace(merchant.AdminUID)
	if uid == "" || uid == nilUUID2546 {
		return ""
	}
	return uid
}

// merchantOwnerLocalUID normalizes merchants.admin_uid into the local users.id
// space (#2546 D1b/D14).
//
// Stored rows have two historical shapes: the local users.id (written by
// CreateMerchant and by ownership transfers) and the IAM user id (old rows
// written before #2250 anchored writes). Read paths must accept both:
//   - admin_uid matches users.id       -> use it directly
//   - admin_uid matches users.iam_sub  -> resolve to that row's local id
//
// Returns (localUID, anchorPresent). localUID is "" when the pointer holds a
// value but does not resolve; anchorPresent reports whether admin_uid holds a
// value at all, so callers can distinguish "未锚定所有人" (no pointer) from
// "成员记录缺失" (pointer present but unresolvable → contact the platform).
// Stored-data convergence stays in #2553 (read-only ops checklist).
func merchantOwnerLocalUID(ctx context.Context, db *gorm.DB, merchant models.Merchant) (string, bool, error) {
	stored := merchantOwnerUID(merchant)
	if stored == "" {
		return "", false, nil
	}
	idb := db.WithContext(database.IdentityCtx(ctx))
	// Find (not First) keeps a "no such row" from being logged as an error: both
	// shapes are expected during the #2553 convergence window.
	var byID []models.User
	if err := idb.Select("id").Where("id = ?", stored).Limit(1).Find(&byID).Error; err != nil {
		return "", true, fmt.Errorf("查询 users.id=%s: %w", stored, err)
	}
	if len(byID) == 1 && byID[0].ID != "" {
		return byID[0].ID, true, nil
	}
	var bySub []models.User
	if err := idb.Select("id").Where("iam_sub = ?", stored).Limit(1).Find(&bySub).Error; err != nil {
		return "", true, fmt.Errorf("查询 users.iam_sub=%s: %w", stored, err)
	}
	if len(bySub) == 1 && bySub[0].ID != "" {
		return bySub[0].ID, true, nil
	}
	return "", true, nil
}

// viewerLocalUserID resolves the logged-in account's local users.id (#2546
// D14). Both the ownership pointer and the viewer gate must live in the same
// ID space, otherwise a real OWNER can never match. iam_sub is globally unique,
// so the lookup runs without tenant scoping (a platform admin's cached row may
// live under the platform tenant while the request is scoped to a merchant).
func viewerLocalUserID(c *gin.Context, db *gorm.DB) (string, error) {
	ctx := c.Request.Context()
	return middleware.LocalUserID(ctx, db.WithContext(database.IdentityCtx(ctx)))
}

// merchantMemberIAMRole maps a tuneloop business role to the IAM relation role
// used when binding a merchant member.
//
// merchant_admin binds ADMIN: IAM allows at most one OWNER per organization and
// ownership only moves through transfer-ownership (#2546 D2). toIAMRole keeps its
// merchant_admin -> OWNER mapping for the creation/activation paths (#2540).
func merchantMemberIAMRole(businessRole string) string {
	if businessRole == "merchant_admin" {
		return "ADMIN"
	}
	return toIAMRole(businessRole)
}

// isOwnerRoleRequest reports whether the caller explicitly asked for OWNER,
// either with the IAM role code or the business code (case-insensitive).
func isOwnerRoleRequest(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "owner")
}

// ownershipTransferError carries the HTTP status/code/message a caller must
// return for a rejected ownership transfer.
type ownershipTransferError struct {
	Status  int
	Code    int
	Message string
}

func (e *ownershipTransferError) Error() string { return e.Message }

// mapTransferOwnershipIAMError maps beaconiam's transfer-ownership status codes
// to tuneloop's contract (#2546 D5). IAM failures are never downgraded to 200.
func mapTransferOwnershipIAMError(iamErr *services.IAMTransferOwnershipError) (int, int, string) {
	switch iamErr.StatusCode {
	case http.StatusForbidden:
		return http.StatusForbidden, 40303, "仅当前所有人可转让所有权：" + iamErr.Message
	case http.StatusNotFound:
		return http.StatusNotFound, 40400, "转让所有权失败（IAM）：" + iamErr.Message
	case http.StatusConflict:
		return http.StatusConflict, 40900, "该商户当前没有所有人，请走修复流程：" + iamErr.Message
	case http.StatusBadRequest:
		return http.StatusBadRequest, 40002, "转让所有权失败（IAM）：" + iamErr.Message
	default:
		return http.StatusBadGateway, 50200, "转让所有权失败（IAM）：" + iamErr.Message
	}
}

// transferMerchantOwnership performs the only allowed ownership change: move the
// single OWNER to another active member of the same merchant.
//
// Order (#2546 D1/D5): local preflight (target must be an active member, not the
// current owner, and must have an IAM account) -> IAM transfer-ownership -> local
// pointer move (merchants.admin_uid + admin_pending=false) and business role
// promotion. IAM is the authority, so a failed transfer leaves the local pointer
// untouched and is surfaced to the caller verbatim.
//
// Returns the previous owner's local user id ("" when the merchant was not
// anchored locally).
func transferMerchantOwnership(c *gin.Context, db *gorm.DB, merchant models.Merchant, targetUserID string) (string, error) {
	// #2546 D1b/D14：previous owner 归一化到本地 users.id —— 与目标（members.user_id）
	// 以及 viewer 门控使用同一 ID 空间；历史行把 admin_uid 存成 IAM id 也能正确判重。
	previousOwnerID, _, ownerErr := merchantOwnerLocalUID(c.Request.Context(), db, merchant)
	if ownerErr != nil {
		// #2592：诊断只进日志，5xx 文案必须是固定字面量。
		log.Printf("[MerchantTransferOwnership] owner resolution failed merchant=%s target_user=%s: %v", merchant.ID, targetUserID, ownerErr)
		return "", &ownershipTransferError{
			Status:  http.StatusInternalServerError,
			Code:    50000,
			Message: "读取商户所有人信息失败，请稍后重试",
		}
	}

	if targetUserID == "" || targetUserID == nilUUID2546 {
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusBadRequest,
			Code:    40002,
			Message: "所有人不能清空，只能转让所有权",
		}
	}
	if merchant.OrgID == "" {
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusInternalServerError,
			Code:    50000,
			Message: "商户 org 缺失",
		}
	}
	if previousOwnerID != "" && targetUserID == previousOwnerID {
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusBadRequest,
			Code:    40002,
			Message: "目标用户已是本商户所有人",
		}
	}

	// Target must be an active member of THIS merchant (no cross-merchant or
	// platform-user leakage: non-members get the same 404).
	var target models.MerchantMember
	err := db.Where("tenant_id = ? AND merchant_id = ? AND user_id = ? AND (status = ? OR status = ? OR status IS NULL)",
		merchant.OrgID, merchant.ID, targetUserID, "active", "").
		First(&target).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return previousOwnerID, &ownershipTransferError{
				Status:  http.StatusNotFound,
				Code:    40400,
				Message: "目标用户不是本商户的成员",
			}
		}
		log.Printf("[MerchantTransferOwnership] target member lookup failed merchant=%s target_user=%s: %v", merchant.ID, targetUserID, err)
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusInternalServerError,
			Code:    50000,
			Message: "查询目标成员失败，请稍后重试",
		}
	}

	var targetUser models.User
	if err := db.Where("id = ?", targetUserID).First(&targetUser).Error; err != nil || targetUser.IAMSub == "" {
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusBadRequest,
			Code:    40002,
			Message: "目标用户缺少 IAM 账号，无法转让所有权",
		}
	}

	iamClient := services.NewIAMClient()
	userToken := services.ExtractUserToken(c)
	if err := iamClient.TransferOwnershipWithToken(userToken, merchant.OrgID, targetUser.IAMSub); err != nil {
		var iamErr *services.IAMTransferOwnershipError
		if errors.As(err, &iamErr) {
			status, code, message := mapTransferOwnershipIAMError(iamErr)
			return previousOwnerID, &ownershipTransferError{Status: status, Code: code, Message: message}
		}
		log.Printf("[MerchantTransferOwnership] IAM transfer failed for merchant %s org %s: %v", merchant.ID, merchant.OrgID, err)
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusBadGateway,
			Code:    50200,
			Message: "转让所有权失败（IAM），请稍后重试",
		}
	}

	// IAM committed the transfer: mirror it locally in ONE transaction (#2546 D16).
	// admin_uid and the target's business role must both land, otherwise roll back
	// and surface 500/50000. A log-only half-sync would leave the local pointer on a
	// member row that is not merchant_admin (owner card would read "未锚定所有人").
	syncErr := db.Transaction(func(tx *gorm.DB) error {
		sync := tx.Model(&models.Merchant{}).
			Where("id = ? AND tenant_id = ?", merchant.ID, merchant.TenantID).
			Updates(map[string]interface{}{"admin_uid": targetUserID, "admin_pending": false})
		if sync.Error != nil {
			return fmt.Errorf("商户所有人指针更新失败: %w", sync.Error)
		}
		if sync.RowsAffected != 1 {
			return fmt.Errorf("商户所有人指针未更新（rows=%d）", sync.RowsAffected)
		}
		// Business role stays merchant_admin for both OWNER and ADMIN (#2546 D1); the
		// target may have been a direct staff member, so promote its row explicitly.
		promote := tx.Model(&models.MerchantMember{}).
			Where("tenant_id = ? AND merchant_id = ? AND user_id = ?", merchant.OrgID, merchant.ID, targetUserID).
			Update("role", "merchant_admin")
		if promote.Error != nil {
			return fmt.Errorf("目标成员业务角色提升失败: %w", promote.Error)
		}
		if promote.RowsAffected != 1 {
			return fmt.Errorf("目标成员业务角色未提升（rows=%d）", promote.RowsAffected)
		}
		return nil
	})
	if syncErr != nil {
		log.Printf("[MerchantTransferOwnership] ERROR: IAM transferred merchant %s org %s (previous owner %s -> target user %s) but the local transaction rolled back: %v",
			merchant.ID, merchant.OrgID, previousOwnerID, targetUserID, syncErr)
		return previousOwnerID, &ownershipTransferError{
			Status:  http.StatusInternalServerError,
			Code:    50000,
			Message: "IAM 已转让，但本地同步失败，请联系平台人工核对",
		}
	}

	return previousOwnerID, nil
}

// TransferOwnership POST /merchants/:id/transfer-ownership - transfer the single
// merchant OWNER to another active member (#2546).
func (h *MerchantMemberHandler) TransferOwnership(c *gin.Context) {
	merchantID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	var input struct {
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Invalid input: " + err.Error()})
		return
	}
	targetUserID := strings.TrimSpace(input.UserID)
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_id 不能为空"})
		return
	}
	if _, err := uuid.Parse(targetUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user_id 必须是合法的 UUID"})
		return
	}

	merchant, ok := loadMerchantForAccess(db, tenantID, merchantID, c)
	if !ok {
		return
	}

	previousOwnerID, err := transferMerchantOwnership(c, db, merchant, targetUserID)
	if err != nil {
		var transferErr *ownershipTransferError
		if errors.As(err, &transferErr) {
			c.JSON(transferErr.Status, gin.H{"code": transferErr.Code, "message": transferErr.Message})
			return
		}
		// #2592：非类型化转让错误（防御分支）同样只回固定文案。
		log.Printf("[MerchantTransferOwnership] transfer failed merchant=%s target_user=%s: %v", merchantID, targetUserID, err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "转让所有权失败（IAM），请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"merchant_id":       merchantID,
			"previous_owner_id": previousOwnerID,
			"new_owner_id":      targetUserID,
		},
	})
}

// hasMerchantAccess checks the merchant belongs to the requesting tenant.
func hasMerchantAccess(db *gorm.DB, tenantID, merchantID string, c *gin.Context) bool {
	var count int64
	idb := db.WithContext(database.IdentityCtx(c.Request.Context()))
	// #2250：商户归属以 org 为准（tenant_id 可能为多商户共享值）
	result := idb.Model(&models.Merchant{}).
		Where("id = ? AND (tenant_id = ? OR org_id = ?)", merchantID, tenantID, tenantID).
		Count(&count)

	// #2274：平台管理员（命名空间主组织）按 id 放行；IAM 不可用时不放权
	if (result.Error != nil || count == 0) && isPlatformOrg2247(services.NewIAMClient(), middleware.GetOrgID(c.Request.Context())) {
		result = idb.Model(&models.Merchant{}).Where("id = ?", merchantID).Count(&count)
	}

	if result.Error != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Merchant not found",
		})
		return false
	}
	return true
}

// loadMerchantForAccess performs the same org-or-tenant access check as
// hasMerchantAccess and returns the merchant row so callers can use the
// merchant org as the single anchor (#2250). Writes the 403 response itself.
func loadMerchantForAccess(db *gorm.DB, tenantID, merchantID string, c *gin.Context) (models.Merchant, bool) {
	var merchant models.Merchant
	idb := db.WithContext(database.IdentityCtx(c.Request.Context()))
	err := idb.Where("id = ? AND (tenant_id = ? OR org_id = ?)", merchantID, tenantID, tenantID).
		First(&merchant).Error
	// #2274：平台管理员按 id 放行；IAM 不可用时不放权
	if err != nil && isPlatformOrg2247(services.NewIAMClient(), middleware.GetOrgID(c.Request.Context())) {
		err = idb.Where("id = ?", merchantID).First(&merchant).Error
	}
	if err != nil {
		// #2276：不同租户 → 404（不泄漏）；同一 shared tenant 的其他商户 org → 403（#2274 契约）
		var target models.Merchant
		if idb.Where("id = ?", merchantID).First(&target).Error == nil {
			var sameTenantMerchant int64
			idb.Model(&models.Merchant{}).
				Where("tenant_id = ? AND org_id = ?", target.TenantID, tenantID).
				Count(&sameTenantMerchant)
			if target.TenantID == tenantID || sameTenantMerchant > 0 {
				c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
				return models.Merchant{}, false
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "merchant not found"})
		return models.Merchant{}, false
	}
	return merchant, true
}
