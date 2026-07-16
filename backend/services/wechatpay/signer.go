package wechatpay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"sync"
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
		return nil, fmt.Errorf("read platform cert: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
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
