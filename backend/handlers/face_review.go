package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

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

// faceReviewItem 审核队列条目（含用户证件照三张 + 自拍素材 URL）。
type faceReviewItem struct {
	BatchID     string   `json:"batch_id"`
	UserID      string   `json:"user_id"`
	UserName    string   `json:"user_name"`
	IDPhotos    []string `json:"id_photos"`
	SelfieURLs  []string `json:"selfie_urls"`
	SubmittedAt string   `json:"submitted_at"`
}

// resolveSelfieURL 组装自拍素材访问 URL。
func resolveSelfieURL(key string) string {
	if key == "" {
		return ""
	}
	if key[0] == '/' || len(key) > 5 && (key[:4] == "http") {
		return key
	}
	return "/uploads/media/" + key
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
		if err := db.Select("id, name, id_photo_front, id_photo_back, id_photo_other").
			Where("id = ?", b.UserID).First(&user).Error; err != nil {
			continue // 用户不存在（可能已删除）跳过
		}
		item := faceReviewItem{
			BatchID:     b.ID,
			UserID:      user.ID,
			UserName:    user.Name,
			SubmittedAt: b.SubmittedAt.Format(time.RFC3339),
		}
		// 证件照三张（隐私边界：仅审核用，不返回身份证号）。
		if user.IdPhotoFront != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(*user.IdPhotoFront))
		}
		if user.IdPhotoBack != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(*user.IdPhotoBack))
		}
		if user.IdPhotoOther != nil {
			item.IDPhotos = append(item.IDPhotos, resolveSelfieURL(*user.IdPhotoOther))
		}
		// 自拍素材（media_assets source_id=batch_id，#1790 M5 关联键）。
		var assets []models.MediaAsset
		db.Where("source_id = ? AND source_type = ?", b.ID, "face_capture").
			Order("created_at ASC").Find(&assets)
		for _, a := range assets {
			item.SelfieURLs = append(item.SelfieURLs, resolveSelfieURL(a.StorageKey))
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
	BatchID     string   `json:"batch_id"`
	Status      string   `json:"status"`
	RejectReason string   `json:"reject_reason,omitempty"`
	SelfieURLs  []string `json:"selfie_urls"`
	SubmittedAt string   `json:"submitted_at"`
	ReviewedAt  string   `json:"reviewed_at,omitempty"`
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
			item.SelfieURLs = append(item.SelfieURLs, resolveSelfieURL(a.StorageKey))
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
		Action         string `json:"action" binding:"required,oneof=approve reject"`
		Reason         string `json:"reason"`
		RealName       string `json:"real_name"` // #1807: 员工根据身份证照核对填写（approve 时）
		IdCardNo       string `json:"id_card_no"`
		IdCardExpire   string `json:"id_card_expire"`   // #1807: 有效期（YYYY-MM-DD 或「长期」）
		IdCardAuthority string `json:"id_card_authority"` // #1807: 签发机关
		IdCardAddress  string `json:"id_card_address"`  // #1807: 证件住址
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "action must be approve or reject"})
		return
	}
	if req.Action == "reject" && req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "reason is required for reject"})
		return
	}
	// #1807: approve 必须由员工填写实名信息（真实姓名/身份证号/有效期/签发机关/住址，
	// 根据证件照核对，防顾客手输伪造）。
	if req.Action == "approve" && (req.RealName == "" || req.IdCardNo == "" || req.IdCardExpire == "" ||
		req.IdCardAuthority == "" || req.IdCardAddress == "") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "real_name, id_card_no, id_card_expire, id_card_authority and id_card_address are required for approve"})
		return
	}

	// 操作人姓名（本地 users 缓存，audit 留痕用）。
	operatorName := operatorID
	var opUser models.User
	if err := db.Select("name").Where("iam_sub = ?", operatorID).First(&opUser).Error; err == nil && opUser.Name != "" {
		operatorName = opUser.Name
	}

	now := time.Now()
	var batch models.FaceCaptureBatch
	if err := db.Where("id = ? AND status = ?", batchID, "pending").First(&batch).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "batch not found or already reviewed"})
		return
	}

	tx := db.Begin()

	if req.Action == "approve" {
		// 批准：face_verified=true + method=manual + 员工填写的实名信息
		// （#1807：真实姓名/身份证号/有效期/签发机关/住址由员工根据身份证照核对填写）。
		if err := tx.Model(&models.User{}).Where("id = ?", batch.UserID).
			Updates(map[string]interface{}{
				"face_verified":       true,
				"face_verify_method":  "manual",
				"face_verified_at":    now,
				"real_name":           req.RealName,
				"id_card_no":          req.IdCardNo,
				"id_card_expire":      req.IdCardExpire,
				"id_card_authority":   req.IdCardAuthority,
				"id_card_address":     req.IdCardAddress,
				"updated_at":          now,
			}).Error; err != nil {
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
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": batch.Status},
	})
}
