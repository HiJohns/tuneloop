package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

type ReferralHandler struct{}

func NewReferralHandler() *ReferralHandler {
	return &ReferralHandler{}
}

// GetPromoQR returns the current user's referral code and QR URL.
func (h *ReferralHandler) GetPromoQR(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	// Generate ref_code if not set
	if user.RefCode == "" {
		user.RefCode = userID[:8]
		db.Model(&user).Update("ref_code", user.RefCode)
	}

	qrcodeURL := "/pages-weapp/profile-complete/index?ref=" + user.RefCode
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"ref_code": user.RefCode,
			"url":      qrcodeURL,
		},
	})
}

// ListReferrals returns the referral list for the current user.
func (h *ReferralHandler) ListReferrals(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	var referrals []models.Referral
	db.Where("referrer_id = ?", user.ID).Order("created_at desc").Find(&referrals)

	type ReferralInfo struct {
		RefCode   string `json:"ref_code"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	var list []ReferralInfo
	for _, r := range referrals {
		var referee models.User
		if db.First(&referee, "id = ?", r.RefereeID).Error == nil {
			list = append(list, ReferralInfo{
				RefCode:   r.RefCode,
				Name:      referee.Name,
				CreatedAt: r.CreatedAt.Format("2006-01-02 15:04"),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"total": len(list),
			"list":  list,
		},
	})
}
