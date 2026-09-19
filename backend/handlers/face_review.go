package handlers

import (
	"context"
	"encoding/json"
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

// FaceReviewHandler (#1791 T3): 实名核身人工审核队列。
// 权限：平台员工/系统管理员（SysPermUserUpdate），非 org 隔离（全用户可见）。
type FaceReviewHandler struct{}

// platformDB returns a DB instance exempt from tenant query scoping.
// Face review is a platform-level feature that must see ALL users including
// customers with empty tenant_id (00000000-...). Without this, the global
// addTenantScope callback filters out zero-tenant users (bug #1812).
func (h *FaceReviewHandler) platformDB(c *gin.Context) *gorm.DB {
	ctx := context.WithValue(c.Request.Context(), database.TenantIDKey, "")
	return database.GetDB().WithContext(ctx)
}

// faceReviewItem 审核队列条目（含用户证件照三张 + 自拍素材 URL + 实名信息采集状态）。
type faceReviewItem struct {
	BatchID         string   `json:"batch_id"`
	UserID          string   `json:"user_id"`
	UserName        string   `json:"user_name"`
	IDPhotos        []string `json:"id_photos"`
	SelfieURLs      []string `json:"selfie_urls"`
	SubmittedAt     string   `json:"submitted_at"`
	Kind            string   `json:"kind"`                        // #1924: registration / second_doc
	HasSecondDoc    bool     `json:"has_second_doc"`              // #1924: 用户已提交第二证件
	OtherVerified   bool     `json:"id_photo_other_verified"`     // #1924: 第二证件认证态
	OtherType       string   `json:"id_photo_other_type"`         // #1924: 审核员指定类型（未定时为空）
	FaceVerified    bool     `json:"face_verified"`               // #1924: 身份证实名是否已认证
	IDInfoCollected bool     `json:"id_info_collected"`           // #1822: 实名信息已采录
	RealName        string   `json:"real_name,omitempty"`         // #1822: 已采录时展示姓名
	IDCardNoMasked  string   `json:"id_card_no_masked,omitempty"` // #1822: 身份证号后四位掩码
	IDCardExpire    string   `json:"id_card_expire,omitempty"`    // #1822: 已采录摘要（弹窗只读核对）
	IDCardAuthority string   `json:"id_card_authority,omitempty"` // #1822: 已采录摘要
	IDCardAddress   string   `json:"id_card_address,omitempty"`   // #1822: 已采录摘要
}

// userHasCoreIDInfo 判断用户是否已采录核心实名信息（真实姓名 + 身份证号，#1822）。
// 仅这两项被视为「采集完成」门槛：expire/authority/address 可后补（用户管理
// id-card 入口维护），但姓名与号码缺失则无法做证/人核验。
func userHasCoreIDInfo(u *models.User) bool {
	return u.RealName != nil && *u.RealName != "" && u.IdCardNo != nil && *u.IdCardNo != ""
}

// resolveSelfieURL 组装核身素材（自拍/证件照）访问 URL。
// #1993：统一经 MediaStorage.GetURL——OSS 模式下私有前缀（face_captures/）返回
// 签名 URL，本地模式返回 /uploads/media/<key>；防历史双前缀脏值 404（#1807）。
func resolveSelfieURL(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key
	}
	// 先归一化（兼容历史脏值 /uploads/media//uploads/media/...，#1807/#1814），
	// 再经 MediaStorage（OSS 私有前缀 → 签名 URL；本地 → 相对路径）。
	clean := normalizeMediaKey(key)
	url, err := services.NewMediaStorage().GetURL(ctx, clean)
	if err != nil || url == "" {
		return mediaURLPrefix + clean
	}
	return url
}

