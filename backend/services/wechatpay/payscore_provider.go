package wechatpay

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
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
	// ApproveAfter 让模拟评估在创建后保持 pending 一段时间（便于真机截图/观察）；
	// 0 = 立即出结果（默认，供单测使用）。
	ApproveAfter time.Duration
	createdAt    map[string]time.Time
}

func NewMockPayscoreProvider() *MockPayscoreProvider {
	return &MockPayscoreProvider{orders: map[string]*PayScoreOrder{}, Approve: true, createdAt: map[string]time.Time{}}
}

// MockApproveAfter 读取模拟评估的 pending 时长（PAYSCORE_MOCK_APPROVE_AFTER_SECONDS，默认 30s）。
// 仅影响 mock provider；真实 provider 由微信侧决定时长。
func MockApproveAfter() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("PAYSCORE_MOCK_APPROVE_AFTER_SECONDS")); err == nil && v >= 0 {
		return time.Duration(v) * time.Second
	}
	return 30 * time.Second
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
	m.createdAt[params.OutOrderNo] = time.Now()
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
	// 模拟评估：创建后 ApproveAfter 内保持 pending（默认 30s，便于真机截图）。
	if m.ApproveAfter > 0 {
		if created, ok := m.createdAt[outOrderNo]; ok && time.Since(created) < m.ApproveAfter {
			cp := *order
			cp.State = mockStatePending
			return &cp, nil
		}
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
		m := NewMockPayscoreProvider()
		m.ApproveAfter = MockApproveAfter()
		return m, nil
	}
	client, err := NewPayScoreClient(cfg)
	if err != nil {
		// 显式失败：已声明启用但凭证不全时不得静默降级为 mock（否则会把模拟结果当真实评估）。
		return nil, fmt.Errorf("payscore: enabled but client init failed: %w", err)
	}
	return client, nil
}
