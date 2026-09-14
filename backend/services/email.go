package services

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// #1898: warning e-mail notifications. SMTP settings mirror the IAM mailer
// (SMTP_HOST/SMTP_PORT/SMTP_USER/SMTP_PASSWORD/SMTP_FROM/SMTP_USE_TLS); the
// recipient list is configured per warning level in system_settings.

// WarningConfigTenantID is the platform tenant that owns warning settings.
const WarningConfigTenantID = "00000000-0000-0000-0000-000000000000"

// WarningConfigKey is the single system_settings key for warning
// notifications (#1908: one config, not per-severity).
const WarningConfigKey = "warning_notification"

// SMTPConfig holds the SMTP connection settings from the environment.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	UseTLS   bool
}

// SMTPConfigRecord is the UI-managed SMTP configuration (阿里云 DirectMail),
// stored in system_settings. Password is write-only for the API and kept
// encrypted at rest (#1910).
type SMTPConfigRecord struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
	Enabled  bool   `json:"enabled"`
	Password string `json:"password,omitempty"`
}

// SMTPConfigKey is the system_settings key for the UI-managed SMTP config.
const SMTPConfigKey = "smtp_config"

// LoadSMTPConfigFromEnv reads the SMTP settings from the environment
// (legacy personal-mailbox fallback; same keys as beaconiam).
func LoadSMTPConfigFromEnv() SMTPConfig {
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if port == 0 {
		port = 587
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = os.Getenv("SMTP_USER")
	}
	return SMTPConfig{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     port,
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     from,
		UseTLS:   os.Getenv("SMTP_USE_TLS") != "0",
	}
}

// Configured reports whether enough settings exist to attempt a send.
func (c SMTPConfig) Configured() bool { return c.Host != "" && c.From != "" }

// LoadSMTPConfigFromDB reads the UI-managed config. ok=false when it is
// absent, disabled or incomplete.
func LoadSMTPConfigFromDB(db *gorm.DB) (SMTPConfig, bool, error) {
	var s models.SystemSetting
	err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigKey).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return SMTPConfig{}, false, nil
	}
	if err != nil {
		return SMTPConfig{}, false, err
	}
	if strings.TrimSpace(s.SettingValue) == "" {
		return SMTPConfig{}, false, nil
	}
	var rec SMTPConfigRecord
	if err := json.Unmarshal([]byte(s.SettingValue), &rec); err != nil {
		return SMTPConfig{}, false, fmt.Errorf("SMTP 配置 JSON 解析失败: %w", err)
	}
	if !rec.Enabled || rec.Host == "" || rec.From == "" {
		return SMTPConfig{}, false, nil
	}
	password, err := DecryptSecret(rec.Password)
	if err != nil {
		return SMTPConfig{}, false, err
	}
	return SMTPConfig{
		Host: rec.Host, Port: rec.Port, User: rec.Username,
		Password: password, From: rec.From, UseTLS: rec.UseTLS,
	}, true, nil
}

// SaveSMTPConfigRecord validates and upserts the UI-managed config. An empty
// password keeps the stored one; a non-empty password is encrypted at rest.
func SaveSMTPConfigRecord(db *gorm.DB, rec SMTPConfigRecord) error {
	if rec.Enabled && (strings.TrimSpace(rec.Host) == "" || strings.TrimSpace(rec.From) == "") {
		return fmt.Errorf("启用时必须填写 SMTP 主机与发件人地址")
	}
	if rec.Port == 0 {
		rec.Port = 587
	}
	if rec.Port < 1 || rec.Port > 65535 {
		return fmt.Errorf("SMTP 端口不合法")
	}
	if rec.Password == "" {
		var s models.SystemSetting
		if err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigKey).First(&s).Error; err == nil {
			var old SMTPConfigRecord
			if json.Unmarshal([]byte(s.SettingValue), &old) == nil {
				rec.Password = old.Password // already-encrypted envelope
			}
		}
	} else {
		enc, err := EncryptSecret(rec.Password)
		if err != nil {
			return err
		}
		rec.Password = enc
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	var s models.SystemSetting
	err = db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigKey).First(&s).Error
	if err == nil {
		return db.Model(&s).Update("setting_value", string(raw)).Error
	}
	if err == gorm.ErrRecordNotFound {
		return db.Create(&models.SystemSetting{
			TenantID:     WarningConfigTenantID,
			SettingKey:   SMTPConfigKey,
			SettingValue: string(raw),
		}).Error
	}
	return err
}

// SendMailWithFallback sends via the UI-managed config (DirectMail) and falls
// back to the legacy env SMTP (personal mailbox) when the primary is missing,
// disabled or fails. Returns the channel actually used (#1910).
func SendMailWithFallback(to []string, subject, body string) (string, error) {
	var primaryErr error
	if db := database.GetDB(); db != nil {
		dbCfg, ok, err := LoadSMTPConfigFromDB(db)
		if err != nil {
			log.Printf("[Mail] UI-managed SMTP config unreadable, using env fallback: %v", err)
		} else if ok {
			if err := smtpSend(dbCfg, to, subject, body); err == nil {
				return "directmail", nil
			} else {
				primaryErr = err
				log.Printf("[Mail] primary (UI) send failed, falling back to env SMTP: %v", err)
			}
		}
	}
	envCfg := LoadSMTPConfigFromEnv()
	if envCfg.Configured() {
		if err := smtpSend(envCfg, to, subject, body); err != nil {
			return "", err
		}
		return "personal", nil
	}
	if primaryErr != nil {
		return "", primaryErr
	}
	return "", fmt.Errorf("SMTP 未配置（界面与 .env 均无可用配置）")
}

