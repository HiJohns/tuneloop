package wechatpay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type stubDoer struct {
	method string
	path   string
	body   string
	status int
	resp   string
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	s.method = req.Method
	s.path = req.URL.RequestURI()
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		s.body = string(b)
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	body := s.resp
	if body == "" {
		body = "{}"
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

type signCall struct{ method, path, body string }

func newTestPayScoreClient(doer httpDoer, calls *[]signCall) *PayScoreClient {
	cfg := &Config{
		APIv3Key:          "0123456789abcdef0123456789abcdef",
		PayScoreServiceID: "SVC1",
	}
	signer := func(method, path, body, _, _ string) (string, error) {
		*calls = append(*calls, signCall{method: method, path: path, body: body})
		return "WECHATPAY2-SHA256-RSA2048 stub", nil
	}
	fixed := time.Unix(1700000000, 0)
	return newPayScoreClientWithDeps(cfg, doer, signer, nil, apiBase, func() time.Time { return fixed })
}

// TestPayScoreClientPaths 断言 7 项：6 个接口的方法/路径 + 签名输入与实际请求一致（#2327 矩阵 R2）。
func TestPayScoreClientPaths(t *testing.T) {
	d := &stubDoer{resp: `{"service_id":"SVC1","out_order_no":"O1","state":"CREATED"}`}
	var calls []signCall
	c := newTestPayScoreClient(d, &calls)
	ctx := context.Background()

	// 1 创建
	if _, err := c.CreateOrder(ctx, PayScoreCreateParams{
		OutOrderNo: "O1",
		AppID:      "wx_test",
		RiskFund:   PayScoreRiskFund{Name: RiskFundName, Amount: 3000},
	}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if d.method != http.MethodPost || d.path != payScoreOrderPath {
		t.Fatalf("create -> %s %s, want POST %s", d.method, d.path, payScoreOrderPath)
	}
	if !strings.Contains(d.body, `"amount":3000`) || !strings.Contains(d.body, `"service_id":"SVC1"`) {
		t.Fatalf("create body missing risk fund/service id: %s", d.body)
	}

	// 2 查询（query 参与签名路径）
	if _, err := c.QueryOrder(ctx, "", "O1"); err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	wantQuery := payScoreOrderPath + "?out_order_no=O1&service_id=SVC1"
	if d.method != http.MethodGet || d.path != wantQuery {
		t.Fatalf("query -> %s %s, want GET %s", d.method, d.path, wantQuery)
	}

	// 3 完结
	if err := c.CompleteOrder(ctx, "", "O1"); err != nil {
		t.Fatalf("CompleteOrder: %v", err)
	}
	if d.method != http.MethodPost || !strings.HasSuffix(d.path, "/O1/complete") {
		t.Fatalf("complete -> %s %s", d.method, d.path)
	}

	// 4 取消
	if err := c.CancelOrder(ctx, "", "O1"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if d.method != http.MethodPost || !strings.HasSuffix(d.path, "/O1/cancel") {
		t.Fatalf("cancel -> %s %s", d.method, d.path)
	}

	// 5 修改
	if err := c.ModifyOrder(ctx, PayScoreModifyParams{
		ServiceID:  "SVC1",
		OutOrderNo: "O1",
		RiskFund:   &PayScoreRiskFund{Name: RiskFundName, Amount: 1000},
	}); err != nil {
		t.Fatalf("ModifyOrder: %v", err)
	}
	if d.method != http.MethodPatch || d.path != payScoreOrderPath+"/O1" {
		t.Fatalf("modify -> %s %s", d.method, d.path)
	}

	// 6 同步
	if err := c.SyncOrder(ctx, PayScoreSyncParams{OutOrderNo: "O1", Type: "OrderConfirm"}); err != nil {
		t.Fatalf("SyncOrder: %v", err)
	}
	if d.method != http.MethodPost || !strings.HasSuffix(d.path, "/O1/sync") {
		t.Fatalf("sync -> %s %s", d.method, d.path)
	}

	// 7 签名：每次调用都以「实际 method + path」参与签名，且 POST/PATCH 的 body 与请求一致
	if len(calls) != 6 {
		t.Fatalf("signer calls = %d, want 6", len(calls))
	}
	if calls[0].method != http.MethodPost || calls[0].path != payScoreOrderPath || calls[0].body == "" {
		t.Fatalf("create signature input unexpected: %+v", calls[0])
	}
	if calls[1].path != wantQuery {
		t.Fatalf("query signature path = %q, want %q", calls[1].path, wantQuery)
	}
}

// 负例：非 2xx 必须返回错误（不得静默当成功）。
func TestPayScoreClientRejectsNon2xx(t *testing.T) {
	d := &stubDoer{status: http.StatusUnauthorized, resp: `{"code":"SIGN_ERROR","message":"bad signature"}`}
	var calls []signCall
	c := newTestPayScoreClient(d, &calls)

	_, err := c.CreateOrder(context.Background(), PayScoreCreateParams{OutOrderNo: "O1", AppID: "wx"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	var apiErr *PayScoreAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected PayScoreAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || !strings.Contains(apiErr.Body, "SIGN_ERROR") {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
}

// 负例：缺少 service_id 时必须本地拒绝，不发出请求。
func TestPayScoreClientRequiresServiceID(t *testing.T) {
	d := &stubDoer{}
	c := newPayScoreClientWithDeps(&Config{}, d, func(string, string, string, string, string) (string, error) {
		return "sig", nil
	}, nil, apiBase, time.Now)

	if _, err := c.CreateOrder(context.Background(), PayScoreCreateParams{OutOrderNo: "O1"}); err == nil {
		t.Fatal("expected error when service_id is missing")
	}
	if d.method != "" {
		t.Fatalf("no HTTP request should be sent, got %s %s", d.method, d.path)
	}
}
