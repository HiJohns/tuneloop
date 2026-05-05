package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

func (c *clientTokenCache) reset() {
	c.mu.Lock()
	c.accessToken = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

func NewIAMClient() *IAMClient {
	// Backend API call identity: IAM_NAMESPACE + IAM_SECRET
	clientID := os.Getenv("IAM_NAMESPACE")
	clientSecret := os.Getenv("IAM_SECRET")

	// Fallback to legacy env vars for backward compatibility
	if clientID == "" {
		clientID = os.Getenv("IAM_PC_CLIENT_ID")
		if clientID == "" {
			clientID = os.Getenv("IAM_CLIENT_ID")
		}
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("IAM_PC_CLIENT_SECRET")
		if clientSecret == "" {
			clientSecret = os.Getenv("IAM_CLIENT_SECRET")
		}
	}

	namespace := clientID
	if namespace == "" {
		namespace = "tuneloop"
	}

	baseURL := GetIAMInternalURL()
	log.Printf("[IAMClient] NewIAMClient: baseURL=%s, clientID=%s, namespace=%s", baseURL, clientID, namespace)

	return &IAMClient{
		baseURL:      baseURL,
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
	body, status, err := c.doRequestWithToken(method, path, token, payload)
	if status == http.StatusUnauthorized {
		log.Printf("[IAMClient] API call returned 401, clearing token cache and retrying once")
		c.tokenCache.reset()
		token, err = c.GetClientToken()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to re-acquire client token after 401: %w", err)
		}
		return c.doRequestWithToken(method, path, token, payload)
	}
	return body, status, err
}

func (c *IAMClient) doRequestWithToken(method, path, token string, payload interface{}) ([]byte, int, error) {
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
	NamespaceID  string             `json:"namespace_id,omitempty"`
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

func (c *IAMClient) CreateOrganizationWithToken(token string, req *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations", c.namespace)
	respBody, statusCode, err := c.doRequestWithToken("POST", path, token, req)
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

	log.Printf("[IAMClient] Created organization with user token: org_id=%s, admin_id=%s", result.Data.OrgID, result.Data.AdminID)
	return &result.Data, nil
}

type Organization struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ParentID    *string `json:"parent_id"`
	NamespaceID string  `json:"namespace_id"`
	Status      string  `json:"status"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// User represents an IAM user
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Status   string `json:"status"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
}

// getNamespaceID resolves namespace ID from name/client_id
func (c *IAMClient) getNamespaceID() (string, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s", c.namespace)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("getNamespaceID request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("getNamespaceID returned status %d: %s", statusCode, string(respBody))
	}
	var ns struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &ns); err != nil {
		return "", fmt.Errorf("failed to parse namespace response: %w", err)
	}
	return ns.ID, nil
}

