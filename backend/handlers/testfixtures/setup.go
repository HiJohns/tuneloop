package testfixtures

import (
	"testing"

	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// SetupIAMEnv prepares the IAM mock environment shared by tests that
// exercise IAM-dependent handlers (register, wx-login). Mirrors the
// setup used in auth_test.go.
func SetupIAMEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IAM_SECRET", "test-secret-1489")
	t.Setenv("IAM_NAMESPACE", "test-ns")
}

// SetupWechatPayMock initializes the wechatpay global with the loaded
// config. Since #1719 the runtime is always real-payment (WECHAT_PAY_MOCK_MODE
// is no longer read); tests that need a stubbed client call
// wechatpay.SetClientForTesting directly.
func SetupWechatPayMock(t *testing.T) {
	t.Helper()
	wechatpay.InitGlobal(wechatpay.LoadConfig())
}

// ResetIAMURLForTesting clears the mocked IAM internal URL after a test.
func ResetIAMURLForTesting(t *testing.T) {
	t.Helper()
	services.SetIAMInternalURLForTesting("")
}