// WarningNotifyConfig is the per-level notification target configuration.
type WarningNotifyConfig struct {
	Enabled         bool     `json:"enabled"`
	Emails          []string `json:"emails"`
	CooldownMinutes int      `json:"cooldown_minutes"`
}

// LoadWarningNotifyConfig reads the single warning notification config;
// a missing row yields a zero (disabled) config without error.
func LoadWarningNotifyConfig(db *gorm.DB) (WarningNotifyConfig, error) {
	var cfg WarningNotifyConfig
	var s models.SystemSetting
	err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, WarningConfigKey).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(s.SettingValue) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(s.SettingValue), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid warning notification config JSON: %w", err)
	}
	return cfg, nil
}

// SaveWarningNotifyConfig validates and upserts the single config.
func SaveWarningNotifyConfig(db *gorm.DB, cfg WarningNotifyConfig) error {
	if len(cfg.Emails) > 50 {
		return fmt.Errorf("收件邮箱最多 50 个")
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(cfg.Emails))
	for _, e := range cfg.Emails {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if _, err := mail.ParseAddress(e); err != nil {
			return fmt.Errorf("邮箱格式不正确: %s", e)
		}
		// Recipients are intentionally NOT limited to platform users.
		if !seen[e] {
			seen[e] = true
			clean = append(clean, e)
		}
	}
	cfg.Emails = clean
	if cfg.CooldownMinutes < 0 || cfg.CooldownMinutes > 1440 {
		return fmt.Errorf("重复间隔必须在 0~1440 分钟之间")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	var s models.SystemSetting
	err = db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, WarningConfigKey).First(&s).Error
	if err == nil {
		return db.Model(&s).Update("setting_value", string(raw)).Error
	}
	if err == gorm.ErrRecordNotFound {
		return db.Create(&models.SystemSetting{
			TenantID:     WarningConfigTenantID,
			SettingKey:   WarningConfigKey,
			SettingValue: string(raw),
		}).Error
	}
	return err
}

// smtpSend is a test seam over SendSMTPMail.
var smtpSend = SendSMTPMail

// SendSMTPMail delivers a UTF-8 plain-text message via SMTP (STARTTLS when
// SMTP_USE_TLS is enabled, which is the default).
func SendSMTPMail(cfg SMTPConfig, to []string, subject, body string) error {
	if len(to) == 0 {
		return fmt.Errorf("no recipients")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	// Port 465 is implicit TLS (SMTPS, e.g. smtp.qq.com as used by the IAM
	// mailer); other TLS ports use STARTTLS.
	implicitTLS := cfg.UseTLS && cfg.Port == 465
	var conn net.Conn
	var err error
	if implicitTLS {
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host})
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Quit()

	if cfg.UseTLS && !implicitTLS {
		if err := c.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", r, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write([]byte(buildWarningMessage(cfg.From, to, subject, body))); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	return wc.Close()
}

func buildWarningMessage(from string, to []string, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body)
	return b.String()
}

func warningLevelLabel(level string) string {
	switch level {
	case models.WarningSeverityLow:
		return "低"
	case models.WarningSeverityMedium:
		return "中"
	case models.WarningSeverityHigh:
		return "高"
	}
	return level
}

// sendWarningEmail e-mails the configured recipients for the warning's level.
// Returns sent=false without error when the level is disabled/unconfigured or
// when the cooldown window has not elapsed; SMTP/configuration failures are
// returned so callers surface them instead of swallowing them (#1898).
func sendWarningEmail(w *models.Warning) (sent bool, recipients int, err error) {
	if w == nil {
		return false, 0, nil
	}
	db := database.GetDB()
	cfg, err := LoadWarningNotifyConfig(db)
	if err != nil {
		return false, 0, err
	}
	if !cfg.Enabled || len(cfg.Emails) == 0 {
		return false, 0, nil
	}
	if w.LastNotifiedAt != nil {
		// cooldown 0 = notify once per warning.
		if cfg.CooldownMinutes <= 0 || time.Since(*w.LastNotifiedAt) < time.Duration(cfg.CooldownMinutes)*time.Minute {
			return false, 0, nil
		}
	}
	subject := fmt.Sprintf("[TuneLoop 警告][%s] %s", warningLevelLabel(w.Level), w.Reason)
	body := fmt.Sprintf("级别: %s\n原因: %s\n分类: %s\n对象: %s/%s\n描述: %s\n时间: %s\n",
		warningLevelLabel(w.Level), w.Reason, w.Category, w.ObjectType, w.ObjectID,
		w.Description, w.CreatedAt.Format("2006-01-02 15:04:05"))
	channel, err := SendMailWithFallback(cfg.Emails, subject, body)
	if err != nil {
		log.Printf("[WarningEmail] send failed warning=%s level=%s: %v", w.ID, w.Level, err)
		return false, len(cfg.Emails), err
	}
	log.Printf("[WarningEmail] sent via %s warning=%s level=%s recipients=%d", channel, w.ID, w.Level, len(cfg.Emails))
	now := time.Now()
	if uerr := db.Model(&models.Warning{}).Where("id = ?", w.ID).Update("last_notified_at", now).Error; uerr != nil {
		log.Printf("[WarningEmail] failed to record last_notified_at warning=%s: %v", w.ID, uerr)
	}
	w.LastNotifiedAt = &now
	return true, len(cfg.Emails), nil
}
