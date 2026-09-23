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
	return BIZNeedIdentity, msg, reasons
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
