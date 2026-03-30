package handlers

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"os"
	"tuneloop-backend/services"
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

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"authorization_url": redirectURI,
		},
	})
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

	_ = state

	tokenResp, err := h.iamService.ExchangeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to exchange code for token",
		})
		return
	}

	// Set access_token and refresh_token cookies
	c.SetCookie("token", tokenResp.AccessToken, 604800, "/", "", false, false)
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
