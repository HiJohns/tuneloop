package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type IAMService struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type PublicKeyResponse struct {
	PublicKey string `json:"public_key"`
}

func NewIAMService() *IAMService {
	baseURL := os.Getenv("BEACONIAM_INTERNAL_URL")
	if baseURL == "" {
		baseURL = os.Getenv("BEACONIAM_EXTERNAL_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("IAM_URL")
	}

	return &IAMService{
		baseURL:      baseURL,
		clientID:     os.Getenv("IAM_CLIENT_ID"),
		clientSecret: os.Getenv("IAM_CLIENT_SECRET"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *IAMService) ExchangeCode(code string) (*TokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/token", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IAM token endpoint returned status: %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return &tokenResp, nil
}

func (s *IAMService) GetPublicKey() (string, error) {
	resp, err := s.httpClient.Get(
		fmt.Sprintf("%s/api/v1/auth/public-key", s.baseURL),
	)
	if err != nil {
		return "", fmt.Errorf("failed to fetch public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public key endpoint returned status: %d", resp.StatusCode)
	}

	var result PublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse public key response: %w", err)
	}
	return result.PublicKey, nil
}

func (s *IAMService) RefreshToken(refreshToken string) (*TokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/token", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call refresh endpoint: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}
	return &tokenResp, nil
}
