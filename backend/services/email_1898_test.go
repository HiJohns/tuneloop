package services

import (
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupWarningEmailTestDB uses the shared test database but only ensures the
// two tables these tests touch (avoids the handlers/testfixtures import cycle
// and does not drop tables other packages may be using).
func setupWarningEmailTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available:", err)
		return nil
	}
	database.SetDB(db)
	require.NoError(t, db.AutoMigrate(&models.SystemSetting{}, &models.Warning{}))
	return db
}

// #1898: per-level warning e-mail recipients are free-form addresses; rejected
// when malformed, de-duplicated, and stored in system_settings.
func TestWarningNotifyConfigValidation(t *testing.T) {
	db := setupWarningEmailTestDB(t)

	level := "high"
	require.NoError(t, db.Where("setting_key = ?", WarningConfigKey(level)).Delete(&models.SystemSetting{}).Error)

	err := SaveWarningNotifyConfig(db, level, WarningNotifyConfig{Enabled: true, Emails: []string{"not-an-email"}})
	require.Error(t, err, "malformed recipient rejected")

	require.NoError(t, SaveWarningNotifyConfig(db, level, WarningNotifyConfig{
		Enabled: true,
		// arbitrary addresses allowed (not limited to platform users)
		Emails:          []string{" ops@example.com ", "ops@example.com", "it@corp.cn"},
		CooldownMinutes: 30,
	}))
	cfg, err := LoadWarningNotifyConfig(db, level)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"ops@example.com", "it@corp.cn"}, cfg.Emails)
	assert.Equal(t, 30, cfg.CooldownMinutes)

	require.Error(t, SaveWarningNotifyConfig(db, "urgent", WarningNotifyConfig{}))
	require.Error(t, SaveWarningNotifyConfig(db, level, WarningNotifyConfig{CooldownMinutes: 2000}))

	require.NoError(t, db.Where("setting_key = ?", WarningConfigKey(level)).Delete(&models.SystemSetting{}).Error)
}

// #1898: sends once per warning (cooldown 0), records the marker, and reports
// missing SMTP config as an explicit error instead of silently skipping.
func TestSendWarningEmailConfigAndCooldown(t *testing.T) {
	db := setupWarningEmailTestDB(t)
	level := "high"
	require.NoError(t, SaveWarningNotifyConfig(db, level, WarningNotifyConfig{
		Enabled: true, Emails: []string{"ops@example.com"},
	}))
	defer db.Where("setting_key = ?", WarningConfigKey(level)).Delete(&models.SystemSetting{})

	w := &models.Warning{
		ID: uuid.New().String(), SiteID: uuid.New().String(), MerchantID: uuid.New().String(),
		ObjectType: "order", ObjectID: uuid.New().String(),
		Level: models.WarningSeverityHigh, Reason: "test-send",
	}
	require.NoError(t, db.Create(w).Error)
	defer db.Where("id = ?", w.ID).Delete(&models.Warning{})

	// SMTP unconfigured → explicit error
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_USER", "")
	sent, err := SendWarningNotification(w)
	assert.False(t, sent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP 未配置")

	// SMTP configured + stub transport → sent once, marker recorded, repeat suppressed
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "bot@example.com")
	orig := smtpSend
	calls := 0
	smtpSend = func(cfg SMTPConfig, to []string, subject, body string) error {
		calls++
		assert.Equal(t, []string{"ops@example.com"}, to)
		assert.Contains(t, subject, "test-send")
		return nil
	}
	defer func() { smtpSend = orig }()

	sent, err = SendWarningNotification(w)
	require.NoError(t, err)
	assert.True(t, sent)
	assert.Equal(t, 1, calls)

	var stored models.Warning
	require.NoError(t, db.First(&stored, "id = ?", w.ID).Error)
	require.NotNil(t, stored.LastNotifiedAt)

	// cooldown 0 = notify once per warning
	sent, err = SendWarningNotification(&stored)
	require.NoError(t, err)
	assert.False(t, sent)
	assert.Equal(t, 1, calls)
}
