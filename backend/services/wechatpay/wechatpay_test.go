package wechatpay

import (
	"context"
	"testing"
)

func TestMockClient_CreateJSAPIOrder(t *testing.T) {
	cfg := &Config{MockMode: true, AppID: "test_app"}
	client := NewClient(cfg)

	result, err := client.CreateJSAPIOrder(context.Background(), JSAPIParams{
		OutTradeNo:  "test_123",
		OpenID:      "mock_openid",
		TotalAmount: 100,
		Description: "test order",
		NotifyURL:   "https://test.com/notify",
	})
	if err != nil {
		t.Fatalf("CreateJSAPIOrder failed: %v", err)
	}
	if result.PrepayID == "" {
		t.Error("expected non-empty prepay_id")
	}
	if result.Package == "" {
		t.Error("expected non-empty package")
	}
}

func TestMockClient_CreateNativeOrder(t *testing.T) {
	cfg := &Config{MockMode: true}
	client := NewClient(cfg)

	result, err := client.CreateNativeOrder(context.Background(), NativeParams{
		OutTradeNo:  "test_456",
		TotalAmount: 200,
		Description: "test native",
	})
	if err != nil {
		t.Fatalf("CreateNativeOrder failed: %v", err)
	}
	if result.CodeURL == "" {
		t.Error("expected non-empty code_url")
	}
}

func TestMockClient_QueryOrder(t *testing.T) {
	cfg := &Config{MockMode: true}
	client := NewClient(cfg)

	result, err := client.QueryOrder(context.Background(), "test_789")
	if err != nil {
		t.Fatalf("QueryOrder failed: %v", err)
	}
	if result.TradeState != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", result.TradeState)
	}
}

func TestMockClient_Refund(t *testing.T) {
	cfg := &Config{MockMode: true}
	client := NewClient(cfg)

	result, err := client.Refund(context.Background(), RefundParams{
		OutTradeNo:   "test_refund",
		OutRefundNo:  "refund_001",
		TotalAmount:  100,
		RefundAmount: 50,
	})
	if err != nil {
		t.Fatalf("Refund failed: %v", err)
	}
	if result.RefundID == "" {
		t.Error("expected non-empty refund_id")
	}
	if result.Status != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", result.Status)
	}
}

func TestLoadConfig_MockMode(t *testing.T) {
	t.Setenv("WECHAT_PAY_MOCK_MODE", "true")
	t.Setenv("WX_APPID", "wx_test")

	cfg := LoadConfig()
	if !cfg.MockMode {
		t.Error("expected mock mode = true")
	}
	if cfg.AppID != "wx_test" {
		t.Errorf("expected wx_test, got %s", cfg.AppID)
	}
}

func TestAmountConversions(t *testing.T) {
	cfg := &Config{}
	if cfg.AmountToCents(10.50) != 1050 {
		t.Errorf("10.50 yuan = %d cents, expected 1050", cfg.AmountToCents(10.50))
	}
	if cfg.CentsToYuan(1050) != 10.50 {
		t.Errorf("1050 cents = %.2f yuan, expected 10.50", cfg.CentsToYuan(1050))
	}
}
