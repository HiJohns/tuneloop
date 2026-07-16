package wechatpay

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

func decryptCallbackResource(nonce, associatedData, ciphertext string, apiV3Key string) (string, error) {
	cipherBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	// WeChat Pay APIv3 uses AEAD_AES_256_GCM with the APIv3 key directly (not hashed)
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceBytes := []byte(nonce)
	plaintext, err := aesGCM.Open(nil, nonceBytes, cipherBytes, []byte(associatedData))
	if err != nil {
		return "", fmt.Errorf("GCM decrypt: %w", err)
	}

	return string(plaintext), nil
}
