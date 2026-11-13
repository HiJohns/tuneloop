package wechatpay

import (
	"context"
	"fmt"
	"sync"
)

// PayscoreProvider 是支付分能力的唯一入口。业务层只依赖该接口：
//   - 生产：*PayScoreClient（真实微信调用，需 service_id + 商户私钥）
//   - 开发/联调/未获商务准入：*MockPayscoreProvider（进程内模拟，不发网络请求）
//
// 这样「外部资质」退化为一个配置项（PAYSCORE_ENABLED），业务代码可以先完成 80%，
// 获批后只需把 provider 换成真实实现，不改业务路径。
type PayscoreProvider interface {
	CreateOrder(ctx context.Context, params PayScoreCreateParams) (*PayScoreOrder, error)
	QueryOrder(ctx context.Context, serviceID, outOrderNo string) (*PayScoreOrder, error)
	CancelOrder(ctx context.Context, serviceID, outOrderNo string) error
	CompleteOrder(ctx context.Context, serviceID, outOrderNo string) error
	VerifyPayScoreCallback(headers PayScoreCallbackHeaders, body []byte) (*PayScoreCallback, error)
	// IsMock 让上层能在响应里标注「本次为模拟评估」，前端据此展示测试态。
	IsMock() bool
}

// 编译期断言：真实客户端实现接口。
var _ PayscoreProvider = (*PayScoreClient)(nil)

func (c *PayScoreClient) IsMock() bool { return false }

// mock 返回的 state 与 services.CreditState* 取值一致（避免导入 services 造成环依赖）。
const (
	mockStatePending  = "pending"
	mockStateApproved = "approved"
	mockStateRejected = "rejected"
)

// MockPayscoreProvider 进程内模拟支付分：创建订单后按 Approve 决定评估结果。
// 仅用于开发/测试；生产环境通过配置无法开启（见 NewPayscoreProvider）。
type MockPayscoreProvider struct {
	mu      sync.Mutex
	orders  map[string]*PayScoreOrder
	Approve bool
}

func NewMockPayscoreProvider() *MockPayscoreProvider {
	return &MockPayscoreProvider{orders: map[string]*PayScoreOrder{}, Approve: true}
}

func (m *MockPayscoreProvider) IsMock() bool { return true }

func (m *MockPayscoreProvider) CreateOrder(_ context.Context, params PayScoreCreateParams) (*PayScoreOrder, error) {
	if params.OutOrderNo == "" {
		return nil, fmt.Errorf("mock payscore: out_order_no is required")
	}
	order := &PayScoreOrder{
		ServiceID:  params.ServiceID,
		OutOrderNo: params.OutOrderNo,
		OrderID:    "mock-" + params.OutOrderNo,
		State:      mockStatePending,
		RiskFund:   &params.RiskFund,
		OpenID:     params.OpenID,
	}
	m.mu.Lock()
	m.orders[params.OutOrderNo] = order
	m.mu.Unlock()
	return order, nil
}

func (m *MockPayscoreProvider) QueryOrder(_ context.Context, _ /*serviceID*/, outOrderNo string) (*PayScoreOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[outOrderNo]
	if !ok {
		return nil, fmt.Errorf("mock payscore: order %q not found", outOrderNo)
	}
	state := mockStateRejected
	if m.Approve {
		state = mockStateApproved
	}
	cp := *order
	cp.State = state
	return &cp, nil
}

func (m *MockPayscoreProvider) CancelOrder(_ context.Context, _ /*serviceID*/, outOrderNo string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[outOrderNo]; !ok {
		return fmt.Errorf("mock payscore: order %q not found", outOrderNo)
	}
	return nil
}

func (m *MockPayscoreProvider) CompleteOrder(_ context.Context, _ /*serviceID*/, outOrderNo string) error {
	return m.CancelOrder(context.Background(), "", outOrderNo)
}

func (m *MockPayscoreProvider) VerifyPayScoreCallback(_ PayScoreCallbackHeaders, body []byte) (*PayScoreCallback, error) {
	return nil, fmt.Errorf("mock payscore: callbacks are not used in mock mode (body=%d bytes)", len(body))
}

// NewPayscoreProvider 按配置选择实现：默认 mock；仅当 PAYSCORE_ENABLED 且具备私钥时走真实客户端。
func NewPayscoreProvider(cfg *Config) (PayscoreProvider, error) {
	if cfg == nil || !cfg.PayScoreEnabled {
		return NewMockPayscoreProvider(), nil
	}
	client, err := NewPayScoreClient(cfg)
	if err != nil {
		// 显式失败：已声明启用但凭证不全时不得静默降级为 mock（否则会把模拟结果当真实评估）。
		return nil, fmt.Errorf("payscore: enabled but client init failed: %w", err)
	}
	return client, nil
}
