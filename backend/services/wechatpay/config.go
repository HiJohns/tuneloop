package wechatpay

import (
	"os"
)

type Config struct {
	MchID              string
	AppID              string
	APIv3Key           string
	CertSerialNo       string
	PrivateKeyPath     string
	NotifyURL          string
	RefundNotifyURL    string
	MockMode           bool
}

func LoadConfig() *Config {
	appID := os.Getenv("WX_APPID")
	if appID == "" {
		appID = "wxcb44a1be70e356ed"
	}

	mockMode := false
	if v := os.Getenv("WECHAT_PAY_MOCK_MODE"); v == "true" || v == "1" {
		mockMode = true
	}
	mchID := os.Getenv("WECHAT_PAY_MCH_ID")
	if mchID == "" {
		mockMode = true
	}

	return &Config{
		MchID:           mchID,
		AppID:           appID,
		APIv3Key:        os.Getenv("WECHAT_PAY_API_V3_KEY"),
		CertSerialNo:    os.Getenv("WECHAT_PAY_CERT_SERIAL_NO"),
		PrivateKeyPath:  os.Getenv("WECHAT_PAY_PRIVATE_KEY_PATH"),
		NotifyURL:       os.Getenv("WECHAT_PAY_NOTIFY_URL"),
		RefundNotifyURL: os.Getenv("WECHAT_PAY_REFUND_NOTIFY_URL"),
		MockMode:        mockMode,
	}
}

func (c *Config) AmountToCents(yuan float64) int64 {
	return int64(yuan * 100 + 0.5)
}

func (c *Config) CentsToYuan(cents int64) float64 {
	return float64(cents) / 100
}
