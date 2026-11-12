package wechatpay

import (
	"context"
	"testing"
	"time"
)

// #2327 D0/R3: package 契约。
//   - 真实客户端必须把微信返回的 package 透出（前端 openBusinessView 唯一凭证）；
//   - mock provider 不返回真实 package，但必须 IsMock()=true。
func TestPayScorePackage(t *testing.T) {
	d := &stubDoer{resp: `{"service_id":"SVC1","out_order_no":"O1","package":"pkg-abc","state":"CREATED"}`}
	var calls []signCall
	c := newTestPayScoreClient(d, &calls)

	got, err := c.CreateOrder(context.Background(), PayScoreCreateParams{
		OutOrderNo: "O1",
		AppID:      "wx_test",
		RiskFund:   PayScoreRiskFund{Name: RiskFundName, Amount: 3000},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if got.Package != "pkg-abc" {
		t.Fatalf("package = %q, want pkg-abc", got.Package)
	}

	// 负例：package 缺失时调用方必须能识别（handler 会拒绝真实路径的空 package）。
	empty := &stubDoer{resp: `{"service_id":"SVC1","out_order_no":"O2","state":"CREATED"}`}
	var calls2 []signCall
	c2 := newTestPayScoreClient(empty, &calls2)
	got2, err := c2.CreateOrder(context.Background(), PayScoreCreateParams{
		OutOrderNo: "O2", RiskFund: PayScoreRiskFund{Name: RiskFundName, Amount: 1},
	})
	if err != nil {
		t.Fatalf("CreateOrder(empty package): %v", err)
	}
	if got2.Package != "" {
		t.Fatalf("empty package expected, got %q", got2.Package)
	}

	// mock：无真实 package，但必须标记 mock。
	m := NewMockPayscoreProvider()
	mo, err := m.CreateOrder(context.Background(), PayScoreCreateParams{OutOrderNo: "O3"})
	if err != nil {
		t.Fatalf("mock CreateOrder: %v", err)
	}
	if mo.Package != "" {
		t.Fatalf("mock must not fabricate a package, got %q", mo.Package)
	}
	if !m.IsMock() {
		t.Fatal("mock provider must report IsMock()=true")
	}
}

// PAYSCORE_PACKAGE_TTL_MINUTES 可配置，默认 30 分钟。
func TestPayScorePackageTTLConfigurable(t *testing.T) {
	t.Setenv("PAYSCORE_PACKAGE_TTL_MINUTES", "")
	if got := PayScorePackageTTL(); got != DefaultPayScorePackageTTL {
		t.Fatalf("default TTL = %v, want %v", got, DefaultPayScorePackageTTL)
	}
	t.Setenv("PAYSCORE_PACKAGE_TTL_MINUTES", "5")
	if got := PayScorePackageTTL(); got != 5*time.Minute {
		t.Fatalf("configured TTL = %v, want 5m", got)
	}
}

// PAYSCORE_ENFORCE 默认关闭（D12）。
func TestPayscoreEnforceDefaultOff(t *testing.T) {
	t.Setenv("PAYSCORE_ENFORCE", "")
	if PayscoreEnforceEnabled() {
		t.Fatal("PAYSCORE_ENFORCE must default to false")
	}
	t.Setenv("PAYSCORE_ENFORCE", "true")
	if !PayscoreEnforceEnabled() {
		t.Fatal("PAYSCORE_ENFORCE=true must enable the gate")
	}
}
