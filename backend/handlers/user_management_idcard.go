package handlers

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
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

	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	updates["updated_at"] = time.Now()
	if err := db.Model(&user).Updates(updates).Error; err != nil {
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
	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	now := time.Now()
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
				"reviewed_at": now,
			}).Error; err != nil {
			return fmt.Errorf("void pending batches: %w", err)
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
