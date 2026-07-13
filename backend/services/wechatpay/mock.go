package wechatpay

import (
	"context"
	"fmt"
	"time"
)

type mockClient struct {
	cfg *Config
}

func newMockClient(cfg *Config) Client {
	return &mockClient{cfg: cfg}
}

func (m *mockClient) mockOrder() int64 {
	return int64(time.Now().UnixNano() % 1000000000)
}

func (m *mockClient) CreateJSAPIOrder(_ context.Context, params JSAPIParams) (*JSAPIResult, error) {
	prepayID := fmt.Sprintf("mock_prepay_%d", m.mockOrder())
	ts := fmt.Sprintf("%d", time.Now().Unix())
	return &JSAPIResult{
		PrepayID:  prepayID,
		Package:   "prepay_id=" + prepayID,
		TimeStamp: ts,
		NonceStr:  fmt.Sprintf("mock_nonce_%d", m.mockOrder()),
		SignType:  "RSA",
		Sign:      "mock_signature",
	}, nil
}

func (m *mockClient) CreateNativeOrder(_ context.Context, params NativeParams) (*NativeResult, error) {
	return &NativeResult{
		CodeURL: fmt.Sprintf("weixin://wxpay/bizpayurl?pr=mock_%d", m.mockOrder()),
	}, nil
}

func (m *mockClient) CreateH5Order(_ context.Context, params H5Params) (*H5Result, error) {
	return &H5Result{
		H5URL: fmt.Sprintf("https://wx.tenpay.com/mock/pay?order=%s", params.OutTradeNo),
	}, nil
}

func (m *mockClient) QueryOrder(_ context.Context, outTradeNo string) (*QueryResult, error) {
	return &QueryResult{
		TradeState:    "SUCCESS",
		TransactionID: "mock_transaction_" + outTradeNo,
		OutTradeNo:    outTradeNo,
		Amount:        0,
		PaidAt:        time.Now().Format(time.RFC3339),
	}, nil
}

func (m *mockClient) CloseOrder(_ context.Context, outTradeNo string) error {
	return nil
}

func (m *mockClient) Refund(_ context.Context, params RefundParams) (*RefundResult, error) {
	return &RefundResult{
		RefundID: fmt.Sprintf("mock_refund_%d", m.mockOrder()),
		Status:   "SUCCESS",
	}, nil
}

func (m *mockClient) QueryRefund(_ context.Context, outRefundNo string) (*RefundResult, error) {
	return &RefundResult{
		RefundID: "mock_refund_" + outRefundNo,
		Status:   "SUCCESS",
	}, nil
}

func (m *mockClient) VerifyPaymentCallback(_ context.Context, body []byte, signature, serial, timestamp, nonce string) (*CallbackResult, error) {
	return nil, fmt.Errorf("mock mode does not process real callbacks; call QueryOrder instead")
}

var _ Client = (*mockClient)(nil)
