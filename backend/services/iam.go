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
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

// IAMAPIError is a structured error returned when an IAM API call fails with
// a non-200 status. ErrorCode is the "error" field parsed from the IAM JSON
// error body, allowing handlers to map it to user-friendly messages.
type IAMAPIError struct {
	StatusCode int
	ErrorCode  string
	Body       string
}

func (e *IAMAPIError) Error() string {
	return fmt.Sprintf("IAM returned status: %d, body: %s", e.StatusCode, e.Body)
}

// newIAMAPIError builds an IAMAPIError from a non-200 response body, parsing
// the "error" field if the body is JSON.
func newIAMAPIError(statusCode int, body []byte) *IAMAPIError {
	var parsed struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	return &IAMAPIError{
		StatusCode: statusCode,
		ErrorCode:  parsed.Error,
		Body:       string(body),
	}
}

var (
	iamInternalURL              = os.Getenv("BEACONIAM_INTERNAL_URL")
	iamExternalURL              = os.Getenv("BEACONIAM_EXTERNAL_URL")
	publicKeyRefreshInterval    = 60 * time.Second
	publicKeyMinRefreshInterval = 5 * time.Second
)

type IAMService struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	publicKey    *rsa.PublicKey
	keyMutex     sync.Mutex
	keyError     error
	keyLoaded    bool
	lastRefresh  time.Time

	// refreshInterval is the minimum time between public key refreshes
	// to prevent thundering herd against IAM. Default is 60s.
	refreshInterval time.Duration
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	WxOpenid     string `json:"wx_openid"`
}

// WxAccount is one account bound to a WeChat openid (beaconiam multi-binding).
type WxAccount struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id"`
	TenantID string `json:"tenant_id"`
}

// WxAccountsResult is the response from IAM GET /api/v1/auth/wx-accounts.
type WxAccountsResult struct {
	OpenID        string      `json:"openid"`
	Accounts      []WxAccount `json:"accounts"`
	ExchangeToken string      `json:"exchange_token"`
}

type PublicKeyResponse struct {
	Alg string `json:"alg"`
	Kty string `json:"kty"`
}

type JWTClaims struct {
	UserID      string   `json:"sub"`
	TenantID    string   `json:"tid"`
	OrgID       string   `json:"oid"` // Organization ID (IAM token format)
	Gid         string   `json:"gid"` // Group ID (current organization)
	NamespaceID string   `json:"nid"`
	Role        string   `json:"role"`
	Name        string   `json:"name"`
	IsOwner     bool     `json:"is_owner"`
	Roles       []string `json:"roles"`    // Functional roles
	SysPerm     int64    `json:"sys_perm"` // System permission bitmap
	CusPerm     int64    `json:"cus_perm"` // Customer permission bitmap
	CusPermExt  string   `json:"cus_perm_ext,omitempty"`
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
	clientID := os.Getenv("IAM_NAMESPACE")
	if clientID == "" {
		clientID = os.Getenv("IAM_PC_CLIENT_ID")
	}
	if clientID == "" {
		clientID = os.Getenv("IAM_CLIENT_ID")
	}
	clientSecret := os.Getenv("IAM_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("IAM_PC_CLIENT_SECRET")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("IAM_CLIENT_SECRET")
	}
	if clientID == "" || clientSecret == "" {
		log.Fatal("IAM client credentials not configured: set IAM_NAMESPACE + IAM_SECRET, or IAM_PC_CLIENT_ID + IAM_PC_CLIENT_SECRET")
	}
	return &IAMService{
		baseURL:         GetIAMInternalURL(),
		clientID:        clientID,
		clientSecret:    clientSecret,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		refreshInterval: publicKeyRefreshInterval,
	}
}

// loadPublicKey loads the public key with debouncing to prevent thundering herd
func (s *IAMService) loadPublicKey() error {
	now := time.Now()

	s.keyMutex.Lock()
	defer s.keyMutex.Unlock()

	// Check if we should skip refresh due to debouncing
	if s.lastRefresh.IsZero() || now.Sub(s.lastRefresh) >= s.refreshInterval {
		s.lastRefresh = now
		s.keyError = s.fetchAndParsePublicKey()
		s.keyLoaded = s.keyError == nil
	}
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
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
			if err := s.loadPublicKey(); err != nil {
				log.Printf("[ValidateToken] Failed to load public key for RS256: %v", err)
				return nil, fmt.Errorf("failed to load public key: %w", err)
			}
			return s.publicKey, nil
		}

		// Handle HS256 (HMAC with secret)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
			return []byte(s.clientSecret), nil
		}

		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	})

	if err != nil {
		log.Printf("[ValidateToken] Token parse error: %v", err)

		// If RS256 signature verification failed, retry once after refreshing public key
		if strings.Contains(err.Error(), "crypto/rsa: verification error") || strings.Contains(err.Error(), "signature is invalid") {
			log.Printf("[ValidateToken] Signature verification failed, attempting public key refresh and retry")

			s.keyMutex.Lock()
			s.lastRefresh = time.Time{}
			s.keyMutex.Unlock()

			token, err = jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
					if err := s.loadPublicKey(); err != nil {
						log.Printf("[ValidateToken] Failed to reload public key: %v", err)
						return nil, fmt.Errorf("failed to reload public key: %w", err)
					}
					return s.publicKey, nil
				}
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			})

			if err != nil {
				log.Printf("[ValidateToken] Retry failed: %v", err)
				return nil, fmt.Errorf("token verification failed after key refresh: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to parse token: %w", err)
		}
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// CreateGuestToken issues a locally-signed GUEST JWT (HS256)
func (s *IAMService) CreateGuestToken() (*TokenResponse, error) {
	now := time.Now()
	claims := JWTClaims{
		Role: "GUEST",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        fmt.Sprintf("guest-%d", now.UnixNano()),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(720 * time.Hour)), // 30 days
			Issuer:    "tuneloop",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.clientSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign GUEST token: %w", err)
	}
	return &TokenResponse{
		AccessToken: signed,
		TokenType:   "Bearer",
		ExpiresIn:   720 * 3600,
	}, nil
}

