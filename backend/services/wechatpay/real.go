package wechatpay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type realClient struct {
	cfg  *Config
	http *httpClient
}

func newRealClient(cfg *Config) Client {
	hc, err := newHTTPClient(cfg)
	if err != nil {
		log.Printf("[wechatpay] failed to create HTTP client: %v, falling back to mock", err)
		return newMockClient(cfg)
	}
	return &realClient{cfg: cfg, http: hc}
}

// ---- JSAPI ----

type jsapiReq struct {
	AppID       string       `json:"appid"`
	MchID       string       `json:"mchid"`
	Description string       `json:"description"`
	OutTradeNo  string       `json:"out_trade_no"`
	NotifyURL   string       `json:"notify_url"`
	Amount      amountReq    `json:"amount"`
	Payer       jsapiPayer   `json:"payer"`
}

type amountReq struct {
	Total    int64  `json:"total"`
	Currency string `json:"currency"`
}

type jsapiPayer struct {
	OpenID string `json:"openid"`
}

type jsapiResp struct {
	PrepayID string `json:"prepay_id"`
}

func (c *realClient) CreateJSAPIOrder(ctx context.Context, params JSAPIParams) (*JSAPIResult, error) {
	reqBody := jsapiReq{
		AppID:       c.cfg.AppID,
		MchID:       c.cfg.MchID,
		Description: params.Description,
		OutTradeNo:  params.OutTradeNo,
		NotifyURL:   params.NotifyURL,
		Amount:      amountReq{Total: params.TotalAmount, Currency: "CNY"},
		Payer:       jsapiPayer{OpenID: params.OpenID},
	}
	var resp jsapiResp
	if _, err := c.http.do(ctx, "POST", "/v3/pay/transactions/jsapi", reqBody, &resp); err != nil {
		return nil, err
	}
	// Build prepay params for wechat JSAPI
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := fmt.Sprintf("%d", time.Now().UnixNano()%10000000000)
	pkg := "prepay_id=" + resp.PrepayID
	paySignStr := fmt.Sprintf("%s\n%s\n%s\n%s\n", c.cfg.AppID, ts, nonce, pkg)
	sig, err := signSHA256WithRSA(c.http.key, paySignStr)
	if err != nil {
		return nil, err
	}
	return &JSAPIResult{
		PrepayID:  resp.PrepayID,
		TimeStamp: ts,
		NonceStr:  nonce,
		Package:   pkg,
		SignType:  "RSA",
		Sign:      sig,
	}, nil
}

// ---- Native ----

type nativeResp struct {
	CodeURL string `json:"code_url"`
}

func (c *realClient) CreateNativeOrder(ctx context.Context, params NativeParams) (*NativeResult, error) {
	reqBody := map[string]interface{}{
		"appid":        c.cfg.AppID,
		"mchid":        c.cfg.MchID,
		"description":  params.Description,
		"out_trade_no": params.OutTradeNo,
		"notify_url":   params.NotifyURL,
		"amount":       amountReq{Total: params.TotalAmount, Currency: "CNY"},
	}
	var resp nativeResp
	if _, err := c.http.do(ctx, "POST", "/v3/pay/transactions/native", reqBody, &resp); err != nil {
		return nil, err
	}
	return &NativeResult{CodeURL: resp.CodeURL}, nil
}

// ---- H5 ----

type h5Req struct {
	AppID       string    `json:"appid"`
	MchID       string    `json:"mchid"`
	Description string    `json:"description"`
	OutTradeNo  string    `json:"out_trade_no"`
	NotifyURL   string    `json:"notify_url"`
	Amount      amountReq `json:"amount"`
	SceneInfo   h5Scene   `json:"scene_info"`
}

type h5Scene struct {
	PayerClientIP string `json:"payer_client_ip"`
	H5Info        h5Info `json:"h5_info"`
}

type h5Info struct {
	Type string `json:"type"`
}

type h5Resp struct {
	H5URL string `json:"h5_url"`
}

func (c *realClient) CreateH5Order(ctx context.Context, params H5Params) (*H5Result, error) {
	reqBody := h5Req{
		AppID:       c.cfg.AppID,
		MchID:       c.cfg.MchID,
		Description: params.Description,
		OutTradeNo:  params.OutTradeNo,
		NotifyURL:   params.NotifyURL,
		Amount:      amountReq{Total: params.TotalAmount, Currency: "CNY"},
		SceneInfo:   h5Scene{PayerClientIP: "127.0.0.1", H5Info: h5Info{Type: "Wap"}},
	}
	var resp h5Resp
	if _, err := c.http.do(ctx, "POST", "/v3/pay/transactions/h5", reqBody, &resp); err != nil {
		return nil, err
	}
	return &H5Result{H5URL: resp.H5URL}, nil
}

// ---- Query ----

type queryResp struct {
	TradeState    string `json:"trade_state"`
	TransactionID string `json:"transaction_id"`
	OutTradeNo    string `json:"out_trade_no"`
	Amount        struct {
		Total int64 `json:"total"`
	} `json:"amount"`
	SuccessTime string `json:"success_time"`
}

