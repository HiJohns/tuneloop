package wechatpay

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const testAPIv3Key = "0123456789abcdef0123456789abcdef" // 32 字节

func encryptResource(t *testing.T, nonce, aad, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(testAPIv3Key))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	sealed := gcm.Seal(nil, []byte(nonce), []byte(plaintext), []byte(aad))
	return base64.StdEncoding.EncodeToString(sealed)
}

func callbackBody(t *testing.T, eventType, nonce, aad, ciphertext string) []byte {
	t.Helper()
	env := PayScoreCallbackEnvelope{ID: "evt-1", EventType: eventType}
	env.Resource.Algorithm = "AEAD_AES_256_GCM"
	env.Resource.Ciphertext = ciphertext
	env.Resource.AssociatedData = aad
	env.Resource.Nonce = nonce
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return raw
}

func newCallbackTestClient() *PayScoreClient {
	fixed := time.Unix(1700000000, 0)
	acceptAll := func(timestamp, nonce string, body []byte, signature, serial string) error {
		if signature != "good-signature" {
			return fmt.Errorf("bad signature")
		}
		return nil
	}
	return newPayScoreClientWithDeps(
		&Config{APIv3Key: testAPIv3Key},
		&stubDoer{}, func(string, string, string, string, string) (string, error) { return "sig", nil },
		acceptAll, apiBase, func() time.Time { return fixed },
	)
}

// TestVerifyPayScoreCallback 覆盖：合法通过 / 篡改密文拒绝 / 过期时间戳拒绝 / 签名拒绝（#2327 矩阵 R3）。
func TestVerifyPayScoreCallback(t *testing.T) {
	nonce := "abcdef123456"
	aad := "transaction"
	plain := `{"out_order_no":"O1","service_id":"SVC1","state":"DOING","risk_fund_amount":3000}`
	ciphertext := encryptResource(t, nonce, aad, plain)

	c := newCallbackTestClient()
	headers := PayScoreCallbackHeaders{
		Signature: "good-signature",
		Serial:    "serial-1",
		Timestamp: "1700000000",
		Nonce:     "cb-nonce",
	}

	// 1 合法
	got, err := c.VerifyPayScoreCallback(headers, callbackBody(t, "PAYSCORE.USER_OPEN_SERVICE", nonce, aad, ciphertext))
	if err != nil {
		t.Fatalf("valid callback rejected: %v", err)
	}
	if got.OutOrderNo != "O1" || got.ServiceID != "SVC1" || got.RiskFundAmount != 3000 {
		t.Fatalf("decoded callback mismatch: %+v", got)
	}
	if got.EventType != "PAYSCORE.USER_OPEN_SERVICE" {
		t.Fatalf("event type = %q", got.EventType)
	}

	// 2 负例：篡改密文必须解密失败
	tampered := encryptResource(t, nonce, aad, plain)
	if tampered == "" {
		t.Fatal("unexpected empty ciphertext")
	}
	rawTampered := []byte(tampered)
	rawTampered[0] ^= 0xFF
	if _, err := c.VerifyPayScoreCallback(headers, callbackBody(t, "PAYSCORE.USER_OPEN_SERVICE", nonce, aad, string(rawTampered))); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}

	// 3 负例：过期时间戳（超过 5 分钟窗口）
	stale := headers
	stale.Timestamp = "1699999000" // now - 1000s
	if _, err := c.VerifyPayScoreCallback(stale, callbackBody(t, "PAYSCORE.USER_OPEN_SERVICE", nonce, aad, ciphertext)); err == nil {
		t.Fatal("stale timestamp must fail")
	}

	// 4 负例：签名不通过
	bad := headers
	bad.Signature = "forged"
	if _, err := c.VerifyPayScoreCallback(bad, callbackBody(t, "PAYSCORE.USER_OPEN_SERVICE", nonce, aad, ciphertext)); err == nil {
		t.Fatal("forged signature must fail")
	}
}
