package wechatpay

import (
	"os"
)

type Config struct {
	MchID           string
	AppID           string
	APIv3Key        string
	CertSerialNo    string
	PrivateKeyPath  string
	NotifyURL       string
	RefundNotifyURL string
	MockMode        bool
}

func LoadConfig() *Config {
	appID := os.Getenv("WX_APPID")
	if appID == "" {
		appID = "wxcb44a1be70e356ed"
	}

	// 模拟支付已废弃（2026-08）：运行时一律真实微信支付。MockMode 仅保留给
	// 单元测试通过构造 Config 直接使用，环境变量无法再开启。
	mchID := os.Getenv("WECHAT_PAY_MCH_ID")

	// Callback URLs are fixed paths, domain derived from EXTERNAL_MOBILE_URL
	baseURL := os.Getenv("EXTERNAL_MOBILE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:5553"
	}

	return &Config{
		MchID:           mchID,
		AppID:           appID,
		APIv3Key:        os.Getenv("WECHAT_PAY_API_V3_KEY"),
		CertSerialNo:    os.Getenv("WECHAT_PAY_CERT_SERIAL_NO"),
		PrivateKeyPath:  os.Getenv("WECHAT_PAY_PRIVATE_KEY_PATH"),
		NotifyURL:       baseURL + "/api/wechatpay/notify",
		RefundNotifyURL: baseURL + "/api/wechatpay/notify", // same URL, WeChat distinguishes by event_type
		MockMode:        false,
	}
}

func (c *Config) AmountToCents(yuan float64) int64 {
	return int64(yuan*100 + 0.5)
}

func (c *Config) CentsToYuan(cents int64) float64 {
	return float64(cents) / 100
}