func (c *realClient) QueryOrder(ctx context.Context, outTradeNo string) (*QueryResult, error) {
	path := "/v3/pay/transactions/out-trade-no/" + outTradeNo
	queryParams := "?mchid=" + c.cfg.MchID
	var resp queryResp
	if _, err := c.http.do(ctx, "GET", path+queryParams, nil, &resp); err != nil {
		return nil, err
	}
	return &QueryResult{
		TradeState:    resp.TradeState,
		TransactionID: resp.TransactionID,
		OutTradeNo:    resp.OutTradeNo,
		Amount:        resp.Amount.Total,
		PaidAt:        resp.SuccessTime,
	}, nil
}

// ---- Close ----

func (c *realClient) CloseOrder(ctx context.Context, outTradeNo string) error {
	path := "/v3/pay/transactions/out-trade-no/" + outTradeNo + "/close"
	reqBody := map[string]string{"mchid": c.cfg.MchID}
	_, err := c.http.do(ctx, "POST", path, reqBody, nil)
	return err
}

// ---- Refund ----

type refundReq struct {
	OutTradeNo  string    `json:"out_trade_no,omitempty"`
	TransactionID string  `json:"transaction_id,omitempty"`
	OutRefundNo string    `json:"out_refund_no"`
	Reason      string    `json:"reason,omitempty"`
	NotifyURL   string    `json:"notify_url,omitempty"`
	Amount      refundAmt `json:"amount"`
}

type refundAmt struct {
	Refund    int64  `json:"refund"`
	Total     int64  `json:"total"`
	Currency  string `json:"currency"`
}

type refundResp struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"`
}

func (c *realClient) Refund(ctx context.Context, params RefundParams) (*RefundResult, error) {
	reqBody := refundReq{
		OutTradeNo:  params.OutTradeNo,
		OutRefundNo: params.OutRefundNo,
		Reason:      params.Reason,
		NotifyURL:   params.NotifyURL,
		Amount:      refundAmt{Refund: params.RefundAmount, Total: params.TotalAmount, Currency: "CNY"},
	}
	var resp refundResp
	if _, err := c.http.do(ctx, "POST", "/v3/refund/domestic/refunds", reqBody, &resp); err != nil {
		return nil, err
	}
	return &RefundResult{RefundID: resp.RefundID, Status: resp.Status}, nil
}

// ---- QueryRefund ----

func (c *realClient) QueryRefund(ctx context.Context, outRefundNo string) (*RefundResult, error) {
	path := "/v3/refund/domestic/refunds/" + outRefundNo
	var resp refundResp
	if _, err := c.http.do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return &RefundResult{RefundID: resp.RefundID, Status: resp.Status}, nil
}

// ---- VerifyPaymentCallback ----

type callbackNotify struct {
	ID           string    `json:"id"`
	CreateTime   string    `json:"create_time"`
	ResourceType string    `json:"resource_type"`
	EventType    string    `json:"event_type"`
	Summary      string    `json:"summary"`
	Resource     callbackResource `json:"resource"`
}

type callbackResource struct {
	Algorithm      string `json:"algorithm"`
	Ciphertext     string `json:"ciphertext"`
	AssociatedData string `json:"associated_data"`
	Nonce          string `json:"nonce"`
	OriginalType   string `json:"original_type"`
}

type transactionResult struct {
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	Amount        struct {
		Total int64 `json:"total"`
	} `json:"amount"`
}

type refundCallbackResult struct {
	OutTradeNo  string `json:"out_trade_no"`
	OutRefundNo string `json:"out_refund_no"`
	RefundID    string `json:"refund_id"`
	RefundStatus string `json:"refund_status"`
	Amount      struct {
		Total int64 `json:"total"`
	} `json:"amount"`
}

func (c *realClient) VerifyPaymentCallback(ctx context.Context, body []byte, signature, serial, timestamp, nonce string) (*CallbackResult, error) {
	// Step 1: Verify signature
	if err := verifyCallbackSignature(timestamp, nonce, string(body), signature); err != nil {
		return nil, fmt.Errorf("callback signature verification failed: %w", err)
	}

	// Step 2: Parse notification body
	var notif callbackNotify
	if err := json.Unmarshal(body, &notif); err != nil {
		return nil, fmt.Errorf("parse callback body: %w", err)
	}

	// Step 3: Decrypt resource
	plaintext, err := decryptCallbackResource(
		notif.Resource.Nonce,
		notif.Resource.AssociatedData,
		notif.Resource.Ciphertext,
		c.cfg.APIv3Key,
	)
	if err != nil {
		return nil, fmt.Errorf("decrypt resource: %w", err)
	}

	log.Printf("[wechatpay] callback event_type=%s plaintext=%s", notif.EventType, plaintext)

	// Step 4: Parse the decrypted content into result
	result := &CallbackResult{Success: true}

	switch notif.EventType {
	case "TRANSACTION.SUCCESS":
		var txn transactionResult
		if err := json.Unmarshal([]byte(plaintext), &txn); err != nil {
			return nil, fmt.Errorf("parse transaction result: %w", err)
		}
		result.OutTradeNo = txn.OutTradeNo
		result.TransactionID = txn.TransactionID
		result.Amount = txn.Amount.Total

	case "REFUND.SUCCESS":
		var ref refundCallbackResult
		if err := json.Unmarshal([]byte(plaintext), &ref); err != nil {
			return nil, fmt.Errorf("parse refund result: %w", err)
		}
		result.OutTradeNo = ref.OutTradeNo
		result.Success = true

	default:
		log.Printf("[wechatpay] unhandled event_type: %s", notif.EventType)
	}

	return result, nil
}

var _ Client = (*realClient)(nil)
