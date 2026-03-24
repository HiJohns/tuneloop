package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"tuneloop-backend/middleware"
)

// WXAuthHandler handles WeChat Mini Program authentication
type WXAuthHandler struct {
	iamService *IAMService
}

func NewWXAuthHandler() *WXAuthHandler {
	return &WXAuthHandler{
		iamService: NewIAMService(),
	}
}

// WeChatLogin - POST /api/auth/wx-login
func (h *WXAuthHandler) WeChatLogin(c *gin.Context) {
	var req struct {
		Code        string `json:"code" binding:"required"`
		IV          string `json:"iv"`
		EncryptedData string `json:"encrypted_data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "code is required",
		})
		return
	}

	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "tenant_id is required",
		})
		return
	}

	// Call WeChat API to get open_id and union_id
	// In production, this would call WeChat's auth.code2Session API
	// For now, we'll mock the response
	openID := "wx_openid_" + req.Code
	unionID := "wx_unionid_" + req.Code

	// Check if user exists, if not create new user
	userID := "user_" + openID
	isNewUser := false

	// Mock user lookup/creation
	// In production, this would query/create in database
	if req.Code == "new_user_code" {
		isNewUser = true
		// Create new user profile
		userID = "user_new_" + time.Now().Format("20060102150405")
	}

	// Generate JWT token (30 days expiry)
	token := "jwt_token_" + userID + "_" + tenantID
	expiresIn := 30 * 24 * 3600 // 30 days

	// Prepare response
	userInfo := gin.H{
		"id":         userID,
		"open_id":    openID,
		"union_id":   unionID,
		"tenant_id":  tenantID,
		"nickname":   "微信用户",
		"avatar_url": "https://example.com/avatar.jpg",
		"phone":      "",
		"is_new":     isNewUser,
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   expiresIn,
			"user_info":    userInfo,
		},
	})
}

// NewIAMService creates a new IAM service (placeholder)
func NewIAMService() *IAMService {
	return &IAMService{}
}

type IAMService struct{}

// ExchangeCode exchanges WeChat code for session info
func (s *IAMService) ExchangeCode(code string) (map[string]interface{}, error) {
	// Mock implementation
	return map[string]interface{}{
		"openid":     "mock_openid_" + code,
		"unionid":    "mock_unionid_" + code,
		"session_key": "mock_session_key",
	}, nil
}
