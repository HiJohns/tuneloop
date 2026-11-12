package wechatpay

import "context"

type JSAPIParams struct {
	OutTradeNo  string
	OpenID      string
	TotalAmount int64 // cents
	Description string
	NotifyURL   string
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
	Amount        int64  `json:"amount"` // cents
	PaidAt        string `json:"paid_at"`
}

type CallbackResult struct {
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Success       bool   `json:"success"`
	EventType     string `json:"event_type"`    // TRANSACTION.SUCCESS / REFUND.SUCCESS
	OutRefundNo   string `json:"out_refund_no"` // 退款单号（仅退款回调有值）
	RefundID      string `json:"refund_id"`     // 微信退款单号（仅退款回调有值）
	OpenID        string `json:"openid"`        // payer.openid（仅 TRANSACTION.SUCCESS 有值，#1731）
}

type RefundParams struct {
	OutTradeNo   string
	OutRefundNo  string
	TotalAmount  int64 // cents, total amount of original order
	RefundAmount int64 // cents, amount to refund
	Reason       string
	NotifyURL    string
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

// ---- 微信支付分（免押租借，#2327 S1）----

type PayScoreRiskFund struct {
	Name        string `json:"name"`
	Amount      int64  `json:"amount"` // 分
	Description string `json:"description,omitempty"`
}

type PayScoreTimeRange struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type PayScoreCreateParams struct {
	ServiceID  string            `json:"service_id"`
	OutOrderNo string            `json:"out_order_no"`
	AppID      string            `json:"appid"`
	RiskFund   PayScoreRiskFund  `json:"risk_fund"`
	TimeRange  PayScoreTimeRange `json:"time_range"`
	NotifyURL  string            `json:"notify_url"`
	OpenID     string            `json:"openid,omitempty"`
}

type PayScoreModifyParams struct {
	ServiceID  string             `json:"service_id"`
	OutOrderNo string             `json:"out_order_no"`
	RiskFund   *PayScoreRiskFund  `json:"risk_fund,omitempty"`
	TimeRange  *PayScoreTimeRange `json:"time_range,omitempty"`
}

type PayScoreSyncParams struct {
	ServiceID  string `json:"service_id"`
	OutOrderNo string `json:"out_order_no"`
	Type       string `json:"type"`
	Detail     string `json:"detail,omitempty"`
}

// PayScoreOrder 服务订单（微信返回的字段子集，按需扩展）。
type PayScoreOrder struct {
	AppID      string            `json:"appid,omitempty"`
	MchID      string            `json:"mchid,omitempty"`
	ServiceID  string            `json:"service_id"`
	OutOrderNo string            `json:"out_order_no"`
	OrderID    string            `json:"order_id,omitempty"`
	State      string            `json:"state,omitempty"` // CREATED/DOING/DONE/MORDIFY/CANCELED/EXPIRED
	RiskFund   *PayScoreRiskFund `json:"risk_fund,omitempty"`
	OpenID     string            `json:"openid,omitempty"`
}

// PayScoreCallbackEnvelope 是回调外层信封（resource 加密）。
type PayScoreCallbackEnvelope struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Resource  struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		AssociatedData string `json:"associated_data"`
		Nonce          string `json:"nonce"`
	} `json:"resource"`
}

// PayScoreCallback 是解密后的业务事件。
type PayScoreCallback struct {
	EventType      string `json:"event_type,omitempty"`
	ServiceID      string `json:"service_id,omitempty"`
	OutOrderNo     string `json:"out_order_no,omitempty"`
	OrderID        string `json:"order_id,omitempty"`
	State          string `json:"state,omitempty"`
	RiskFundAmount int64  `json:"risk_fund_amount,omitempty"`
	OpenID         string `json:"openid,omitempty"`
}
