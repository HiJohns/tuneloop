package wechatpay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	certMu   sync.Mutex
	certData []byte
)

func getPlatformCert() (*x509.Certificate, error) {
	certMu.Lock()
	defer certMu.Unlock()

	path := getPlatformCertPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read platform cert: %w (run --download-platform-cert first)", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// DownloadAndSavePlatformCert downloads WeChat platform certificates and saves to disk.
// This is intended to be called from a one-off CLI flag (--download-platform-cert).
func DownloadAndSavePlatformCert() error {
	cfg := GetConfig()
	if cfg == nil || cfg.MchID == "" {
		return fmt.Errorf("no config available")
	}

	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	method := "GET"
	path := "/v3/certificates"
	nonce := uuid.New().String()[:16]
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	auth, err := buildAuthHeader(cfg.MchID, cfg.CertSerialNo, method, path, timestamp, nonce, "", key)
	if err != nil {
		return fmt.Errorf("build auth: %w", err)
	}

	req, _ := http.NewRequest(method, "https://api.mch.weixin.qq.com"+path, nil)
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body))
	}

	// Parse response: {"data":[{"serial_no":"...","encrypt_certificate":{...}}]}
	var result struct {
		Data []struct {
			SerialNo           string `json:"serial_no"`
			EncryptCertificate struct {
				Algorithm      string `json:"algorithm"`
				Nonce          string `json:"nonce"`
				AssociatedData string `json:"associated_data"`
				Ciphertext     string `json:"ciphertext"`
			} `json:"encrypt_certificate"`
			EffectiveTime string `json:"effective_time"`
			ExpireTime    string `json:"expire_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse cert response: %w", err)
	}

	if len(result.Data) == 0 {
		return fmt.Errorf("no certificates returned")
	}

	// Decrypt the first certificate
	cert := result.Data[0]
	plaintext, err := decryptCallbackResource(
		cert.EncryptCertificate.Nonce,
		cert.EncryptCertificate.AssociatedData,
		cert.EncryptCertificate.Ciphertext,
		cfg.APIv3Key,
	)
	if err != nil {
		return fmt.Errorf("decrypt cert: %w", err)
	}

	savePath := getPlatformCertPath()
	if err := os.WriteFile(savePath, []byte(plaintext), 0644); err != nil {
		return fmt.Errorf("write cert file: %w", err)
	}

	log.Printf("[wechatpay] platform cert downloaded (serial=%s) and saved to %s", cert.SerialNo, savePath)
	return nil
}

// loadPrivateKey reads an RSA private key from a PEM file.
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode PEM private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return rsaKey, nil
}

// signSHA256WithRSA signs data with RSA-SHA256 and returns base64 encoded signature.
func signSHA256WithRSA(key *rsa.PrivateKey, data string) (string, error) {
	hash := sha256.Sum256([]byte(data))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// buildAuthHeader builds the WECHATPAY2-SHA256-RSA2048 Authorization header.
// signature string = method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + body + "\n"
func buildAuthHeader(mchID, serialNo, method, path, timestamp, nonce, body string, key *rsa.PrivateKey) (string, error) {
	signStr := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n", method, path, timestamp, nonce, body)
	sig, err := signSHA256WithRSA(key, signStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`,
		mchID, nonce, timestamp, serialNo, sig), nil
}

// verifySHA256WithRSA verifies an RSA-SHA256 signature.
func verifySHA256WithRSA(pub *rsa.PublicKey, data, signatureB64 string) error {
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	hash := sha256.Sum256([]byte(data))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sig)
}
