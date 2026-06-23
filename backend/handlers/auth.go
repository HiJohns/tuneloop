package handlers

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"strings"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
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
	iamNs := os.Getenv("IAM_NAMESPACE")
	clientID := ""
	if iamNs != "" {
		clientID = iamNs + "_web"
	}
	if clientID == "" {
		clientID = os.Getenv("IAM_PC_CLIENT_ID")
	}
	if clientID == "" {
		clientID = os.Getenv("IAM_CLIENT_ID")
	}
	redirectURI := os.Getenv("EXTERNAL_WEB_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:5554"
	}
	redirectURI += "/callback"

	culture := middleware.GetCulture(c)
	authURL := externalURL + "/oauth/authorize?client_id=" + clientID + "&redirect_uri=" + redirectURI + "&response_type=code&culture=" + culture + "&noRegister=1"

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

	var postBody struct {
		Code       string `json:"code"`
		State      string `json:"state"`
		ClientType string `json:"client_type"`
	}
	if c.Request.Method == "POST" {
		if err := c.ShouldBindJSON(&postBody); err == nil {
			if code == "" {
				code = postBody.Code
			}
			if state == "" {
				state = postBody.State
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

	redirectURI := os.Getenv("EXTERNAL_WEB_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:5554"
	}
	redirectURI += "/callback"

	clientType := c.Query("client_type")
	if clientType == "" {
		clientType = postBody.ClientType
	}
	if clientType == "wx" || clientType == "wechat" || clientType == "mobile" {
		if wxURI := os.Getenv("EXTERNAL_MOBILE_URL"); wxURI != "" {
			redirectURI = wxURI + "/callback"
		}
	}

	if referer := c.GetHeader("Referer"); os.Getenv("EXTERNAL_WEB_URL") != "" && referer != "" {
		if strings.Contains(referer, "wx.") || strings.Contains(referer, "wx-") {
			if wxURI := os.Getenv("EXTERNAL_MOBILE_URL"); wxURI != "" {
				redirectURI = wxURI + "/callback"
			}
		}
	}

	// Use app-specific credentials (not namespace) for code exchange
	iamNs := os.Getenv("IAM_NAMESPACE")
	appClientID := iamNs + "_web"
	appSecret := services.GetAppSecret(appClientID)
	if clientType == "wx" || clientType == "wechat" || clientType == "mobile" {
		appClientID = iamNs + "_wechat"
		appSecret = services.GetAppSecret(appClientID)
	}

	var tokenResp *services.TokenResponse
	var err error
	if appSecret != "" {
		tokenResp, err = services.ExchangeCode(appClientID, appSecret, code, redirectURI)
	} else {
		tokenResp, err = h.iamService.ExchangeCodeWithRedirect(code, redirectURI)
	}
	if err != nil {
		log.Printf("[Auth] Token exchange failed: client=%s redirectURI=%s error=%v", appClientID, redirectURI, err)
		if strings.Contains(err.Error(), "already used") {
			c.JSON(http.StatusOK, gin.H{
				"code":    20000,
				"message": "code already used, please login again",
				"data":    gin.H{"relogin": true},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to exchange code for token: " + err.Error(),
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

		// Sync IAM user to local users table (same pattern as WxLogin)
		var existing models.User
		if err := h.db.Where("iam_sub = ?", claims.UserID).First(&existing).Error; err != nil {
			newUser := models.User{
				IAMSub:   claims.UserID,
				TenantID: claims.TenantID,
				OrgID:    claims.OrgID,
				Name:     claims.Name,
				Role:     "USER",
				Status:   "active",
			}
			if createErr := h.db.Create(&newUser).Error; createErr != nil {
				log.Printf("[Auth] Failed to create local user for iam_sub %s: %v", claims.UserID, createErr)
			}
		}
	}

	// Set access_token and refresh_token cookies
	// PC 端会话时长设置为 1 小时 (3600 秒)
	c.SetSameSite(http.SameSiteLaxMode)

	// Set cookie domain for subdomain sharing
	cookieDomain := ""
	if c.Request.Host != "" {
		if strings.Contains(c.Request.Host, "cadenzayueqi.com") {
			cookieDomain = ".cadenzayueqi.com"
		} else if strings.Contains(c.Request.Host, "linxdeep.com") {
			cookieDomain = ".linxdeep.com"
		}
	}

	if cookieDomain != "" {
		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 900
		}
		c.SetCookie("token", tokenResp.AccessToken, maxAge, "/", cookieDomain, false, false)
		c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", cookieDomain, false, true)
	} else {
		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 900
		}
		c.SetCookie("token", tokenResp.AccessToken, maxAge, "/", "", false, false)
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
	authCode := "auth-code-" + req.Username + "-" + middleware.GetTenantID(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"code":         authCode,
			"redirect_uri": req.RedirectURI,
		},
	})
}

func (h *AuthHandler) WxLogin(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameter: code",
		})
		return
	}

	tokenResp, err := h.iamService.WxLogin(req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "wx-login failed: " + err.Error(),
		})
		return
	}

	// Sync IAM user to local users table
	if tokenResp != nil && tokenResp.AccessToken != "" {
		claims, parseErr := h.iamService.ValidateToken(tokenResp.AccessToken)
		if parseErr == nil && claims.UserID != "" {
			var user models.User
			result := h.db.Where("iam_sub = ?", claims.UserID).First(&user)
			if result.Error != nil {
				newUser := models.User{
					IAMSub:   claims.UserID,
					TenantID: claims.TenantID,
					OrgID:    claims.OrgID,
					Name:     claims.Name,
					Role:     "USER",
					Status:   "active",
				}
				if err := h.db.Create(&newUser).Error; err != nil {
					log.Printf("[WxLogin] Failed to create local user for iam_sub %s: %v", claims.UserID, err)
				}
			}
		} else if parseErr != nil {
			log.Printf("[WxLogin] Failed to parse JWT for local sync: %v", parseErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}

func (h *AuthHandler) WxPhone(c *gin.Context) {
	var req struct {
		EncryptedData string `json:"encrypted_data" binding:"required"`
		IV            string `json:"iv" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameters: encrypted_data, iv",
		})
		return
	}

	resp, err := h.iamService.WxPhone(req.EncryptedData, req.IV)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "wx-phone failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": resp,
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
