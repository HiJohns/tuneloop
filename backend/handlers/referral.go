package handlers

import (
	"encoding/base64"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

type ReferralHandler struct{}

func NewReferralHandler() *ReferralHandler {
	return &ReferralHandler{}
}

// GetPromoQR returns the current user's referral code and QR URLs.
func (h *ReferralHandler) GetPromoQR(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	if user.RefCode == "" {
		user.RefCode = userID[:8]
		db.Model(&user).Update("ref_code", user.RefCode)
	}

	wxURL := os.Getenv("EXTERNAL_MOBILE_URL")
	if wxURL == "" {
		wxURL = "http://localhost:5553"
	}
	h5URL := wxURL + "/?ref=" + user.RefCode + "&open=profile-complete"

	var wxacodeBase64 *string
	accessToken, tokenErr := services.GetWxAccessToken()
	if tokenErr == nil {
		scene := "ref=" + user.RefCode
		imgData, err := services.GetWxacodeUnlimited(accessToken, scene, "pages-weapp/profile-complete/index", services.GetWxEnvVersion())
		if err == nil {
			b64 := base64.StdEncoding.EncodeToString(imgData)
			wxacodeBase64 = &b64
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"ref_code":       user.RefCode,
			"h5_url":         h5URL,
			"wxacode_base64": wxacodeBase64,
		},
	})
}

// LandingPage redirects to the SPA with a ref code for H5 referral flows.
func (h *ReferralHandler) LandingPage(c *gin.Context) {
	ref := c.Query("ref")
	wxURL := os.Getenv("EXTERNAL_MOBILE_URL")
	if wxURL == "" {
		wxURL = "http://localhost:5553"
	}
	redirect := wxURL + "/?ref=" + ref + "&open=profile-complete"
	c.Redirect(http.StatusFound, redirect)
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
