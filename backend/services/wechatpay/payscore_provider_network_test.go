package wechatpay

import (
	"context"
	"os"
	"strings"
	"testing"
)

// #2327 R11：开关关闭时不得有任何支付分外部调用。
// 断言方式：默认 provider 必须是 mock，且 mock 实现内部不出现任何 http/net 调用点
// （静态可核验，避免“看起来是 mock 但偷偷发请求”）。
func TestProviderNoNetworkWhenDisabled(t *testing.T) {
	p, err := NewPayscoreProvider(&Config{PayScoreEnabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsMock() {
		t.Fatal("disabled payscore must use the mock provider")
	}

	src, err := os.ReadFile("payscore_provider.go")
	if err != nil {
		t.Fatalf("read provider source: %v", err)
	}
	text := string(src)
	start := strings.Index(text, "func (m *MockPayscoreProvider) CreateOrder")
	end := strings.Index(text, "func NewPayscoreProvider")
	if start < 0 || end <= start {
		t.Fatal("cannot locate mock implementation block")
	}
	mockBlock := text[start:end]
	for _, banned := range []string{"http.", "net/", "httpClient", "doer.Do"} {
		if strings.Contains(mockBlock, banned) {
			t.Fatalf("mock provider must not perform network I/O, found %q", banned)
		}
	}

	// 实际调用一遍，确保不会阻塞或外呼
	ctx := context.Background()
	if _, err := p.CreateOrder(ctx, PayScoreCreateParams{OutOrderNo: "R11"}); err != nil {
		t.Fatalf("mock create: %v", err)
	}
	if _, err := p.QueryOrder(ctx, "", "R11"); err != nil {
		t.Fatalf("mock query: %v", err)
	}
}
