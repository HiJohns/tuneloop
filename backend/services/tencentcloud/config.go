package tencentcloud

import "os"

// Config holds Tencent Cloud credentials shared across services (#1787/#1782).
type Config struct {
	SecretID   string
	SecretKey  string
	Region     string
	EIDMerchantID string // #1807: E证通商户 ID（人脸核身控制台自助接入申请）
}

// LoadConfig reads Tencent Cloud credentials from environment variables.
func LoadConfig() Config {
	return Config{
		SecretID:   os.Getenv("TENCENTCLOUD_SECRET_ID"),
		SecretKey:  os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		Region:     getEnvOrDefault("TENCENTCLOUD_FACEID_REGION", "ap-guangzhou"),
		EIDMerchantID: os.Getenv("EID_MERCHANT_ID"),
	}
}

// Configured returns true if the essential credentials are set.
func (c Config) Configured() bool {
	return c.SecretID != "" && c.SecretKey != ""
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
