package tencentcloud

import "os"

// Config holds Tencent Cloud credentials shared across services (#1787/#1782).
type Config struct {
	SecretID  string
	SecretKey string
	Region    string
}

// LoadConfig reads Tencent Cloud credentials from environment variables.
func LoadConfig() Config {
	return Config{
		SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
		SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		Region:    getEnvOrDefault("TENCENTCLOUD_FACEID_REGION", "ap-guangzhou"),
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