// IAMLogin proxies email/password login to beaconiam via OAuth password grant
func (s *IAMService) IAMLogin(identifier, password string) (*TokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "password",
		"username":      identifier,
		"password":      password,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	}
	jsonBody, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/token", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IAM login returned status: %d, body: %s", resp.StatusCode, string(body))
	}
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse IAM login response: %w", err)
	}
	return &tokenResp, nil
}

// IAMRegister proxies user registration to beaconiam
func (s *IAMService) IAMRegister(name, phone, email, password string) (*TokenResponse, error) {
	payload := map[string]string{
		"name":     name,
		"phone":    phone,
		"email":    email,
		"password": password,
	}
	jsonPayload, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/register", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IAM register returned status: %d, body: %s", resp.StatusCode, string(body))
	}
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse IAM register response: %w", err)
	}
	return &tokenResp, nil
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

func (s *IAMService) WxLogin(code string) (*TokenResponse, error) {
	payload := map[string]string{
		"code":      code,
		"client_id": s.clientID,
	}

	jsonPayload, _ := json.Marshal(payload)
	wxURL := fmt.Sprintf("%s/api/v1/auth/wx-login", s.baseURL)
	log.Printf("[IAM DEBUG] WxLogin calling %s with code len=%d", wxURL, len(code))
	resp, err := s.httpClient.Post(
		wxURL,
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		log.Printf("[IAM DEBUG] WxLogin HTTP error: %v", err)
		return nil, fmt.Errorf("failed to call IAM wx-login endpoint: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[IAM DEBUG] WxLogin response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[IAM DEBUG] WxLogin non-200 body: %s", string(body))
		return nil, fmt.Errorf("IAM wx-login returned status: %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse wx-login response: %w", err)
	}
	return &tokenResp, nil
}

// WxAccounts exchanges a WeChat login code for the openid and returns every
// account bound to it (0/1/N). Used for login routing and the register-time
// "one customer per openid" check.
func (s *IAMService) WxAccounts(code string) (*WxAccountsResult, error) {
	reqURL := fmt.Sprintf("%s/api/v1/auth/wx-accounts?code=%s&client_id=%s",
		s.baseURL, url.QueryEscape(code), url.QueryEscape(s.clientID))
	resp, err := s.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM wx-accounts endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[IAM DEBUG] WxAccounts non-200 status=%d body=%s", resp.StatusCode, string(body))
		return nil, newIAMAPIError(resp.StatusCode, body)
	}

	var result WxAccountsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse wx-accounts response: %w", err)
	}
	return &result, nil
}

// WxLoginSelect logs in a specific user bound to the openid (multi-account
// selection flow). Passes user_id to IAM wx-login which validates the binding.
// exchangeToken (from wx-accounts) avoids re-exchanging the single-use code.
func (s *IAMService) WxLoginSelect(exchangeToken, userID string) (*TokenResponse, error) {
	payload := map[string]string{
		"exchange_token": exchangeToken,
		"client_id":      s.clientID,
		"user_id":        userID,
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/wx-login", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM wx-login endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[IAM DEBUG] WxLoginSelect non-200 status=%d body=%s", resp.StatusCode, string(body))
		return nil, newIAMAPIError(resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse wx-login response: %w", err)
	}
	return &tokenResp, nil
}

// WxBindResult is the response from IAM POST /api/v1/auth/wx-bind.
type WxBindResult struct {
	UserID   string `json:"user_id"`
	WxOpenid string `json:"wx_openid"`
}

// WxBind exchanges a WeChat login code and binds the openid to the given
// IAM user. Used during the registration flow: the user is created first,
// then the WeChat identity is bound (beaconiam #480).
func (s *IAMService) WxBind(code, userID string) (*WxBindResult, error) {
	payload := map[string]string{
		"code":      code,
		"client_id": s.clientID,
		"user_id":   userID,
	}

	jsonPayload, _ := json.Marshal(payload)
	bindURL := fmt.Sprintf("%s/api/v1/auth/wx-bind", s.baseURL)
	resp, err := s.httpClient.Post(
		bindURL,
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM wx-bind endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[IAM DEBUG] WxBind non-200 status=%d body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("IAM wx-bind returned status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result WxBindResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse wx-bind response: %w", err)
	}
	return &result, nil
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

// GetSMTPHost returns the SMTP host configuration (stub implementation)
func GetSMTPHost() string {
	// TODO: Implement actual SMTP configuration retrieval
	// For now, return empty string to indicate not configured
	return ""
}

func (s *IAMService) WxPhone(encryptedData, iv, userID string) (map[string]interface{}, error) {
	payload := map[string]string{
		"encrypted_data": encryptedData,
		"iv":             iv,
	}
	if userID != "" {
		payload["user_id"] = userID
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/wx-phone", s.baseURL),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call IAM wx-phone endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IAM wx-phone returned status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse wx-phone response: %w", err)
	}
	return result, nil
}

// GetSMSGateway returns the SMS gateway configuration (stub implementation)
func GetSMSGateway() string {
	// TODO: Implement actual SMS gateway configuration retrieval
	// For now, return empty string to indicate not configured
	return ""
}
