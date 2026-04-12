package handlers

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	iamService *services.IAMService
	db         *gorm.DB
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	iamSvc := services.NewIAMService()
	return &AuthHandler{
		iamService: iamSvc,
		db:         db,
	}
}

func (h *AuthHandler) GetOIDCAuthorizationURL(c *gin.Context) {
	externalURL := services.GetIAMExternalURL()
	clientID := os.Getenv("IAM_CLIENT_ID")
	redirectURI := os.Getenv("IAM_REDIRECT_URI")

	if redirectURI == "" {
		redirectURI = externalURL + "/authorize?client_id=" + clientID
	}

	state := generateRandomState(32)
	c.SetCookie("oauth_state", state, 300, "/", "", false, false)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"authorization_url": redirectURI + "&state=" + state,
		},
	})
}

func generateRandomState(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}
	return string(bytes)
}

func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" && c.Request.Method == "POST" {
		var req struct {
			Code string `json:"code"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			code = req.Code
		}
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameter: code",
		})
		return
	}

	expectedState, err := c.Cookie("oauth_state")
	if err != nil || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing or invalid state parameter",
		})
		return
	}

	if state != expectedState {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid state parameter - possible CSRF attack",
		})
		return
	}

	c.SetCookie("oauth_state", "", -1, "/", "", false, false)

	tokenResp, err := h.iamService.ExchangeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to exchange code for token",
		})
		return
	}

	// Validate token and extract claims to get tenant_id
	claims, err := h.iamService.ValidateToken(tokenResp.AccessToken)
	if err != nil {
		// Log the error but continue - don't block login
		log.Printf("[WARNING] Token validation failed: %v, proceeding with empty tenant_id", err)
		// Still set token but with empty tenant_id - middleware can handle this
		c.Set("tenant_id", "")
		log.Printf("[DEBUG Callback] Setting cookies with empty tenant_id due to validation failure")
	} else {
		// Set tenant_id in context from claims
		c.Set("tenant_id", claims.TenantID)
		log.Printf("[DEBUG Callback] Setting cookies for tenant: %s", claims.TenantID)
	}

	// Log token info for debugging (DO NOT log full token in production!)
	log.Printf("[DEBUG Callback] Access token length: %d", len(tokenResp.AccessToken))
	log.Printf("[DEBUG Callback] Refresh token length: %d", len(tokenResp.RefreshToken))

	// Set access_token and refresh_token cookies
	// PC 端会话时长设置为 1 小时 (3600 秒)
	// Use SameSite=None and Secure=false for local development
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", tokenResp.AccessToken, 3600, "/", "", false, false)            // 1 hour
	c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", "", false, true) // 30 days, httpOnly

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}

func (h *AuthHandler) PostLogin(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		RedirectURI string `json:"redirect_uri"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	// For now, return a simple authorization code
	// In production, this would validate credentials and generate a proper code
	authCode := "auth-code-" + req.Username + "-" + c.GetString("tenant_id")

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"code":         authCode,
			"redirect_uri": req.RedirectURI,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request body",
		})
		return
	}

	tokenResp, err := h.iamService.RefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}
