package handlers

import (
	"log"
	"net/http"
	"regexp"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/tencentcloud"

	"github.com/gin-gonic/gin"
)

var idCardRegex = regexp.MustCompile(`^\d{17}[\dX]$`)

type FaceVerifyHandler struct {
	provider tencentcloud.FaceVerifyProvider
}

func NewFaceVerifyHandler(provider tencentcloud.FaceVerifyProvider) *FaceVerifyHandler {
	return &FaceVerifyHandler{provider: provider}
}

// faceVerifyTokenRequest is the request body for POST /user/face-verify/token.
type faceVerifyTokenRequest struct {
	Name     string `json:"name" binding:"required"`
	IdCardNo string `json:"id_card_no" binding:"required"`
}

// Token requests a face-verification BizToken from Tencent Cloud and
// persists the user's real name + ID card number.
// POST /api/user/face-verify/token (userOptionalAuth)
func (h *FaceVerifyHandler) Token(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40104, "message": "not logged in"})
		return
	}

	var req faceVerifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request: " + err.Error()})
		return
	}
	if !idCardRegex.MatchString(req.IdCardNo) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id_card_no format, must be 18 digits (last may be X)"})
		return
	}

	bizToken, err := h.provider.GetToken(req.Name, req.IdCardNo)
	if err != nil {
		if err == tencentcloud.ErrNotConfigured {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40012, "message": "face verify not configured"})
			return
		}
		log.Printf("[FaceVerify] GetToken failed for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "face verify token failed: " + err.Error()})
		return
	}

	// Persist real name and id_card_no to the user record.
	db := database.GetDB().WithContext(ctx)
	updates := map[string]interface{}{
		"real_name": req.Name,
		"id_card_no": req.IdCardNo,
		"updated_at": time.Now(),
	}
	if err := db.Model(&models.User{}).Where("iam_sub = ?", userID).Updates(updates).Error; err != nil {
		log.Printf("[FaceVerify] persist user info failed for %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50002, "message": "failed to save user info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"biz_token": bizToken,
		},
	})
}

// faceVerifyResultRequest is the request body for POST /user/face-verify/result.
type faceVerifyResultRequest struct {
	BizToken string `json:"biz_token" binding:"required"`
}

// Result queries the face-verification result from Tencent Cloud and
// marks the user as verified on success.
// POST /api/user/face-verify/result (userOptionalAuth)
func (h *FaceVerifyHandler) Result(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40104, "message": "not logged in"})
		return
	}

	var req faceVerifyResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request: " + err.Error()})
		return
	}

	passed, similarity, err := h.provider.GetResult(req.BizToken)
	if err != nil {
		if err == tencentcloud.ErrNotConfigured {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40012, "message": "face verify not configured"})
			return
		}
		log.Printf("[FaceVerify] GetResult failed for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50003, "message": "face verify result failed: " + err.Error()})
		return
	}

	if passed {
		db := database.GetDB().WithContext(ctx)
		now := time.Now()
		if err := db.Model(&models.User{}).Where("iam_sub = ?", userID).Updates(map[string]interface{}{
			"face_verified":     true,
			"face_verified_at":  now,
			"updated_at":        now,
		}).Error; err != nil {
			log.Printf("[FaceVerify] persist verification failed for %s: %v", userID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50004, "message": "failed to save verification result"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"passed":     passed,
			"similarity": similarity,
		},
	})
}
