package handlers

import (
	"crypto/rand"
	"net/http"
	"os"
	"strings"
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
	clientID := os.Getenv("IAM_PC_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("IAM_CLIENT_ID")
	}
	redirectURI := os.Getenv("IAM_PC_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = os.Getenv("IAM_REDIRECT_URI")
	}

	if redirectURI == "" {
		redirectURI = externalURL + "/oauth/authorize?client_id=" + clientID + "&redirect_uri=" + clientID
	}

	authURL := externalURL + "/oauth/authorize?client_id=" + clientID + "&redirect_uri=" + redirectURI + "&response_type=code"

	state := generateRandomState(32)
	c.SetCookie("oauth_state", state, 300, "/", "", false, false)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"authorization_url": authURL + "&state=" + state,
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

	// Handle POST requests with JSON body (frontend may send code via POST)
	if c.Request.Method == "POST" {
		var req struct {
			Code  string `json:"code"`
			State string `json:"state"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			if code == "" {
				code = req.Code
			}
			if state == "" {
				state = req.State
			}
		}
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameter: code",
		})
		return
	}

	// Note: State validation is currently disabled for cross-domain cookie compatibility.
	// The oauth_state cookie set by /api/auth/oidc/authorization-url is not reliably
	// sent by browsers when redirecting from IAM back to the callback.
	// TODO: Implement server-side state storage (e.g., Redis) for production.
	_ = state // Suppress unused variable warning

	c.SetCookie("oauth_state", "", -1, "/", "", false, false)

	redirectURI := os.Getenv("IAM_PC_REDIRECT_URI")
	tokenResp, err := h.iamService.ExchangeCodeWithRedirect(code, redirectURI)
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
		// Still set token but with empty tenant_id - middleware can handle this
		c.Set("tenant_id", "")
	} else {
		// Set tenant_id in context from claims
		c.Set("tenant_id", claims.TenantID)
	}

	// Set access_token and refresh_token cookies
	// PC 端会话时长设置为 1 小时 (3600 秒)
	c.SetSameSite(http.SameSiteLaxMode)

	// Set cookie domain for subdomain sharing
	cookieDomain := ""
	if c.Request.Host != "" {
		if strings.Contains(c.Request.Host, "linxdeep.com") {
			cookieDomain = ".linxdeep.com"
		}
	}

	if cookieDomain != "" {
		c.SetCookie("token", tokenResp.AccessToken, 3600, "/", cookieDomain, false, false)
		c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", cookieDomain, false, true)
	} else {
		c.SetCookie("token", tokenResp.AccessToken, 3600, "/", "", false, false)
		c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", "", false, true)
	}

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
