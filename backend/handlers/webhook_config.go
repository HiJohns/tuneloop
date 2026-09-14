package handlers

import (
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

// #1909: WeCom robot webhook configuration.
//   - platform robot: system admin, part of /warning-settings
//   - merchant robot: the merchant admin configures their own group robot
// In-app notifications are always written; an absent/failing robot degrades
// to the built-in notification.

// requireMerchantAdmin gates merchant-scoped webhook management.
func requireMerchantAdmin(c *gin.Context) bool {
	if middleware.GetBusinessRole(c.Request.Context()) != middleware.BusinessRoleMerchantAdmin {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "仅商户管理员可配置商户通知"})
		return false
	}
	return true
}

// GetMerchantWebhook returns the merchant's robot config (URL masked).
func GetMerchantWebhook(c *gin.Context) {
	if !requireMerchantAdmin(c) {
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	cfg, err := services.LoadMerchantWebhook(db, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"enabled":   cfg.Enabled,
		"url_set":   cfg.URL != "",
		"url":       "", // never echo the robot URL back
		"tenant_id": tenantID,
	}})
}

// UpdateMerchantWebhook saves the merchant's robot config (empty URL keeps
// the stored one).
func UpdateMerchantWebhook(c *gin.Context) {
	if !requireMerchantAdmin(c) {
		return
	}
	var req struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	if err := services.SaveMerchantWebhook(db, tenantID, services.WebhookConfig{Enabled: req.Enabled, URL: req.URL}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
}

// TestMerchantWebhook pushes a test message to the merchant's configured robot.
func TestMerchantWebhook(c *gin.Context) {
	if !requireMerchantAdmin(c) {
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	cfg, err := services.LoadMerchantWebhook(db, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	if !cfg.Enabled || cfg.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "尚未配置或未启用的企业微信 Webhook"})
		return
	}
	if err := services.SendTestWebhook(cfg.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "测试发送失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "测试消息已发送"})
}

// TestPlatformWebhook pushes a test message to the platform robot configured
// in the warning settings.
func TestPlatformWebhook(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	cfg, err := services.LoadWarningNotifyConfig(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	if !cfg.WebhookEnabled || cfg.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "尚未配置或未启用的企业微信 Webhook"})
		return
	}
	if err := services.SendTestWebhook(cfg.WebhookURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "测试发送失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "测试消息已发送"})
}
