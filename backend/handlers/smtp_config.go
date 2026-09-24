package handlers

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
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

	// #2062: 平台级配置 → 审计租户用平台租户（全零 uuid，与 SaveSMTPConfigRecord 同源）；
	// middleware.GetTenantID 对系统管理员为 "" → 非法 uuid，写入仍会静默失败。
	writeAuditLog(db, services.WarningConfigTenantID, middleware.GetUserID(ctx),
		"update_smtp_config", "", services.SMTPConfigKey, map[string]interface{}{"event": "update"})
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
	// #2062: 同上传——平台级审计租户用全零 uuid
	writeAuditLog(database.GetDB().WithContext(ctx), services.WarningConfigTenantID, middleware.GetUserID(ctx),
		"test_smtp_config", "", services.SMTPConfigKey, map[string]interface{}{"event": "test_send", "channel": channel})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"channel": channel}})
}
