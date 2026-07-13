package wechatpay

import "context"

type JSAPIParams struct {
	OutTradeNo   string
	OpenID       string
	TotalAmount  int64  // cents
	Description  string
	NotifyURL    string
}

type NativeParams struct {
	OutTradeNo  string
	TotalAmount int64
	Description string
	NotifyURL   string
}

type H5Params struct {
	OutTradeNo  string
	TotalAmount int64
	Description string
	NotifyURL   string
}

type JSAPIResult struct {
	PrepayID  string `json:"prepay_id"`
	Sign      string `json:"sign"`
	TimeStamp string `json:"time_stamp"`
	NonceStr  string `json:"nonce_str"`
	Package   string `json:"package"`
	SignType  string `json:"sign_type"`
}

type NativeResult struct {
	CodeURL string `json:"code_url"`
}

type H5Result struct {
	H5URL string `json:"h5_url"`
}

type QueryResult struct {
	TradeState    string `json:"trade_state"`    // SUCCESS / REFUND / NOTPAY / CLOSED / REVOKED
	TransactionID string `json:"transaction_id"` // 微信支付单号
	OutTradeNo    string `json:"out_trade_no"`
	Amount        int64  `json:"amount"`         // cents
	PaidAt        string `json:"paid_at"`
}

type CallbackResult struct {
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Success       bool   `json:"success"`
}

type RefundParams struct {
	OutTradeNo  string
	OutRefundNo string
	TotalAmount int64  // cents, total amount of original order
	RefundAmount int64 // cents, amount to refund
	Reason      string
	NotifyURL   string
}

type RefundResult struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"` // PROCESSING / SUCCESS / CLOSED / ABNORMAL
}

type Client interface {
	CreateJSAPIOrder(ctx context.Context, params JSAPIParams) (*JSAPIResult, error)
	CreateNativeOrder(ctx context.Context, params NativeParams) (*NativeResult, error)
	CreateH5Order(ctx context.Context, params H5Params) (*H5Result, error)
	QueryOrder(ctx context.Context, outTradeNo string) (*QueryResult, error)
	CloseOrder(ctx context.Context, outTradeNo string) error
	Refund(ctx context.Context, params RefundParams) (*RefundResult, error)
	QueryRefund(ctx context.Context, outRefundNo string) (*RefundResult, error)
	VerifyPaymentCallback(ctx context.Context, body []byte, signature, serial, timestamp, nonce string) (*CallbackResult, error)
}
