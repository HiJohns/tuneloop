package wechatpay

import (
	"context"
	"testing"
)

// #2327：外部资质（service_id/商务准入）退化为配置项——默认走 mock，业务可先行开发。
func TestNewPayscoreProviderDefaultsToMock(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
	}{
		{"nil 配置", nil},
		{"零值配置（PAYSCORE_ENABLED 未开）", &Config{}},
		{"显式关闭", &Config{PayScoreEnabled: false, PayScoreServiceID: "SVC1"}},
	}
	for _, tc := range cases {
		p, err := NewPayscoreProvider(tc.cfg)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
		if !p.IsMock() {
			t.Fatalf("%s: want mock provider", tc.name)
		}
	}
}

// 负例：声明启用但缺少私钥时必须显式失败，不得静默降级为 mock（否则模拟结果会被当成真实评估）。
func TestNewPayscoreProviderEnabledWithoutKeyFails(t *testing.T) {
	_, err := NewPayscoreProvider(&Config{
		PayScoreEnabled:   true,
		PayScoreServiceID: "SVC1",
		PrivateKeyPath:    "/nonexistent/key.pem",
	})
	if err == nil {
		t.Fatal("expected error when enabled without a usable private key")
	}
}

// Mock 生命周期：创建 → 查询通过/拒绝 → 取消。
func TestMockPayscoreProviderLifecycle(t *testing.T) {
	ctx := context.Background()
	p := NewMockPayscoreProvider()

	order, err := p.CreateOrder(ctx, PayScoreCreateParams{
		ServiceID:  "SVC1",
		OutOrderNo: "O1",
		RiskFund:   PayScoreRiskFund{Name: RiskFundName, Amount: 3000},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if order.State != mockStatePending || order.RiskFund == nil || order.RiskFund.Amount != 3000 {
		t.Fatalf("unexpected create result: %+v", order)
	}

	q, err := p.QueryOrder(ctx, "SVC1", "O1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if q.State != mockStateApproved {
		t.Fatalf("default mock should approve, got %q", q.State)
	}

	// 负例：拒绝分支
	p.Approve = false
	q2, err := p.QueryOrder(ctx, "SVC1", "O1")
	if err != nil {
		t.Fatalf("query(reject): %v", err)
	}
	if q2.State != mockStateRejected {
		t.Fatalf("want rejected, got %q", q2.State)
	}

	if err := p.CancelOrder(ctx, "SVC1", "O1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// 负例：未知订单必须报错
	if err := p.CancelOrder(ctx, "SVC1", "missing"); err == nil {
		t.Fatal("cancel unknown order must fail")
	}
}
