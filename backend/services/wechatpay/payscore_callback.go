package wechatpay

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// 回调时间戳允许的最大偏移，防止重放。
const payScoreCallbackMaxSkew = 5 * time.Minute

// PayScoreCallbackHeaders 是微信回调请求头中与验签相关的字段。
type PayScoreCallbackHeaders struct {
	Signature string
	Serial    string
	Timestamp string
	Nonce     string
}

// callbackVerifier 抽象平台证书验签，便于测试注入。
type callbackVerifier func(timestamp, nonce string, body []byte, signature, serial string) error

// platformCertVerifier 使用微信平台证书校验回调签名（APIv3 规范：timestamp\nnonce\nbody\n）。
func platformCertVerifier(timestamp, nonce string, body []byte, signature, _ string) error {
	cert, err := getPlatformCert()
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("payscore callback: unexpected platform public key type")
	}
	message := timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	return verifySHA256WithRSA(pub, message, signature)
}

func checkCallbackSkew(timestamp string, now time.Time) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("payscore callback: invalid timestamp %q", timestamp)
	}
	skew := now.Sub(time.Unix(ts, 0))
	if skew > payScoreCallbackMaxSkew || skew < -payScoreCallbackMaxSkew {
		return fmt.Errorf("payscore callback: timestamp out of window (skew=%s)", skew)
	}
	return nil
}

// VerifyPayScoreCallback 校验并解密支付分回调，返回业务事件。
//
// 顺序：时间窗校验 → 平台证书验签 → 解密 resource → 解析事件。
// 任一步失败都返回错误（不静默吞错），调用方据此返回 4xx，让微信重试。
func (c *PayScoreClient) VerifyPayScoreCallback(headers PayScoreCallbackHeaders, body []byte) (*PayScoreCallback, error) {
	if err := checkCallbackSkew(headers.Timestamp, c.now()); err != nil {
		return nil, err
	}
	if c.verifier != nil {
		if err := c.verifier(headers.Timestamp, headers.Nonce, body, headers.Signature, headers.Serial); err != nil {
			return nil, fmt.Errorf("payscore callback: signature rejected: %w", err)
		}
	}

	var env PayScoreCallbackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("payscore callback: decode envelope: %w", err)
	}
	if env.Resource.Ciphertext == "" {
		return nil, fmt.Errorf("payscore callback: empty resource ciphertext")
	}

	var apiV3Key string
	if c.cfg != nil {
		apiV3Key = c.cfg.APIv3Key
	}
	plain, err := decryptCallbackResource(env.Resource.Nonce, env.Resource.AssociatedData, env.Resource.Ciphertext, apiV3Key)
	if err != nil {
		return nil, fmt.Errorf("payscore callback: decrypt resource: %w", err)
	}

	var out PayScoreCallback
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, fmt.Errorf("payscore callback: decode resource: %w", err)
	}
	if out.EventType == "" {
		out.EventType = env.EventType
	}
	return &out, nil
}
