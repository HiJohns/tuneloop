package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WeChat mini-program shipping compliance (#1693): physical-goods transactions
// must report shipping info (upload_shipping_info) at shipment and remind
// confirm-receive (notify_confirm_receive) at receipt, otherwise the platform
// keeps the funds frozen. Virtual/service transactions (membership, renewal)
// are exempt — callers only invoke these for physical flows (rent/repair).

// expressCompanyCodes maps common Chinese courier names to WeChat's
// express_company codes (物流公司ID from the WeChat express integration doc).
var expressCompanyCodes = map[string]string{
	"顺丰":  "SF",
	"顺丰速运": "SF",
	"圆通":  "YTO",
	"圆通速递": "YTO",
	"中通":  "ZTO",
	"中通快递": "ZTO",
	"申通":  "STO",
	"申通快递": "STO",
	"韵达":  "YUNDA",
	"韵达快递": "YUNDA",
	"百世":  "BEST",
	"德邦":  "DB",
	"德邦快递": "DB",
	"京东":  "JDL",
	"京东快递": "JDL",
	"极兔":  "JTSD",
	"极兔快递": "JTSD",
	"中国邮政": "EMS",
	"邮政":  "EMS",
	"EMS": "EMS",
	"优速":  "UCE",
	"天天":  "HHTT",
}

// ResolveExpressCompanyCode maps a courier display name to the WeChat
// express_company code; returns the input unchanged when unknown (WeChat
// still accepts unrecognized codes with a best-effort match).
func ResolveExpressCompanyCode(name string) string {
	if code, ok := expressCompanyCodes[name]; ok {
		return code
	}
	return name
}

type shippingOrderKey struct {
	OrderNumberType int    `json:"order_number_type"`
	Mchid           string `json:"mchid,omitempty"`
	OutTradeNo      string `json:"out_trade_no,omitempty"`
	TransactionID   string `json:"transaction_id,omitempty"`
}

type shippingPayer struct {
	OpenID string `json:"openid"`
}

type shippingItem struct {
	TrackingNo     string `json:"tracking_no,omitempty"`
	ExpressCompany string `json:"express_company,omitempty"`
	ItemDesc       string `json:"item_desc"`
}

type uploadShippingRequest struct {
	OrderKey       shippingOrderKey `json:"order_key"`
	LogisticsType  int              `json:"logistics_type"`
	DeliveryMode   int              `json:"delivery_mode"`
	ShippingList   []shippingItem   `json:"shipping_list"`
	UploadTime     string           `json:"upload_time"`
	Payer          shippingPayer    `json:"payer"`
	IsAllDelivered bool             `json:"is_all_delivered,omitempty"`
}

// UploadShippingInfo reports shipment to WeChat (upload_shipping_info) so the
// platform unfreezes funds for physical goods. Non-fatal by design — a report
// failure must not block the business shipment.
func UploadShippingInfo(openid, outTradeNo, transactionID, trackingNo, courierCompany, itemDesc string) error {
	token, err := GetWxAccessToken()
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	reqBody := uploadShippingRequest{
		OrderKey: shippingOrderKey{
			OrderNumberType: 1, // merchant-side out_trade_no form
			OutTradeNo:      outTradeNo,
		},
		LogisticsType: 1, // 实体物流配送
		DeliveryMode:  1, // 统一发货
		ShippingList: []shippingItem{{
			TrackingNo:     trackingNo,
			ExpressCompany: ResolveExpressCompanyCode(courierCompany),
			ItemDesc:       itemDesc,
		}},
		UploadTime: time.Now().Format(time.RFC3339),
		Payer:      shippingPayer{OpenID: openid},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal shipping request: %w", err)
	}
	url := fmt.Sprintf("%s/wxa/sec/order/upload_shipping_info?access_token=%s", wxAPIBaseURL, token)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("upload shipping info: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode upload shipping response: %w", err)
	}
	if result.Errcode != 0 {
		return fmt.Errorf("wechat upload_shipping_info error %d: %s", result.Errcode, result.Errmsg)
	}
	return nil
}

type notifyConfirmReceiveRequest struct {
	MerchantTradeNo string `json:"merchant_trade_no"`
	ReceivedTime    int64  `json:"received_time"`
}

// NotifyConfirmReceive reminds WeChat that the goods were signed (courier
// receipt) so the platform can settle funds; one call per order.
func NotifyConfirmReceive(outTradeNo string, receivedTime time.Time) error {
	token, err := GetWxAccessToken()
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	reqBody := notifyConfirmReceiveRequest{
		MerchantTradeNo: outTradeNo,
		ReceivedTime:    receivedTime.Unix(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal notify request: %w", err)
	}
	url := fmt.Sprintf("%s/wxa/sec/order/notify_confirm_receive?access_token=%s", wxAPIBaseURL, token)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify confirm receive: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode notify response: %w", err)
	}
	if result.Errcode != 0 {
		return fmt.Errorf("wechat notify_confirm_receive error %d: %s", result.Errcode, result.Errmsg)
	}
	return nil
}
