package handlers

// #2272: 核身能力计算（前端按 capability 决定自动/手动通道）。

import "os"

// FaceVerifyCapability returns (provider, autoEnabled).
// provider ∈ manual|eid|faceid；autoEnabled 仅在腾讯云密钥就绪且
// provider 所需参数齐全时为 true；非法/缺参一律降级为 manual/false。
func FaceVerifyCapability() (string, bool) {
	provider := os.Getenv("FACE_VERIFY_PROVIDER")
	if provider == "" {
		provider = "manual"
	}
	configured := os.Getenv("TENCENTCLOUD_SECRET_ID") != "" && os.Getenv("TENCENTCLOUD_SECRET_KEY") != ""
	switch provider {
	case "faceid":
		return provider, configured
	case "eid":
		if configured && os.Getenv("EID_MERCHANT_ID") != "" {
			return provider, true
		}
		return "manual", false
	default:
		return "manual", false
	}
}
