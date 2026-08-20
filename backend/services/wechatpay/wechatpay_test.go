package wechatpay

import (
	"testing"
)

func TestAmountConversions(t *testing.T) {
	cfg := &Config{}
	if cfg.AmountToCents(10.50) != 1050 {
		t.Errorf("10.50 yuan = %d cents, expected 1050", cfg.AmountToCents(10.50))
	}
	if cfg.CentsToYuan(1050) != 10.50 {
		t.Errorf("1050 cents = %.2f yuan, expected 10.50", cfg.CentsToYuan(1050))
	}
}
