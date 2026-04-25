package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type IAMClient struct {
	baseURL      string
	clientID     string
	clientSecret string
	namespace    string
	httpClient   *http.Client
	tokenCache   *clientTokenCache
}

type clientTokenCache struct {
	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
}

func NewIAMClient() *IAMClient {
	clientID := os.Getenv("IAM_PC_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("IAM_CLIENT_ID")
	}
	clientSecret := os.Getenv("IAM_PC_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("IAM_CLIENT_SECRET")
	}
	namespace := os.Getenv("IAM_NAMESPACE")
	if namespace == "" {
		namespace = "tuneloop"
	}
	return &IAMClient{
		baseURL:      GetIAMInternalURL(),
		clientID:     clientID,
		clientSecret: clientSecret,
		namespace:    namespace,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		tokenCache:   &clientTokenCache{},
	}
}

func (c *IAMClient) GetClientToken() (string, error) {
	c.tokenCache.mu.RLock()
	if c.tokenCache.accessToken != "" && time.Now().Before(c.tokenCache.expiresAt.Add(-30*time.Second)) {
		token := c.tokenCache.accessToken
		c.tokenCache.mu.RUnlock()
		return token, nil
	}
	c.tokenCache.mu.RUnlock()

	c.tokenCache.mu.Lock()
	defer c.tokenCache.mu.Unlock()

	if c.tokenCache.accessToken != "" && time.Now().Before(c.tokenCache.expiresAt.Add(-30*time.Second)) {
		return c.tokenCache.accessToken, nil
	}

	reqBody := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/v1/auth/token", c.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("failed to request client token: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	c.tokenCache.accessToken = tokenResp.AccessToken
	c.tokenCache.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)

	log.Printf("[IAMClient] Obtained client credentials token, expires in %ds", expiresIn)
	return tokenResp.AccessToken, nil
}

func (c *IAMClient) doRequest(method, path string, payload interface{}) ([]byte, int, error) {
	token, err := c.GetClientToken()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get client token: %w", err)
	}

	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s%s", c.baseURL, path), bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

type CreateOrganizationRequest struct {
	Name         string             `json:"name"`
	ParentID     string             `json:"parent_id,omitempty"`
	Address      string             `json:"address,omitempty"`
	ContactPhone string             `json:"contact_phone,omitempty"`
	AdminInfo    *OrganizationAdmin `json:"admin_info,omitempty"`
	CallbackURL  string             `json:"callback_url,omitempty"`
	OperatorID   string             `json:"operator_id,omitempty"`
}

type OrganizationAdmin struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type CreateOrganizationResponse struct {
	OrgID   string `json:"org_id"`
	AdminID string `json:"admin_id"`
}

func (c *IAMClient) CreateOrganization(req *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations", c.namespace)
	respBody, statusCode, err := c.doRequest("POST", path, req)
	if err != nil {
		return nil, fmt.Errorf("CreateOrganization request failed: %w", err)
	}

	if statusCode == http.StatusConflict {
		return nil, fmt.Errorf("organization name conflict: %s", string(respBody))
	}
	if statusCode == http.StatusForbidden {
		return nil, fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("CreateOrganization returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Data CreateOrganizationResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		var direct CreateOrganizationResponse
		if err2 := json.Unmarshal(respBody, &direct); err2 == nil {
			return &direct, nil
		}
		return nil, fmt.Errorf("failed to parse CreateOrganization response: %w", err)
	}

	log.Printf("[IAMClient] Created organization: org_id=%s, admin_id=%s", result.Data.OrgID, result.Data.AdminID)
	return &result.Data, nil
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	CallbackURL string `json:"callback_url,omitempty"`
	OperatorID  string `json:"operator_id,omitempty"`
}

type CreateUserResponse struct {
	UserID string `json:"user_id"`
	Status string `json:"status"`
}

func (c *IAMClient) CreateUser(req *CreateUserRequest) (*CreateUserResponse, error) {
	respBody, statusCode, err := c.doRequest("POST", "/api/v1/users", req)
	if err != nil {
		return nil, fmt.Errorf("CreateUser request failed: %w", err)
	}

	if statusCode == http.StatusConflict {
		return nil, fmt.Errorf("user already exists: %s", string(respBody))
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("CreateUser returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Data CreateUserResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		var direct CreateUserResponse
		if err2 := json.Unmarshal(respBody, &direct); err2 == nil {
			return &direct, nil
		}
		return nil, fmt.Errorf("failed to parse CreateUser response: %w", err)
	}

	log.Printf("[IAMClient] Created user: user_id=%s, status=%s", result.Data.UserID, result.Data.Status)
	return &result.Data, nil
}

type UpdateUserRequest struct {
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Password    string `json:"password,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
	OperatorID  string `json:"operator_id,omitempty"`
}

func (c *IAMClient) UpdateUser(userID string, req *UpdateUserRequest) error {
	path := fmt.Sprintf("/api/v1/users/%s", userID)
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("UpdateUser request failed: %w", err)
	}

	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user not found: %s", userID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UpdateUser returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Updated user: user_id=%s", userID)
	return nil
}

type BindUserRequest struct {
	Action     string `json:"action"`
	Role       string `json:"role,omitempty"`
	OperatorID string `json:"operator_id,omitempty"`
}

func (c *IAMClient) BindUserToOrganization(userID, orgID, role, operatorID string) error {
	path := fmt.Sprintf("/api/v1/users/%s/organizations/%s/bind", userID, orgID)
	req := &BindUserRequest{
		Action:     "bind",
		Role:       role,
		OperatorID: operatorID,
	}
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("BindUser request failed: %w", err)
	}

	if statusCode == http.StatusBadRequest {
		return fmt.Errorf("bad request (sole admin cannot be demoted): %s", string(respBody))
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user or organization not found: user=%s org=%s", userID, orgID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("BindUser returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Bound user %s to org %s with role %s", userID, orgID, role)
	return nil
}

func (c *IAMClient) UnbindUserFromOrganization(userID, orgID, operatorID string) error {
	path := fmt.Sprintf("/api/v1/users/%s/organizations/%s/bind", userID, orgID)
	req := &BindUserRequest{
		Action:     "unbind",
		OperatorID: operatorID,
	}
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("UnbindUser request failed: %w", err)
	}

	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user or organization not found: user=%s org=%s", userID, orgID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UnbindUser returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Unbound user %s from org %s", userID, orgID)
	return nil
}