// Queue handles GET /admin/face-review/queue.
func (h *FaceReviewHandler) Queue(c *gin.Context) {
	db := h.platformDB(c)

	var batches []models.FaceCaptureBatch
	q := db.Where("status = ?", "pending")
	// #1813: optional user_id filter for single-user focus from user management detail.
	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if err := q.Order("submitted_at ASC").Find(&batches).Error; err != nil {
		log.Printf("[FaceReview] queue query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to load review queue"})
		return
	}

	items := make([]faceReviewItem, 0, len(batches))
	for _, b := range batches {
		var user models.User
		if err := db.Select("id, name, id_photo_front, id_photo_back, id_photo_other, real_name, id_card_no, id_card_expire, id_card_authority, id_card_address, id_photo_other_verified, id_photo_other_type, face_verified").
			Where("id = ?", b.UserID).First(&user).Error; err != nil {
			continue // 用户不存在（可能已删除）跳过
		}
		item := faceReviewItem{
			BatchID:       b.ID,
			UserID:        user.ID,
			UserName:      user.Name,
			SubmittedAt:   b.SubmittedAt.Format(time.RFC3339),
			Kind:          b.Kind,
			HasSecondDoc:  user.IdPhotoOther != nil && *user.IdPhotoOther != "",
			OtherVerified: user.IdPhotoOtherVerified,
			FaceVerified:  user.FaceVerified,
		}
		// #1822: 实名信息采集状态（已采录时展示脱敏摘要，审核员无需重复抄录）。
		if userHasCoreIDInfo(&user) {
			item.IDInfoCollected = true
			if user.RealName != nil {
				item.RealName = *user.RealName
			}
			if user.IdCardNo != nil {
				cardNo := *user.IdCardNo
				if len(cardNo) > 4 {
					item.IDCardNoMasked = "****" + cardNo[len(cardNo)-4:]
				} else {
					item.IDCardNoMasked = cardNo
				}
			}
			if user.IdCardExpire != nil {
				item.IDCardExpire = *user.IdCardExpire
			}
			if user.IdCardAuthority != nil {
				item.IDCardAuthority = *user.IdCardAuthority
			}
			if user.IdCardAddress != nil {
				item.IDCardAddress = *user.IdCardAddress
			}
		}
		// 证件照三张（隐私边界：仅审核用，不返回身份证号）。
		if user.IdPhotoFront != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(c.Request.Context(), *user.IdPhotoFront))
		}
		if user.IdPhotoBack != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(c.Request.Context(), *user.IdPhotoBack))
		}
		if user.IdPhotoOther != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(c.Request.Context(), *user.IdPhotoOther))
		}
		// 自拍素材（media_assets source_id=batch_id，#1790 M5 关联键）。
		var assets []models.MediaAsset
		db.Where("source_id = ? AND source_type = ?", b.ID, "face_capture").
			Order("created_at ASC").Find(&assets)
		for _, a := range assets {
			item.SelfieURLs = append(item.SelfieURLs, resolveSelfieURL(c.Request.Context(), a.StorageKey))
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": items, "total": len(items)},
	})
}

// UserBatches handles GET /admin/face-review/user/:userId.
// Returns ALL batches of one user (pending/approved/rejected history) with
// selfie material URLs — the user-detail dialog module 2 data source (#1810).
type faceReviewBatchItem struct {
	BatchID      string   `json:"batch_id"`
	Kind         string   `json:"kind"`
	Status       string   `json:"status"`
	RejectReason string   `json:"reject_reason,omitempty"`
	SelfieURLs   []string `json:"selfie_urls"`
	SubmittedAt  string   `json:"submitted_at"`
	ReviewedAt   string   `json:"reviewed_at,omitempty"`
}

func (h *FaceReviewHandler) UserBatches(c *gin.Context) {
	db := h.platformDB(c)
	userID := c.Param("userId")

	var user models.User
	if err := db.Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	var batches []models.FaceCaptureBatch
	if err := db.Where("user_id = ?", userID).
		Order("submitted_at DESC").Find(&batches).Error; err != nil {
		log.Printf("[FaceReview] user batches query failed for %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to load user batches"})
		return
	}

	items := make([]faceReviewBatchItem, 0, len(batches))
	for _, b := range batches {
		item := faceReviewBatchItem{
			BatchID:     b.ID,
			Kind:        b.Kind,
			Status:      b.Status,
			SubmittedAt: b.SubmittedAt.Format(time.RFC3339),
		}
		if b.RejectReason != nil {
			item.RejectReason = *b.RejectReason
		}
		if b.ReviewedAt != nil {
			item.ReviewedAt = b.ReviewedAt.Format(time.RFC3339)
		}
		var assets []models.MediaAsset
		db.Where("source_id = ? AND source_type = ?", b.ID, "face_capture").
			Order("created_at ASC").Find(&assets)
		for _, a := range assets {
			item.SelfieURLs = append(item.SelfieURLs, resolveSelfieURL(c.Request.Context(), a.StorageKey))
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": items, "total": len(items)},
	})
}

