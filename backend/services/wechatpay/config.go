package wechatpay

import (
	"os"
	"strconv"
	"time"
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

	// 微信支付分（免押租借，#2327）。默认关闭，未获商务准入前不得对线上开启。
	PayScoreEnabled      bool
	PayScoreEnforce      bool
	PayScoreServiceID    string
	PayScoreRiskMaxCents int64
}

// truthyEnv 是环境变量布尔值解析的唯一实现（避免各处各写一套判断）。
func truthyEnv(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}

// PayscoreEnforceEnabled 报告是否强制 reduced/zero 档必须先通过信用评估。
//
// 与 PAYSCORE_ENABLED 的分工（#2327 D12）：
//   - PAYSCORE_ENABLED 只决定 provider（false → mock，零网络请求）；
//   - PAYSCORE_ENFORCE 决定支付闸门是否生效（默认 false = 现有行为不变）。
func PayscoreEnforceEnabled() bool {
	return truthyEnv(os.Getenv("PAYSCORE_ENFORCE"))
}

// DefaultPayScorePackageTTL 是拉起凭证 package 的默认有效期。
// 微信创建接口不返回过期时间，由本服务按配置保守估算（PAYSCORE_PACKAGE_TTL_MINUTES）。
const DefaultPayScorePackageTTL = 30 * time.Minute

// PayScorePackageTTL 返回 package 有效期的配置值（默认 30 分钟）。
func PayScorePackageTTL() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("PAYSCORE_PACKAGE_TTL_MINUTES")); err == nil && v > 0 {
		return time.Duration(v) * time.Minute
	}
	return DefaultPayScorePackageTTL
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

	payScoreEnabled := false
	switch os.Getenv("PAYSCORE_ENABLED") {
	case "1", "true", "TRUE", "yes":
		payScoreEnabled = true
	}
	riskMax, _ := strconv.ParseInt(os.Getenv("PAYSCORE_RISK_MAX_CENTS"), 10, 64)

	return &Config{
		MchID:           mchID,
		AppID:           appID,
		APIv3Key:        os.Getenv("WECHAT_PAY_API_V3_KEY"),
		CertSerialNo:    os.Getenv("WECHAT_PAY_CERT_SERIAL_NO"),
		PrivateKeyPath:  os.Getenv("WECHAT_PAY_PRIVATE_KEY_PATH"),
		NotifyURL:       baseURL + "/api/wechatpay/notify",
		RefundNotifyURL: baseURL + "/api/wechatpay/notify", // same URL, WeChat distinguishes by event_type
		MockMode:        false,

		PayScoreEnabled:      payScoreEnabled,
		PayScoreEnforce:      truthyEnv(os.Getenv("PAYSCORE_ENFORCE")),
		PayScoreServiceID:    os.Getenv("PAYSCORE_SERVICE_ID"),
		PayScoreRiskMaxCents: riskMax,
	}
}

func (c *Config) AmountToCents(yuan float64) int64 {
	return int64(yuan*100 + 0.5)
}

func (c *Config) CentsToYuan(cents int64) float64 {
	return float64(cents) / 100
}
