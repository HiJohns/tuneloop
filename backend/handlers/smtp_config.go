package handlers

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// #1910: UI-managed SMTP configuration (阿里云 DirectMail with a legacy
// personal-mailbox fallback). System-admin only; the password is never
// returned and is encrypted at rest.

// GetSMTPConfig returns the stored config with the password masked.
func GetSMTPConfig(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	db := database.GetDB().WithContext(c.Request.Context())

	rec := services.SMTPConfigRecord{Port: 587}
	found := false
	var s models.SystemSetting
	if err := db.Where("tenant_id = ? AND setting_key = ?", services.WarningConfigTenantID, services.SMTPConfigKey).First(&s).Error; err == nil && strings.TrimSpace(s.SettingValue) != "" {
		if err := json.Unmarshal([]byte(s.SettingValue), &rec); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "SMTP 配置解析失败: " + err.Error()})
			return
		}
		found = true
	}

	passwordSet := rec.Password != ""
	source := "env"
	if rec.Enabled && rec.Host != "" && rec.From != "" {
		source = "db"
	} else if !services.LoadSMTPConfigFromEnv().Configured() {
		source = "none"
	}

	rec.Password = ""
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"host":         rec.Host,
		"port":         rec.Port,
		"username":     rec.Username,
		"from":         rec.From,
		"use_tls":      rec.UseTLS,
		"enabled":      rec.Enabled,
		"configured":   found,
		"password_set": passwordSet,
		"source":       source,
	}})
}

// UpdateSMTPConfig validates and saves the config (empty password keeps the
// stored one).
func UpdateSMTPConfig(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var rec services.SMTPConfigRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}
	if rec.From != "" {
		if _, err := mail.ParseAddress(rec.From); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "发件人邮箱格式不正确"})
			return
		}
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if err := services.SaveSMTPConfigRecord(db, rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	details := "SMTP 配置已更新"
	db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   middleware.GetTenantID(ctx),
		UserID:     middleware.GetUserID(ctx),
		Action:     "update_smtp_config",
		ResourceID: services.SMTPConfigKey,
		Details:    &details,
		CreatedAt:  time.Now(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
}

// TestSMTPConfig sends a test e-mail through the effective channel
// (UI-managed primary, then env fallback).
func TestSMTPConfig(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var req struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "收件邮箱必填"})
		return
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(req.To)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "收件邮箱格式不正确"})
		return
	}
	channel, err := services.SendMailWithFallback(
		[]string{strings.TrimSpace(req.To)},
		"[TuneLoop] 邮件配置测试",
		"这是一封 SMTP 配置测试邮件。\n\n如果你收到它，说明邮件通道工作正常。\n",
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "测试发送失败: " + err.Error()})
		return
	}
	ctx := c.Request.Context()
	details := "SMTP 测试发送成功（通道: " + channel + "）"
	database.GetDB().WithContext(ctx).Create(&models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   middleware.GetTenantID(ctx),
		UserID:     middleware.GetUserID(ctx),
		Action:     "test_smtp_config",
		ResourceID: services.SMTPConfigKey,
		Details:    &details,
		CreatedAt:  time.Now(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"channel": channel}})
}
