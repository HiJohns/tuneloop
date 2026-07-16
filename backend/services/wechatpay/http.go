package wechatpay

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const apiBase = "https://api.mch.weixin.qq.com"

type httpClient struct {
	http     *http.Client
	mchID    string
	serialNo string
	key      *rsa.PrivateKey
}

func newHTTPClient(cfg *Config) (*httpClient, error) {
	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load key: %w", err)
	}
	return &httpClient{
		http:     &http.Client{Timeout: 30 * time.Second},
		mchID:    cfg.MchID,
		serialNo: cfg.CertSerialNo,
		key:      key,
	}, nil
}

func (c *httpClient) do(ctx context.Context, method, path string, body interface{}, result interface{}) (int, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("marshal body: %w", err)
		}
	}

	nonce := uuid.New().String()[:16]
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	bodyStr := string(bodyBytes)
	if bodyBytes == nil {
		bodyStr = ""
	}

	auth, err := buildAuthHeader(c.mchID, c.serialNo, method, path, timestamp, nonce, bodyStr, c.key)
	if err != nil {
		return 0, fmt.Errorf("build auth: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	// Log WeChat API response for debugging
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("wechat API error status=%d body=%s", resp.StatusCode, string(respBytes))
	}

	if result != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, result); err != nil {
			return resp.StatusCode, fmt.Errorf("unmarshal response: %w, body=%s", err, string(respBytes))
		}
	}

	// Verify WeChat response signature (not critical for flow, log only)
	wxSig := resp.Header.Get("Wechatpay-Signature")
	wxSerial := resp.Header.Get("Wechatpay-Serial")
	wxTimestamp := resp.Header.Get("Wechatpay-Timestamp")
	wxNonce := resp.Header.Get("Wechatpay-Nonce")
	if wxSig != "" && wxSerial != "" {
		if err := verifyResponseSignature(wxSerial, wxTimestamp, wxNonce, string(respBytes), wxSig); err != nil {
			log.Printf("[wechatpay] response signature verification failed: %v", err)
		}
	}

	return resp.StatusCode, nil
}

// WeChat platform certificate cache
var (
	platformCertPath = "/tmp/wechatpay_platform_cert.pem"
)

func getPlatformCertPath() string {
	if p := os.Getenv("WECHAT_PAY_PLATFORM_CERT_PATH"); p != "" {
		return p
	}
	return platformCertPath
}

func verifyResponseSignature(serialNo, timestamp, nonce, body, signatureB64 string) error {
	cert, err := getPlatformCert()
	if err != nil {
		return err
	}
	pubKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate public key is not RSA")
	}
	data := timestamp + "\n" + nonce + "\n" + body + "\n"
	return verifySHA256WithRSA(pubKey, data, signatureB64)
}

func verifyCallbackSignature(timestamp, nonce, body, signatureB64 string) error {
	cert, err := getPlatformCert()
	if err != nil {
		return err
	}
	pubKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate public key is not RSA")
	}
	data := timestamp + "\n" + nonce + "\n" + body + "\n"
	return verifySHA256WithRSA(pubKey, data, signatureB64)
}
