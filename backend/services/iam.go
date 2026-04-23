package services

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

var (
	iamInternalURL = os.Getenv("BEACONIAM_INTERNAL_URL")
	iamExternalURL = os.Getenv("BEACONIAM_EXTERNAL_URL")
)

type IAMService struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	publicKey    *rsa.PublicKey
	keyOnce      sync.Once
	keyError     error
	keyLoaded    bool
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type PublicKeyResponse struct {
	Alg string `json:"alg"`
	Kty string `json:"kty"`
}

type JWTClaims struct {
	UserID   string `json:"sub"`
	TenantID string `json:"tid"`
	Role     string `json:"role"`
	IsOwner  bool   `json:"is_owner"`
	jwt.RegisteredClaims
}

func GetIAMInternalURL() string {
	if iamInternalURL != "" {
		return iamInternalURL
	}
	if iamExternalURL != "" {
		return iamExternalURL
	}
	return os.Getenv("IAM_URL")
}

// SetIAMInternalURLForTesting sets the IAM internal URL for testing purposes
func SetIAMInternalURLForTesting(url string) {
	iamInternalURL = url
}

func GetIAMExternalURL() string {
	if iamExternalURL != "" {
		return iamExternalURL
	}
	if iamInternalURL != "" {
		return iamInternalURL
	}
	return os.Getenv("IAM_URL")
}

func NewIAMService() *IAMService {
	return &IAMService{
		baseURL:      GetIAMInternalURL(),
		clientID:     os.Getenv("IAM_CLIENT_ID"),
		clientSecret: os.Getenv("IAM_CLIENT_SECRET"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *IAMService) loadPublicKey() error {
	s.keyOnce.Do(func() {
		s.keyError = s.fetchAndParsePublicKey()
		s.keyLoaded = s.keyError == nil
	})
	return s.keyError
}

func (s *IAMService) fetchAndParsePublicKey() error {
	resp, err := s.httpClient.Get(
		fmt.Sprintf("%s/api/v1/auth/public-key.pem", s.baseURL),
	)
	if err != nil {
		return fmt.Errorf("failed to fetch public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("public key endpoint returned status: %d", resp.StatusCode)
	}

	pemData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(pemData)
	if err != nil {
		return fmt.Errorf("failed to parse RSA public key: %w", err)
	}

	s.publicKey = pubKey
	return nil
}

func (s *IAMService) IsRS256Enabled() bool {
	if err := s.loadPublicKey(); err != nil {
		return false
	}
	return s.publicKey != nil
}

func (s *IAMService) ValidateToken(tokenString string) (*JWTClaims, error) {
	log.Printf("[ValidateToken] Starting validation, token length: %d", len(tokenString))

	// Parse token to inspect the header and determine signing method
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		log.Printf("[ValidateToken] Token signing method: %v", token.Header["alg"])

		// Handle RS256 (RSA with public key)
		if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
			if err := s.loadPublicKey(); err != nil {
				log.Printf("[ValidateToken] Failed to load public key for RS256: %v", err)
				return nil, fmt.Errorf("failed to load public key: %w", err)
			}
			return s.publicKey, nil
		}

		// Handle HS256 (HMAC with secret)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
			log.Printf("[ValidateToken] Using HS256 with client secret")
			return []byte(s.clientSecret), nil
		}

		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	})

	if err != nil {
		log.Printf("[ValidateToken] Token parse error: %v", err)
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		log.Printf("[ValidateToken] Token valid, sub=%s, tid=%s, role=%s", claims.Subject, claims.TenantID, claims.Role)
		return claims, nil
	}

	log.Printf("[ValidateToken] Token claims extraction failed")
	return nil, fmt.Errorf("invalid token")
}

func (s *IAMService) ExchangeCode(code string) (*TokenResponse, error) {
	return s.ExchangeCodeWithRedirect(code, "")
}

func (s *IAMService) ExchangeCodeWithRedirect(code string, redirectURI string) (*TokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	}

	if redirectURI != "" {
		payload["redirect_uri"] = redirectURI
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IAM token endpoint returned status: %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return &tokenResp, nil
}

func (s *IAMService) GetPublicKeyInfo() (*PublicKeyResponse, error) {
	resp, err := s.httpClient.Get(
		fmt.Sprintf("%s/api/v1/auth/public-key", s.baseURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public key info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public key endpoint returned status: %d", resp.StatusCode)
	}

	var result PublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse public key response: %w", err)
	}
	return &result, nil
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
