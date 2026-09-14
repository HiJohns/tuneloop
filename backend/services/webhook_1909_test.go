package services

import (
	"testing"

	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1909: only the official WeCom robot endpoint is allowed (SSRF guard).
func TestValidateWeComWebhookURL(t *testing.T) {
	ok := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc123"
	require.NoError(t, ValidateWeComWebhookURL(ok))
	require.Error(t, ValidateWeComWebhookURL("http://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
	require.Error(t, ValidateWeComWebhookURL("https://evil.example.com/cgi-bin/webhook/send?key=abc"))
	require.Error(t, ValidateWeComWebhookURL("https://qyapi.weixin.qq.com/cgi-bin/webhook/send"))
	require.Error(t, ValidateWeComWebhookURL("https://qyapi.weixin.qq.com/other?key=abc"))
	require.Error(t, ValidateWeComWebhookURL("not-a-url"))
}

// #1909: merchant robot config is encrypted at rest, kept on empty input.
func TestMerchantWebhookRoundTrip(t *testing.T) {
	db := setupWarningEmailTestDB(t)
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	tenantID := uuid.New().String()
	defer db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).Delete(&models.SystemSetting{})

	url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=merchant-key"
	require.NoError(t, SaveMerchantWebhook(db, tenantID, WebhookConfig{Enabled: true, URL: url}))

	var s models.SystemSetting
	require.NoError(t, db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).First(&s).Error)
	assert.NotContains(t, s.SettingValue, "merchant-key", "URL must be encrypted at rest")

	cfg, err := LoadMerchantWebhook(db, tenantID)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, url, cfg.URL)

	// empty URL keeps the stored value
	require.NoError(t, SaveMerchantWebhook(db, tenantID, WebhookConfig{Enabled: false}))
	cfg2, err := LoadMerchantWebhook(db, tenantID)
	require.NoError(t, err)
	assert.False(t, cfg2.Enabled)
	assert.Equal(t, url, cfg2.URL)

	// invalid host rejected
	require.Error(t, SaveMerchantWebhook(db, tenantID, WebhookConfig{Enabled: true, URL: "https://evil.example.com/x?key=1"}))
}

// #1909: merchant robot takes precedence; otherwise the platform robot is
// used; no configuration means no webhook send (in-app stays the fallback).
func TestWarningWebhookRouting(t *testing.T) {
	db := setupWarningEmailTestDB(t)
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")

	origSend := webhookSend
	defer func() { webhookSend = origSend }()
	var sent []string
	webhookSend = func(webhookURL, content string) error {
		sent = append(sent, webhookURL)
		return nil
	}

	platformURL := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=platform"
	merchantURL := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=merchant"

	mkConfig := func(webhookEnabled bool, webhookURL string) {
		require.NoError(t, SaveWarningNotifyConfig(db, WarningNotifyConfig{
			Enabled: false, WebhookEnabled: webhookEnabled, WebhookURL: webhookURL,
		}))
	}
	defer db.Where("setting_key = ?", WarningConfigKey).Delete(&models.SystemSetting{})

	warning := &models.Warning{
		ID: uuid.New().String(), SiteID: uuid.New().String(),
		Level: models.WarningSeverityHigh, Reason: "routing-test",
	}

	// 1) platform robot only
	mkConfig(true, platformURL)
	sent = nil
	_, err := SendWarningNotification(warning)
	require.NoError(t, err)
	assert.Equal(t, []string{platformURL}, sent)

	// 2) merchant robot has precedence over the platform robot
	tenantID := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: uuid.New().String(),
		Name: "测试商户", AdminUID: uuid.New().String(),
	}).Error)
	var m models.Merchant
	require.NoError(t, db.Where("tenant_id = ?", tenantID).First(&m).Error)
	require.NoError(t, SaveMerchantWebhook(db, tenantID, WebhookConfig{Enabled: true, URL: merchantURL}))
	warning.MerchantID = m.ID
	sent = nil
	_, err = SendWarningNotification(warning)
	require.NoError(t, err)
	assert.Equal(t, []string{merchantURL}, sent)
	defer db.Where("id = ?", m.ID).Delete(&models.Merchant{})
	defer db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).Delete(&models.SystemSetting{})

	// 3) no webhook configured → nothing sent, no error
	mkConfig(false, "")
	warning.MerchantID = ""
	sent = nil
	_, err = SendWarningNotification(warning)
	require.NoError(t, err)
	assert.Empty(t, sent)
}
