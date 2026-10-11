package handlers

import (
	"net/http"

	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BIZNeedIdentity is returned when an order is blocked by a missing identity
// prerequisite (#2037/#2040): ID photo missing and/or real name not verified.
const BIZNeedIdentity = 40310

// CheckIdentityForOrder returns (0, "", nil) when the user may place an order.
// Otherwise it returns the business code, a user-facing message and the
// machine-readable reasons (no_id_photo / not_verified) so the checkout can
// guide precisely.
func CheckIdentityForOrder(db *gorm.DB, userID string) (int, string, []string) {
	var u models.User
	if err := db.Select("face_verified, id_photo_front, id_photo_back").
		First(&u, "id = ?", userID).Error; err != nil {
		return BIZNeedIdentity, "请先完成实名认证", []string{"not_verified"}
	}
	var reasons []string
	if u.IdPhotoFront == nil || *u.IdPhotoFront == "" || u.IdPhotoBack == nil || *u.IdPhotoBack == "" {
		reasons = append(reasons, "no_id_photo")
	}
	if !u.FaceVerified {
		reasons = append(reasons, "not_verified")
	}
	if len(reasons) == 0 {
		return 0, "", nil
	}
	msg := "请先上传身份证照片并完成实名认证（平台审核通过后）再下单"
	if len(reasons) == 1 && reasons[0] == "no_id_photo" {
		msg = "请先上传身份证正反面照片"
	}
	// #2202: pending_review 期间（已提交自拍+视频待审核）提示审核中
	if len(reasons) == 1 && reasons[0] == "not_verified" && hasPendingFaceBatch(db, userID) {
		msg = "实名认证审核中，请等待审核通过后再下单"
	}
	return BIZNeedIdentity, msg, reasons
}

// hasPendingFaceBatch checks if user has a pending FaceCaptureBatch (#2202).
func hasPendingFaceBatch(db *gorm.DB, userID string) bool {
	var count int64
	db.Model(&models.FaceCaptureBatch{}).
		Where("user_id = ? AND status = ?", userID, "pending").
		Count(&count)
	return count > 0
}

// requireIdentityForOrder writes the 40310 response and returns false when the
// user is blocked; returns true when the order may proceed.
func requireIdentityForOrder(c *gin.Context, db *gorm.DB, userID string) bool {
	bizCode, msg, reasons := CheckIdentityForOrder(db, userID)
	if bizCode == 0 {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"code":    bizCode,
		"message": msg,
		"data":    gin.H{"reasons": reasons},
	})
	return false
}
