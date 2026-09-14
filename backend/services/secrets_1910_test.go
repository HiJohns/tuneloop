package services

import (
	"fmt"
	"testing"

	"tuneloop-backend/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1910: secrets are encrypted at rest; a missing key refuses to store.
func TestSecretEncryptDecrypt(t *testing.T) {
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	t.Setenv("IAM_SECRET", "")
	enc, err := EncryptSecret("s3cret")
	require.NoError(t, err)
	assert.NotContains(t, enc, "s3cret")

	got, err := DecryptSecret(enc)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", got)

	got, err = DecryptSecret("plain-legacy")
	require.NoError(t, err)
	assert.Equal(t, "plain-legacy", got)

	t.Setenv("CONFIG_ENCRYPTION_KEY", "another-key")
	_, err = DecryptSecret(enc)
	require.Error(t, err, "wrong key must not decrypt")

	t.Setenv("CONFIG_ENCRYPTION_KEY", "")
	t.Setenv("IAM_SECRET", "")
	_, err = EncryptSecret("x")
	require.Error(t, err, "no key material → refuse to store")
}

// #1910: DB record round-trip (encrypted password), keep-on-empty, disabled.
func TestSMTPConfigRecordRoundTrip(t *testing.T) {
	db := setupWarningEmailTestDB(t)
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	defer db.Where("setting_key = ?", SMTPConfigKey).Delete(&models.SystemSetting{})

	require.NoError(t, SaveSMTPConfigRecord(db, SMTPConfigRecord{
		Host: "smtpdm.aliyun.com", Port: 465, Username: "u@d.com",
		From: "noreply@d.com", UseTLS: true, Enabled: true, Password: "p@ss",
	}))

	var s models.SystemSetting
	require.NoError(t, db.Where("setting_key = ?", SMTPConfigKey).First(&s).Error)
	assert.NotContains(t, s.SettingValue, "p@ss", "password must be encrypted at rest")

	cfg, ok, err := LoadSMTPConfigFromDB(db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "p@ss", cfg.Password)
	assert.Equal(t, 465, cfg.Port)

	// empty password keeps the stored one
	require.NoError(t, SaveSMTPConfigRecord(db, SMTPConfigRecord{
		Host: "smtpdm.aliyun.com", Port: 465, From: "noreply@d.com", UseTLS: true, Enabled: true,
	}))
	cfg2, ok2, err2 := LoadSMTPConfigFromDB(db)
	require.NoError(t, err2)
	require.True(t, ok2)
	assert.Equal(t, "p@ss", cfg2.Password)

	// disabled → not usable
	require.NoError(t, SaveSMTPConfigRecord(db, SMTPConfigRecord{Host: "h", From: "f@d.com", Enabled: false}))
	_, ok3, _ := LoadSMTPConfigFromDB(db)
	assert.False(t, ok3)
}

// #1910: primary (UI) config first, env personal mailbox as fallback.
func TestSendMailWithFallback_UsesPrimaryThenEnv(t *testing.T) {
	db := setupWarningEmailTestDB(t)
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	defer db.Where("setting_key = ?", SMTPConfigKey).Delete(&models.SystemSetting{})

	orig := smtpSend
	defer func() { smtpSend = orig }()
	var calls []string

	require.NoError(t, SaveSMTPConfigRecord(db, SMTPConfigRecord{
		Host: "primary", Port: 587, From: "n@d.com", Enabled: true,
	}))
	t.Setenv("SMTP_HOST", "envhost")
	t.Setenv("SMTP_FROM", "e@d.com")

	smtpSend = func(cfg SMTPConfig, to []string, subject, body string) error {
		calls = append(calls, cfg.Host)
		return nil
	}
	channel, err := SendMailWithFallback([]string{"x@y.com"}, "s", "b")
	require.NoError(t, err)
	assert.Equal(t, "directmail", channel)
	assert.Equal(t, []string{"primary"}, calls)

	calls = nil
	smtpSend = func(cfg SMTPConfig, to []string, subject, body string) error {
		calls = append(calls, cfg.Host)
		if cfg.Host == "primary" {
			return fmt.Errorf("primary down")
		}
		return nil
	}
	channel, err = SendMailWithFallback([]string{"x@y.com"}, "s", "b")
	require.NoError(t, err)
	assert.Equal(t, "personal", channel)
	assert.Equal(t, []string{"primary", "envhost"}, calls)

	// neither configured → explicit error
	db.Where("setting_key = ?", SMTPConfigKey).Delete(&models.SystemSetting{})
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_USER", "")
	_, err = SendMailWithFallback([]string{"x@y.com"}, "s", "b")
	require.Error(t, err)
}
