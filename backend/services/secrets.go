package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// #1910: at-rest encryption for stored secrets (SMTP password, webhook URLs).
// Key material precedence: CONFIG_ENCRYPTION_KEY (explicit) → IAM_SECRET
// (derived, so no new mandatory .env entry). Storing a secret is refused when
// neither is available.

const secretEnvelopePrefix = "enc:v1:"

func secretKey() ([]byte, error) {
	if v := strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")); v != "" {
		sum := sha256.Sum256([]byte("tuneloop-config-key:" + v))
		return sum[:], nil
	}
	if v := strings.TrimSpace(os.Getenv("IAM_SECRET")); v != "" {
		sum := sha256.Sum256([]byte("tuneloop-config-fallback:" + v))
		return sum[:], nil
	}
	return nil, fmt.Errorf("未配置 CONFIG_ENCRYPTION_KEY（且 IAM_SECRET 缺失），无法安全保存密钥类配置")
}

// EncryptSecret seals plain with AES-256-GCM and returns a prefixed base64
// envelope. Empty input returns an empty string (nothing to store).
func EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := secretKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretEnvelopePrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret opens an envelope produced by EncryptSecret. Plain values
// without the envelope prefix are returned as-is (legacy/manual entries).
func DecryptSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, secretEnvelopePrefix) {
		return stored, nil
	}
	key, err := secretKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, secretEnvelopePrefix))
	if err != nil {
		return "", fmt.Errorf("密钥配置解码失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密钥配置格式不正确")
	}
	nonce, cipherText := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", fmt.Errorf("密钥解密失败（加密密钥可能已变更，请重新保存该配置）: %w", err)
	}
	return string(plain), nil
}