// Review handles POST /admin/face-review/:batchId.
// action: approve → users.face_verified=true + face_verify_method=manual + 批次 approved
//
//	reject → 批次 rejected + reason
//
// 留痕双写（R2 M4）：face_capture_batches.reviewed_by/at + audit_logs。
func (h *FaceReviewHandler) Review(c *gin.Context) {
	ctx := c.Request.Context()
	db := h.platformDB(c)
	operatorID := middleware.GetUserID(ctx)
	batchID := c.Param("batchId")

	var req struct {
		Action          string `json:"action" binding:"required,oneof=approve reject"`
		Reason          string `json:"reason"`
		RealName        string `json:"real_name"` // #1807: 员工根据身份证照核对填写（approve 时）
		IdCardNo        string `json:"id_card_no"`
		IdCardExpire    string `json:"id_card_expire"`    // #1807: 有效期（YYYY-MM-DD 或「长期」）
		IdCardAuthority string `json:"id_card_authority"` // #1807: 签发机关
		IdCardAddress   string `json:"id_card_address"`   // #1807: 证件住址
		SecondDocType   string `json:"second_doc_type"`   // #1924: 员工审核时指定第二证件类型（student/teacher/work/other）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "action must be approve or reject"})
		return
	}
	if req.Action == "reject" && req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "reason is required for reject"})
		return
	}

	now := time.Now()
	var batch models.FaceCaptureBatch
	if err := db.Where("id = ? AND status = ?", batchID, "pending").First(&batch).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "batch not found or already reviewed"})
		return
	}

	// #1807/#1822: approve 时实名信息校验。
	// - 已采录（用户详情已录 real_name + id_card_no）：审核 = 证/人一致性核验，
	//   approve 无需携带 5 项字段（复用已存，弹窗呈只读摘要），也不得覆盖已存字段
	// - 未采录：员工必须填写 5 项实名信息（按证件照抄录，防顾客手输伪造）
	// 目标用户（#1924：审核涉及第二证件状态）。
	var targetUser models.User
	targetUserFound := db.Select("id, tenant_id, real_name, id_card_no, id_photo_other, id_photo_other_type, face_verified").
		Where("id = ?", batch.UserID).First(&targetUser).Error == nil
	secondDocPresent := targetUserFound && targetUser.IdPhotoOther != nil && *targetUser.IdPhotoOther != ""

	collected := false
	if req.Action == "approve" {
		if batch.Kind == "second_doc" {
			// 第二证件复审：身份证实名信息已认证，仅需指定类型。
			if !secondDocPresent {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "user has no second document to review"})
				return
			}
			if !validSecondDocType(req.SecondDocType) {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "second_doc_type 必填且需为 student/teacher/work/other"})
				return
			}
		} else {
			collected = targetUserFound && userHasCoreIDInfo(&targetUser)
			if !collected && (req.RealName == "" || req.IdCardNo == "" || req.IdCardExpire == "" ||
				req.IdCardAuthority == "" || req.IdCardAddress == "") {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "real_name, id_card_no, id_card_expire, id_card_authority and id_card_address are required for approve (user has no existing ID info)"})
				return
			}
			// #1924: 顾客提交了第二证件 → 审核时必须指定类型。
			if secondDocPresent && !validSecondDocType(req.SecondDocType) {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "second_doc_type 必填且需为 student/teacher/work/other"})
				return
			}
		}
	}

	// 操作人姓名（本地 users 缓存，audit 留痕用）。
	operatorName := operatorID
	var opUser models.User
	if err := db.Select("name").Where("iam_sub = ?", operatorID).First(&opUser).Error; err == nil && opUser.Name != "" {
		operatorName = opUser.Name
	}

	if !targetUserFound {
		log.Printf("[FaceReview] target user %s not found", batch.UserID)
	}

	tx := db.Begin()

	if req.Action == "approve" {
		// 批准：face_verified=true + method=manual + 实名信息（#1807/#1822）。
		// - 已采录（id_info_collected）：仅更新 face_verified 相关字段，保留已存实名信息
		// - 未采录：员工填写 5 项实名信息一并落库
		userUpdates := map[string]interface{}{}
		if batch.Kind == "second_doc" {
			// #1924: 第二证件复审只落类型与认证态，不触碰 face_verified。
			userUpdates["id_photo_other_type"] = req.SecondDocType
			userUpdates["id_photo_other_verified"] = true
			userUpdates["updated_at"] = now
		} else {
			userUpdates["face_verified"] = true
			userUpdates["face_verify_method"] = "manual"
			userUpdates["face_verified_at"] = now
			userUpdates["updated_at"] = now
			// 未采录：员工填写 5 项实名信息一并写入；已采录：仅更新 face_verified。
			if !collected {
				userUpdates["real_name"] = req.RealName
				userUpdates["id_card_no"] = req.IdCardNo
				userUpdates["id_card_expire"] = req.IdCardExpire
				userUpdates["id_card_authority"] = req.IdCardAuthority
				userUpdates["id_card_address"] = req.IdCardAddress
			}
			if secondDocPresent {
				userUpdates["id_photo_other_type"] = req.SecondDocType
				userUpdates["id_photo_other_verified"] = true
			}
		}
		if err := tx.Model(&models.User{}).Where("id = ?", batch.UserID).
			Updates(userUpdates).Error; err != nil {
			tx.Rollback()
			log.Printf("[FaceReview] approve user update failed for %s: %v", batch.UserID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to approve"})
			return
		}
		if err := tx.Model(&batch).Updates(map[string]interface{}{
			"status": "approved", "reviewed_by": operatorName, "reviewed_at": now,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update batch"})
			return
		}
	} else {
		// 驳回：批次 rejected + reason。
		if err := tx.Model(&batch).Updates(map[string]interface{}{
			"status": "rejected", "reject_reason": req.Reason,
			"reviewed_by": operatorName, "reviewed_at": now,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update batch"})
			return
		}
	}

	// audit_logs 留痕（R2 M4）。平台员工审核为全租户操作（无 tenant 上下文），
	// tenant_id 用 UUID 零值（列 not null + uuid 类型，空字符串触发 22P02）；
	// details 为 jsonb 列，必须是合法 JSON。
	action := "face_review_approve"
	detailsJSON, _ := json.Marshal(map[string]string{"result": "approved", "method": "manual"})
	if req.Action == "reject" {
		action = "face_review_reject"
		detailsJSON, _ = json.Marshal(map[string]string{"result": "rejected", "reason": req.Reason})
	}
	detailStr := string(detailsJSON)
	if err := tx.Create(&models.AuditLog{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		UserID:       operatorID,
		Action:       action,
		ResourceType: "user",
		ResourceID:   batch.UserID,
		Details:      &detailStr,
		CreatedAt:    now,
	}).Error; err != nil {
		log.Printf("[FaceReview] audit log write failed: %v", err)
	}

	tx.Commit()

	// #1816: 审核通过/驳回后通知顾客（ntype=id_verify，与 user_management_idcard.go 一致）。
	if targetUser.ID != "" {
		if req.Action == "approve" {
			services.Notify(db, targetUser.TenantID, targetUser.ID, "id_verify", "实名认证已通过",
				"您的人脸核身已审核通过，实名认证已完成", targetUser.ID, "user")
		} else {
			services.Notify(db, targetUser.TenantID, targetUser.ID, "id_verify", "实名认证未通过，请重新采集",
				"您的人脸核身审核未通过，请重新采集。原因："+req.Reason, targetUser.ID, "user")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": batch.Status},
	})
}

// #1924: reviewer-assigned second document type.
func validSecondDocType(t string) bool {
	switch t {
	case "student", "teacher", "work", "other":
		return true
	}
	return false
}
