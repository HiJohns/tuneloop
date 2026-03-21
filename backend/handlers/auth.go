package handlers

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"os"
)

type AuthHandler struct {
	iamService *IAMService
	db         *gorm.DB
}

type IAMService struct {
	baseURL      string
	clientID     string
	clientSecret string
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	iamURL := os.Getenv("BEACONIAM_INTERNAL_URL")
	if iamURL == "" {
		iamURL = os.Getenv("BEACONIAM_EXTERNAL_URL")
	}
	if iamURL == "" {
		iamURL = os.Getenv("IAM_URL")
	}

	return &AuthHandler{
		iamService: &IAMService{
			baseURL:      iamURL,
			clientID:     os.Getenv("IAM_CLIENT_ID"),
			clientSecret: os.Getenv("IAM_CLIENT_SECRET"),
		},
		db: db,
	}
}

func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameters: code and state",
		})
		return
	}

	tokenResp, err := h.iamService.ExchangeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to exchange code for token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
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

func (s *IAMService) ExchangeCode(code string) (*TokenResponse, error) {
	return &TokenResponse{
		AccessToken:  "mock_access_token_for_code_" + code,
		RefreshToken: "mock_refresh_token_for_code_" + code,
		ExpiresIn:    2592000,
		TokenType:    "Bearer",
	}, nil
}

func (s *IAMService) RefreshToken(refreshToken string) (*TokenResponse, error) {
	return &TokenResponse{
		AccessToken:  "new_access_token_for_refresh_" + refreshToken,
		RefreshToken: refreshToken,
		ExpiresIn:    2592000,
		TokenType:    "Bearer",
	}, nil
}