// ListOrganizations gets all organizations under the configured namespace
func (c *IAMClient) ListOrganizations() ([]Organization, error) {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return nil, fmt.Errorf("ListOrganizations: failed to resolve namespace: %w", err)
	}

	path := fmt.Sprintf("/api/v1/organizations?namespace_id=%s&page_size=1000", nsID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("ListOrganizations request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("ListOrganizations returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Organizations []Organization `json:"organizations"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse ListOrganizations response: %w", err)
	}

	return result.Organizations, nil
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

// ListUsers gets all users from IAM
func (c *IAMClient) ListUsers() ([]User, error) {
	log.Printf("[IAMClient] ListUsers: baseURL=%s, namespace=%s, clientID=%s", c.baseURL, c.namespace, c.clientID)

	respBody, statusCode, err := c.doRequest("GET", "/api/v1/users", nil)
	if err != nil {
		log.Printf("[IAMClient] ListUsers: doRequest error: %v", err)
		return nil, fmt.Errorf("ListUsers request failed: %w", err)
	}

	log.Printf("[IAMClient] ListUsers: statusCode=%d, responseLen=%d", statusCode, len(respBody))

	if statusCode != http.StatusOK {
		log.Printf("[IAMClient] ListUsers: non-200 response: %s", string(respBody))
		return nil, fmt.Errorf("ListUsers returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Users []User `json:"users"`
		Data  []User `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[IAMClient] ListUsers: unmarshal error: %v, response: %s", err, string(respBody))
		return nil, fmt.Errorf("failed to parse ListUsers response: %w", err)
	}

	log.Printf("[IAMClient] ListUsers: parsed result - users count=%d, data count=%d", len(result.Users), len(result.Data))

	if len(result.Users) > 0 {
		log.Printf("[IAMClient] ListUsers: returning %d users from 'users' field", len(result.Users))
		return result.Users, nil
	}
	log.Printf("[IAMClient] ListUsers: returning %d users from 'data' field", len(result.Data))
	return result.Data, nil
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

func (c *IAMClient) CreateUserWithToken(token string, req *CreateUserRequest) (*CreateUserResponse, error) {
	respBody, statusCode, err := c.doRequestWithToken("POST", "/api/v1/users", token, req)
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

	log.Printf("[IAMClient] Created user with user token: user_id=%s, status=%s", result.Data.UserID, result.Data.Status)
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

func (c *IAMClient) BindUserToOrganizationWithToken(token, userID, orgID, role, operatorID string) error {
	path := fmt.Sprintf("/api/v1/users/%s/organizations/%s/bind", userID, orgID)
	req := &BindUserRequest{
		Action:     "bind",
		Role:       role,
		OperatorID: operatorID,
	}
	respBody, statusCode, err := c.doRequestWithToken("PUT", path, token, req)
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

	log.Printf("[IAMClient] Bound user %s to org %s with role %s (user token)", userID, orgID, role)
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

func (c *IAMClient) DeleteUser(iamUserID string) error {
	path := fmt.Sprintf("/api/v1/users/%s", iamUserID)
	respBody, statusCode, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("DeleteUser request failed: %w", err)
	}

	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user not found: %s", iamUserID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("DeleteUser returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Deleted user: user_id=%s", iamUserID)
	return nil
}

type ResetPasswordResult struct {
	Sent    int `json:"sent"`
	Skipped int `json:"skipped"`
}

func (c *IAMClient) ResetPasswordWithToken(userToken string, userIDs []string, redirectURL string) (*ResetPasswordResult, error) {
	req := map[string]interface{}{
		"user_ids":      userIDs,
		"redirect_url":  redirectURL,
	}
	respBody, statusCode, err := c.doRequestWithToken("POST", "/api/v1/users/reset-password", userToken, req)
	if err != nil {
		return nil, fmt.Errorf("ResetPassword request failed: %w", err)
	}

	if statusCode == http.StatusForbidden {
		return nil, fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("ResetPassword returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Data ResetPasswordResult `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		var direct ResetPasswordResult
		if err2 := json.Unmarshal(respBody, &direct); err2 == nil {
			return &direct, nil
		}
		return nil, fmt.Errorf("failed to parse ResetPassword response: %w", err)
	}

	return &result.Data, nil
}

func ExtractUserToken(c *gin.Context) string {
	if token, err := c.Cookie("token"); err == nil && token != "" {
		return token
	}
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

// PermissionDef represents a customer permission to register with IAM.
type PermissionDef struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PermissionMapping represents a registered customer permission with bit code.
type PermissionMapping struct {
	Code    string `json:"code"`
	BitCode int    `json:"bit_code"`
	Name    string `json:"name"`
	IsActive bool  `json:"is_active"`
}

type registerCustomerPermissionsReq struct {
	Permissions []PermissionDef `json:"permissions"`
}

type listCustomerPermissionsResp struct {
	NamespaceID string              `json:"namespace_id"`
	Permissions []PermissionMapping `json:"permissions"`
}

type setPermissionCodesReq struct {
	PermissionCodes []string `json:"permission_codes"`
}

// RegisterCustomerPermissions registers custom permission definitions with IAM.
// Idempotent: submitting the same code multiple times returns the existing bit code.
func (c *IAMClient) RegisterCustomerPermissions(namespaceID string, perms []PermissionDef) ([]PermissionMapping, error) {
	req := registerCustomerPermissionsReq{Permissions: perms}
	path := fmt.Sprintf("/api/v1/namespaces/%s/customer-permissions", namespaceID)

	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return nil, fmt.Errorf("RegisterCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("RegisterCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}

	var resp listCustomerPermissionsResp
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse RegisterCustomerPermissions response: %w", err)
	}
	return resp.Permissions, nil
}

// GetCustomerPermissions fetches all registered customer permissions from IAM.
func (c *IAMClient) GetCustomerPermissions(namespaceID string) ([]PermissionMapping, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/customer-permissions", namespaceID)

	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("GetCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("GetCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}

	var resp listCustomerPermissionsResp
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetCustomerPermissions response: %w", err)
	}
	return resp.Permissions, nil
}

// SetRoleCustomerPermissions sets the cus_perm bit codes for a role template.
func (c *IAMClient) SetRoleCustomerPermissions(namespaceID, roleID string, permCodes []string) error {
	req := setPermissionCodesReq{PermissionCodes: permCodes}
	path := fmt.Sprintf("/api/v1/namespaces/%s/role-templates/%s/customer-permissions", namespaceID, roleID)

	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SetRoleCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SetRoleCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}
	return nil
}

// SetUserCustomerPermissions sets the direct cus_perm bit codes for a user in an organization.
func (c *IAMClient) SetUserCustomerPermissions(orgID, userID string, permCodes []string) error {
	req := setPermissionCodesReq{PermissionCodes: permCodes}
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/customer-permissions", orgID, userID)

	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SetUserCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SetUserCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}
	return nil
}

// GetNamespace returns the configured namespace name.
func (c *IAMClient) GetNamespace() string {
	return c.namespace
}
