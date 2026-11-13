package wechatpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// 微信支付分（免押租借）服务端接口前缀。
const payScoreOrderPath = "/v3/payscore/serviceorder"

// httpDoer 抽出 *http.Client，便于测试注入假响应。
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// payScoreSigner 生成 Authorization 头；测试注入固定实现以断言签名输入。
type payScoreSigner func(method, path, body, nonce, timestamp string) (string, error)

// PayScoreClient 微信支付分客户端。
//
// 注意：本客户端只负责与微信通信（创建/查询/完结/取消/修改/同步/回调验签），
// 不含任何押金业务规则；档位与风险金额由调用方（ComputeRiskAmount）决定。
type PayScoreClient struct {
	cfg      *Config
	doer     httpDoer
	signer   payScoreSigner
	verifier callbackVerifier
	baseURL  string
	now      func() time.Time
}

// PayScoreAPIError 保存微信返回的非 2xx 响应，避免静默吞错。
type PayScoreAPIError struct {
	StatusCode int
	Body       string
}

func (e *PayScoreAPIError) Error() string {
	return fmt.Sprintf("payscore api error: status=%d body=%s", e.StatusCode, e.Body)
}

// NewPayScoreClient 构造生产客户端（需要商户私钥）。
func NewPayScoreClient(cfg *Config) (*PayScoreClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("payscore: nil config")
	}
	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("payscore: load private key: %w", err)
	}
	c := &PayScoreClient{
		cfg:      cfg,
		doer:     &http.Client{Timeout: 30 * time.Second},
		verifier: platformCertVerifier,
		baseURL:  apiBase,
		now:      time.Now,
	}
	c.signer = func(method, path, body, nonce, timestamp string) (string, error) {
		return buildAuthHeader(cfg.MchID, cfg.CertSerialNo, method, path, timestamp, nonce, body, key)
	}
	return c, nil
}

// newPayScoreClientWithDeps 仅用于测试：注入 doer/signer/verifier/baseURL。
func newPayScoreClientWithDeps(cfg *Config, doer httpDoer, signer payScoreSigner, verifier callbackVerifier, baseURL string, now func() time.Time) *PayScoreClient {
	if now == nil {
		now = time.Now
	}
	return &PayScoreClient{cfg: cfg, doer: doer, signer: signer, verifier: verifier, baseURL: baseURL, now: now}
}

func (c *PayScoreClient) serviceID(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if c.cfg != nil {
		return c.cfg.PayScoreServiceID
	}
	return ""
}

func (c *PayScoreClient) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("payscore: marshal body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("payscore: new request: %w", err)
	}

	nonce := uuid.New().String()[:16]
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	auth, err := c.signer(method, path, string(raw), nonce, timestamp)
	if err != nil {
		return fmt.Errorf("payscore: sign request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doer.Do(req)
	if err != nil {
		return fmt.Errorf("payscore: http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("payscore: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &PayScoreAPIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("payscore: decode response: %w", err)
	}
	return nil
}

// CreateOrder 创建支付分服务订单（POST /v3/payscore/serviceorder）。
func (c *PayScoreClient) CreateOrder(ctx context.Context, params PayScoreCreateParams) (*PayScoreOrder, error) {
	if params.ServiceID == "" {
		params.ServiceID = c.serviceID("")
	}
	if params.ServiceID == "" {
		return nil, fmt.Errorf("payscore: service_id is required")
	}
	if params.OutOrderNo == "" {
		return nil, fmt.Errorf("payscore: out_order_no is required")
	}
	var out PayScoreOrder
	if err := c.do(ctx, http.MethodPost, payScoreOrderPath, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryOrder 查询服务订单（GET /v3/payscore/serviceorder?service_id=&out_order_no=）。
func (c *PayScoreClient) QueryOrder(ctx context.Context, serviceID, outOrderNo string) (*PayScoreOrder, error) {
	sid := c.serviceID(serviceID)
	if sid == "" || outOrderNo == "" {
		return nil, fmt.Errorf("payscore: service_id and out_order_no are required")
	}
	q := url.Values{}
	q.Set("service_id", sid)
	q.Set("out_order_no", outOrderNo)
	var out PayScoreOrder
	if err := c.do(ctx, http.MethodGet, payScoreOrderPath+"?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CompleteOrder 完结服务订单（用户已履约）。
func (c *PayScoreClient) CompleteOrder(ctx context.Context, serviceID, outOrderNo string) error {
	sid := c.serviceID(serviceID)
	if sid == "" || outOrderNo == "" {
		return fmt.Errorf("payscore: service_id and out_order_no are required")
	}
	path := fmt.Sprintf("%s/%s/complete", payScoreOrderPath, url.PathEscape(outOrderNo))
	return c.do(ctx, http.MethodPost, path, map[string]string{"service_id": sid}, nil)
}

// CancelOrder 取消服务订单（未履约，释放风险金额）。
func (c *PayScoreClient) CancelOrder(ctx context.Context, serviceID, outOrderNo string) error {
	sid := c.serviceID(serviceID)
	if sid == "" || outOrderNo == "" {
		return fmt.Errorf("payscore: service_id and out_order_no are required")
	}
	path := fmt.Sprintf("%s/%s/cancel", payScoreOrderPath, url.PathEscape(outOrderNo))
	return c.do(ctx, http.MethodPost, path, map[string]string{"service_id": sid}, nil)
}

// ModifyOrder 修改服务订单（风险金额/服务时间）。
func (c *PayScoreClient) ModifyOrder(ctx context.Context, params PayScoreModifyParams) error {
	if params.ServiceID == "" {
		params.ServiceID = c.serviceID("")
	}
	if params.ServiceID == "" || params.OutOrderNo == "" {
		return fmt.Errorf("payscore: service_id and out_order_no are required")
	}
	path := fmt.Sprintf("%s/%s", payScoreOrderPath, url.PathEscape(params.OutOrderNo))
	return c.do(ctx, http.MethodPatch, path, params, nil)
}

// SyncOrder 同步服务订单状态（履约进度上报）。
func (c *PayScoreClient) SyncOrder(ctx context.Context, params PayScoreSyncParams) error {
	if params.ServiceID == "" {
		params.ServiceID = c.serviceID("")
	}
	if params.ServiceID == "" || params.OutOrderNo == "" {
		return fmt.Errorf("payscore: service_id and out_order_no are required")
	}
	path := fmt.Sprintf("%s/%s/sync", payScoreOrderPath, url.PathEscape(params.OutOrderNo))
	return c.do(ctx, http.MethodPost, path, params, nil)
}
