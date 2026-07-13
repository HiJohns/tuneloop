package wechatpay

import (
	"context"
	"fmt"
)

type realClient struct {
	cfg *Config
}

func newRealClient(cfg *Config) Client {
	return &realClient{cfg: cfg}
}

func (c *realClient) CreateJSAPIOrder(_ context.Context, params JSAPIParams) (*JSAPIResult, error) {
	return nil, fmt.Errorf("wechatpay real client: CreateJSAPIOrder not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) CreateNativeOrder(_ context.Context, params NativeParams) (*NativeResult, error) {
	return nil, fmt.Errorf("wechatpay real client: CreateNativeOrder not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) CreateH5Order(_ context.Context, params H5Params) (*H5Result, error) {
	return nil, fmt.Errorf("wechatpay real client: CreateH5Order not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) QueryOrder(_ context.Context, outTradeNo string) (*QueryResult, error) {
	return nil, fmt.Errorf("wechatpay real client: QueryOrder not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) CloseOrder(_ context.Context, outTradeNo string) error {
	return fmt.Errorf("wechatpay real client: CloseOrder not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) Refund(_ context.Context, params RefundParams) (*RefundResult, error) {
	return nil, fmt.Errorf("wechatpay real client: Refund not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) QueryRefund(_ context.Context, outRefundNo string) (*RefundResult, error) {
	return nil, fmt.Errorf("wechatpay real client: QueryRefund not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

func (c *realClient) VerifyPaymentCallback(_ context.Context, body []byte, signature, serial, timestamp, nonce string) (*CallbackResult, error) {
	return nil, fmt.Errorf("wechatpay real client: VerifyPaymentCallback not yet implemented; use WECHAT_PAY_MOCK_MODE=true in development")
}

var _ Client = (*realClient)(nil)
