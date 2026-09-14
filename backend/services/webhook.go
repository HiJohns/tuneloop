package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// #1909: WeCom (企业微信) group-robot webhook channel.
//
// Routing: a warning with a merchant scope uses that merchant's own robot
// (configured by the merchant admin); otherwise the platform robot (configured
// in the warning settings) is used. In-app notifications are always written,
// so an absent/failing webhook degrades to the built-in notification.

const (
	// MerchantWebhookKey is the per-tenant system_settings key for a merchant's
	// own WeCom robot configuration.
	MerchantWebhookKey = "notification_webhook"

	wecomWebhookHost = "qyapi.weixin.qq.com"
	wecomWebhookPath = "/cgi-bin/webhook/send"
)

// WebhookConfig is the stored per-merchant (or platform) robot configuration.
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"` // encrypted envelope at rest
}

// ValidateWeComWebhookURL enforces the only allowed destination (SSRF guard).
func ValidateWeComWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("Webhook 地址格式不正确")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("Webhook 地址必须为 https")
	}
	if u.Host != wecomWebhookHost {
		return fmt.Errorf("Webhook 地址必须为企业微信官方地址（%s）", wecomWebhookHost)
	}
	if !strings.HasPrefix(u.Path, wecomWebhookPath) {
		return fmt.Errorf("Webhook 地址路径不正确")
	}
	if u.Query().Get("key") == "" {
		return fmt.Errorf("Webhook 地址缺少 key")
	}
	return nil
}

// webhookSend is a test seam over sendWeComWebhook.
var webhookSend = sendWeComWebhook

// sendWeComWebhook posts a plain-text message to a WeCom robot.
func sendWeComWebhook(webhookURL, content string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook 请求失败: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d: %s", resp.StatusCode, body.ErrMsg)
	}
	if body.ErrCode != 0 {
		return fmt.Errorf("webhook 返回错误 %d: %s", body.ErrCode, body.ErrMsg)
	}
	return nil
}

// LoadMerchantWebhook reads a merchant tenant's robot config (URL decrypted).
func LoadMerchantWebhook(db *gorm.DB, tenantID string) (WebhookConfig, error) {
	var cfg WebhookConfig
	if tenantID == "" {
		return cfg, nil
	}
	var s models.SystemSetting
	err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).First(&s).Error
	if err == gorm.ErrRecordNotFound || strings.TrimSpace(s.SettingValue) == "" {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(s.SettingValue), &cfg); err != nil {
		return cfg, fmt.Errorf("通知 Webhook 配置解析失败: %w", err)
	}
	plain, err := DecryptSecret(cfg.URL)
	if err != nil {
		return cfg, err
	}
	cfg.URL = plain
	return cfg, nil
}

// SaveMerchantWebhook upserts a merchant tenant's robot config. An empty URL
// keeps the stored one; a non-empty URL is validated and encrypted at rest.
func SaveMerchantWebhook(db *gorm.DB, tenantID string, cfg WebhookConfig) error {
	if tenantID == "" {
		return fmt.Errorf("缺少商户信息")
	}
	if cfg.URL != "" {
		if err := ValidateWeComWebhookURL(cfg.URL); err != nil {
			return err
		}
		enc, err := EncryptSecret(cfg.URL)
		if err != nil {
			return err
		}
		cfg.URL = enc
	} else {
		var s models.SystemSetting
		if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).First(&s).Error; err == nil {
			var old WebhookConfig
			if json.Unmarshal([]byte(s.SettingValue), &old) == nil {
				cfg.URL = old.URL
			}
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var s models.SystemSetting
	err = db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).First(&s).Error
	if err == nil {
		return db.Model(&s).Update("setting_value", string(raw)).Error
	}
	if err == gorm.ErrRecordNotFound {
		return db.Create(&models.SystemSetting{
			TenantID:     tenantID,
			SettingKey:   MerchantWebhookKey,
			SettingValue: string(raw),
		}).Error
	}
	return err
}

// merchantTenantID resolves a warning's merchant reference (merchant id or
// org id) to the merchant tenant.
func merchantTenantID(db *gorm.DB, merchantRef string) string {
	if merchantRef == "" {
		return ""
	}
	var m models.Merchant
	if err := db.Where("id = ? OR org_id = ?", merchantRef, merchantRef).First(&m).Error; err != nil {
		return ""
	}
	return m.TenantID
}

// resolveWebhookTarget picks the robot URL for a warning: the merchant's own
// robot first, then the platform robot configured in the warning settings.
func resolveWebhookTarget(db *gorm.DB, warning *models.Warning, platformCfg WarningNotifyConfig) string {
	if warning != nil && warning.MerchantID != "" {
		if tenantID := merchantTenantID(db, warning.MerchantID); tenantID != "" {
			if mc, err := LoadMerchantWebhook(db, tenantID); err == nil && mc.Enabled && mc.URL != "" {
				return mc.URL
			}
		}
	}
	if platformCfg.WebhookEnabled && platformCfg.WebhookURL != "" {
		return platformCfg.WebhookURL
	}
	return ""
}

// sendWarningWebhook pushes the warning to the resolved robot. No target or a
// send failure is logged; in-app notifications were already written, so the
// alert still reaches staff (#1909 fallback).
func sendWarningWebhook(w *models.Warning) error {
	if w == nil {
		return nil
	}
	db := database.GetDB()
	cfg, err := LoadWarningNotifyConfig(db)
	if err != nil {
		return err
	}
	target := resolveWebhookTarget(db, w, cfg)
	if target == "" {
		return nil
	}
	content := fmt.Sprintf("[TuneLoop 警告][%s] %s\n分类: %s\n对象: %s/%s\n描述: %s\n时间: %s",
		warningLevelLabel(w.Level), w.Reason, w.Category, w.ObjectType, w.ObjectID,
		w.Description, w.CreatedAt.Format("2006-01-02 15:04:05"))
	if err := webhookSend(target, content); err != nil {
		log.Printf("[WarningWebhook] send failed warning=%s: %v", w.ID, err)
		return err
	}
	log.Printf("[WarningWebhook] sent warning=%s", w.ID)
	return nil
}

// SendTestWebhook pushes a connectivity test message (used by the config
// pages for both platform and merchant robots).
func SendTestWebhook(webhookURL string) error {
	if err := ValidateWeComWebhookURL(webhookURL); err != nil {
		return err
	}
	return webhookSend(webhookURL, "[TuneLoop] Webhook 配置测试：如果你在群里看到这条消息，说明机器人配置成功。")
}
