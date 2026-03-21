package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type BrandConfig struct {
	ID           uint   `json:"id"`
	ClientID     string `json:"client_id"`
	PrimaryColor string `json:"primary_color"`
	LogoURL      string `json:"logo_url"`
	BrandName    string `json:"brand_name"`
	SupportPhone string `json:"support_phone"`
}

func GetBrandConfig(c *gin.Context) {
	clientID := c.Query("client_id")
	if clientID == "" {
		clientID = "default"
	}

	mockConfigs := map[string]BrandConfig{
		"default": {
			ID:           1,
			ClientID:     "default",
			PrimaryColor: "#6366F1",
			LogoURL:      "/logo.png",
			BrandName:    "TuneLoop",
			SupportPhone: "400-123-4567",
		},
		"tuneloop-pro": {
			ID:           2,
			ClientID:     "tuneloop-pro",
			PrimaryColor: "#1E40AF",
			LogoURL:      "/pro-logo.png",
			BrandName:    "TuneLoop Pro",
			SupportPhone: "400-888-9999",
		},
	}

	config, exists := mockConfigs[clientID]
	if !exists {
		config = mockConfigs["default"]
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": config,
	})
}
