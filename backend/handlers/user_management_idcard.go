package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UpdateIdCard saves verified ID card identity fields entered by platform
// staff based on the ID photos (module 1: 身份证信息采集, #1810).
// PUT /admin/user-management/:id/id-card
// All fields optional except format validation when provided:
// real_name / id_card_no (18-digit) / id_card_expire / id_card_authority / id_card_address.
// On success notifies the user (ntype=id_verify).
func (h *UserManagementHandler) UpdateIdCard(c *gin.Context) {
	var req struct {
		RealName       *string `json:"real_name"`
		IdCardNo       *string `json:"id_card_no"`
		IdCardExpire   *string `json:"id_card_expire"`
		IdCardAuthority *string `json:"id_card_authority"`
		IdCardAddress  *string `json:"id_card_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.RealName != nil {
		updates["real_name"] = strings.TrimSpace(*req.RealName)
	}
	if req.IdCardNo != nil {
		cardNo := strings.ToUpper(strings.TrimSpace(*req.IdCardNo))
		if cardNo != "" && !regexp.MustCompile(`^\d{17}[\dX]$`).MatchString(cardNo) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "身份证号格式不正确（18 位，末位可为 X）"})
			return
		}
		updates["id_card_no"] = cardNo
	}
	if req.IdCardExpire != nil {
		updates["id_card_expire"] = strings.TrimSpace(*req.IdCardExpire)
	}
	if req.IdCardAuthority != nil {
		updates["id_card_authority"] = strings.TrimSpace(*req.IdCardAuthority)
	}
	if req.IdCardAddress != nil {
		updates["id_card_address"] = strings.TrimSpace(*req.IdCardAddress)
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no fields provided"})
		return
	}

	ctx := c.Request.Context()
	operatorID := middleware.GetUserID(ctx)
	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	updates["updated_at"] = time.Now()
	// audit_logs 与业务写入同事务（与 face_review.go Review 一致）；audit 失败仅
	// log（不阻断业务提交），但错误必须可见，不得静默吞错。
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return fmt.Errorf("update user id card: %w", err)
		}
		detailsJSON, _ := json.Marshal(map[string]interface{}{
			"fields_updated": updates,
		})
		detailStr := string(detailsJSON)
		if err := tx.Create(&models.AuditLog{
			ID:           uuid.New().String(),
			TenantID:     user.TenantID,
			UserID:       operatorID,
			Action:       "id_card_update",
			ResourceType: "user",
			ResourceID:   user.ID,
			Details:      &detailStr,
			CreatedAt:    time.Now(),
		}).Error; err != nil {
			log.Printf("[IdCard] audit log write failed for user %s: %v", user.ID, err)
		}
		return nil
	})
	if err != nil {
		log.Printf("[IdCard] update failed for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update id card info"})
		return
	}

	services.Notify(db, user.TenantID, user.ID, "id_verify", "实名信息已采集",
		"平台已完成您的实名信息采集，请继续完成人脸信息采集", user.ID, "user")

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "success"})
}

// RejectIdPhotos clears ID photos and identity fields when staff deems the
// ID photos invalid (module 1: 拒绝采用, #1810). Also voids pending face
// capture batches (they are meaningless without valid ID photos).
// POST /admin/user-management/:id/id-photo/reject
// On success notifies the user (ntype=id_verify).
func (h *UserManagementHandler) RejectIdPhotos(c *gin.Context) {
	ctx := c.Request.Context()
	operatorID := middleware.GetUserID(ctx)
	// 操作人姓名（本地 users 缓存，reviewed_by/audit 留痕用；与 face_review.go 一致）。
	operatorName := operatorID
	var opUser models.User
	if err := h.platformDB(c).Select("name").Where("iam_sub = ?", operatorID).First(&opUser).Error; err == nil && opUser.Name != "" {
		operatorName = opUser.Name
	}
	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	now := time.Now()
	// audit_logs 与业务写入同事务（与 face_review.go Review 一致）；audit 失败仅
	// log（不阻断业务提交），但错误必须可见，不得静默吞错。
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"id_photo_front":     nil,
			"id_photo_back":      nil,
			"id_photo_other":     nil,
			"id_photo_other_type": nil,
			"real_name":          nil,
			"id_card_no":         nil,
			"id_card_expire":     nil,
			"id_card_authority":  nil,
			"id_card_address":    nil,
			"face_verified":      false,
			"face_verify_method": nil,
			"updated_at":         now,
		}).Error; err != nil {
			return fmt.Errorf("clear user id photos: %w", err)
		}
		if err := tx.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND status = ?", user.ID, "pending").
			Updates(map[string]interface{}{
				"status": "rejected", "reject_reason": "证件照被管理员拒绝，请重新提交",
				"reviewed_by": operatorName, "reviewed_at": now,
			}).Error; err != nil {
			return fmt.Errorf("void pending batches: %w", err)
		}
		detailsJSON, _ := json.Marshal(map[string]string{"result": "rejected", "reason": "证件照被管理员拒绝"})
		detailStr := string(detailsJSON)
		if err := tx.Create(&models.AuditLog{
			ID:           uuid.New().String(),
			TenantID:     user.TenantID,
			UserID:       operatorID,
			Action:       "id_photo_reject",
			ResourceType: "user",
			ResourceID:   user.ID,
			Details:      &detailStr,
			CreatedAt:    now,
		}).Error; err != nil {
			log.Printf("[IdCard] audit log write failed for user %s: %v", user.ID, err)
		}
		return nil
	})
	if err != nil {
		log.Printf("[IdCard] reject failed for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reject id photos"})
		return
	}

	services.Notify(db, user.TenantID, user.ID, "id_verify", "证件照未通过，请重新上传",
		"您提交的身份证照片未通过审核，请重新上传清晰的证件照", user.ID, "user")

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "success"})
}
