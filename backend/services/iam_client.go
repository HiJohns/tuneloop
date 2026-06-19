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

// NewIAMClientWithCredentials creates an IAM client using specific OAuth credentials.
// Used for activating tenant-specific identities like the app's UUID client_id.
func NewIAMClientWithCredentials(clientID, clientSecret string) *IAMClient {
	return &IAMClient{
		baseURL:      GetIAMInternalURL(),
		clientID:     clientID,
		clientSecret: clientSecret,
		namespace:    os.Getenv("IAM_NAMESPACE"),
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

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("GetClientToken: IAM response missing access_token")
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
	Name             string             `json:"name"`
	ParentID         string             `json:"parent_id,omitempty"`
	NamespaceID      string             `json:"namespace_id,omitempty"`
	Address          string             `json:"address,omitempty"`
	ContactPhone     string             `json:"contact_phone,omitempty"`
	AdminInfo        *OrganizationAdmin `json:"admin_info,omitempty"`
	CallbackURL      string             `json:"callback_url,omitempty"`
	OperatorID       string             `json:"operator_id,omitempty"`
	SkipActivation   bool               `json:"skip_activation,omitempty"`
	NotificationLang string             `json:"notification_lang,omitempty"`
}

type OrganizationAdmin struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type CreateOrganizationResponse struct {
	OrgID           string `json:"org_id"`
	AdminID         string `json:"admin_id"`
	InitialPassword string `json:"initial_password,omitempty"`
}

func (c *IAMClient) CreateOrganization(req *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return nil, fmt.Errorf("CreateOrganization: failed to resolve namespace: %w", err)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations", nsID)
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

	var parsed CreateOrganizationResponse
	// Try flat format first (IAM returns {org_id, admin_id, ...})
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.OrgID == "" {
		// Fallback to wrapped format {data: {org_id, admin_id, ...}}
		var result struct {
			Data CreateOrganizationResponse `json:"data"`
		}
		if err2 := json.Unmarshal(respBody, &result); err2 == nil {
			parsed = result.Data
		}
	}
	if parsed.OrgID == "" {
		return nil, fmt.Errorf("CreateOrganization response missing org_id")
	}
	log.Printf("[IAMClient] Created organization: org_id=%s, admin_id=%s", parsed.OrgID, parsed.AdminID)
	return &parsed, nil
}

func (c *IAMClient) CreateOrganizationWithToken(token string, req *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return nil, fmt.Errorf("CreateOrganizationWithToken: failed to resolve namespace: %w", err)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations", nsID)
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

	var parsed CreateOrganizationResponse
	// Try flat format first (IAM returns {org_id, admin_id, ...})
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.OrgID == "" {
		// Fallback to wrapped format {data: {org_id, admin_id, ...}}
		var result struct {
			Data CreateOrganizationResponse `json:"data"`
		}
		if err2 := json.Unmarshal(respBody, &result); err2 == nil {
			parsed = result.Data
		}
	}
	if parsed.OrgID == "" {
		return nil, fmt.Errorf("CreateOrganization response missing org_id")
	}
	log.Printf("[IAMClient] Created organization with user token: org_id=%s, admin_id=%s", parsed.OrgID, parsed.AdminID)
	return &parsed, nil
}

func (c *IAMClient) DeleteOrganization(orgID string) error {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return fmt.Errorf("DeleteOrganization: failed to resolve namespace: %w", err)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations/%s", nsID, orgID)
	_, statusCode, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("DeleteOrganization request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusNoContent {
		return fmt.Errorf("DeleteOrganization returned status %d", statusCode)
	}
	log.Printf("[IAMClient] Deleted organization: org_id=%s", orgID)
	return nil
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
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	Phone           string     `json:"phone"`
	Status          string     `json:"status"`
	OrgID           string     `json:"org_id"`
	Role            string     `json:"role"`
	EmailSentAt     *time.Time `json:"email_sent_at,omitempty"`
	EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
}

func (c *IAMClient) GetNamespaceID() (string, error) {
	return c.getNamespaceID()
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
	if ns.ID == "" {
		return "", fmt.Errorf("getNamespaceID: IAM response missing namespace id")
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

func (c *IAMClient) ListOrganizationsWithToken(token string) ([]Organization, error) {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return nil, fmt.Errorf("ListOrganizationsWithToken: failed to resolve namespace: %w", err)
	}

	path := fmt.Sprintf("/api/v1/organizations?namespace_id=%s&page_size=1000", nsID)
	respBody, statusCode, err := c.doRequestWithToken("GET", path, token, nil)
	if err != nil {
		return nil, fmt.Errorf("ListOrganizationsWithToken request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("ListOrganizationsWithToken returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Organizations []Organization `json:"organizations"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse ListOrganizationsWithToken response: %w", err)
	}

	return result.Organizations, nil
}

// GetOrganization fetches a single organization by ID from IAM.
func (c *IAMClient) GetOrganization(orgID string) (*Organization, error) {
	path := fmt.Sprintf("/api/v1/organizations/%s", orgID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("GetOrganization request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("GetOrganization returned status %d: %s", statusCode, string(respBody))
	}
	var org Organization
	if err := json.Unmarshal(respBody, &org); err != nil {
		return nil, fmt.Errorf("failed to parse GetOrganization response: %w", err)
	}
	if org.ID == "" {
		return nil, fmt.Errorf("GetOrganization: IAM response missing org id")
	}
	return &org, nil
}

type CreateUserRequest struct {
	Username              string `json:"username"`
	Name                  string `json:"name"`
	Email                 string `json:"email"`
	Phone                 string `json:"phone"`
	Password              string `json:"password,omitempty"`
	SkipActivation        bool   `json:"skip_activation"`
	SendNotificationEmail bool   `json:"send_notification_email,omitempty"`
	NotificationLang      string `json:"notification_lang,omitempty"`
	ForcePasswordChange   bool   `json:"force_password_change,omitempty"`
	CallbackURL           string `json:"callback_url,omitempty"`
	Reason                string `json:"reason,omitempty"`
	OperatorID            string `json:"operator_id,omitempty"`
}

type CreateUserResponse struct {
	UserID          string `json:"user_id"`
	Status          string `json:"status"`
	InitialPassword string `json:"initial_password,omitempty"`
}

// ExistingUserInfo holds data returned when creating a user that already exists.
type ExistingUserInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	MatchedFields []string `json:"matched_fields"`
}

// CreateUserResult holds the outcome of a create-or-get user attempt.
type CreateUserResult struct {
	UserID        string             `json:"user_id,omitempty"`
	Status        string             `json:"status,omitempty"`
	Conflict      bool               `json:"conflict"`
	ExistingUsers []ExistingUserInfo `json:"existing_users,omitempty"`
}

// CreateOrGetUser creates a user or returns existing user info on conflict.
func (c *IAMClient) CreateOrGetUser(token string, req *CreateUserRequest) (*CreateUserResult, error) {
	// Check for conflicts before creating
	allUsers, err := c.ListUsers()
	if err != nil {
		log.Printf("[IAMClient] CreateOrGetUser: ListUsers failed (proceeding with create): %v", err)
	} else if len(allUsers) > 0 {
		var conflicts []ExistingUserInfo
		emailToUser := make(map[string]*User)
		phoneToUser := make(map[string]*User)
		for i := range allUsers {
			u := &allUsers[i]
			if u.Email != "" {
				emailToUser[u.Email] = u
			}
			if u.Phone != "" {
				phoneToUser[u.Phone] = u
			}
		}

		matchedFields := make(map[string][]string)
		addConflict := func(u *User, field string) {
			if _, exists := matchedFields[u.ID]; !exists {
				matchedFields[u.ID] = []string{field}
				conflicts = append(conflicts, ExistingUserInfo{
					ID:    u.ID,
					Name:  u.Name,
					Email: u.Email,
					Phone: u.Phone,
				})
			} else {
				matchedFields[u.ID] = append(matchedFields[u.ID], field)
			}
		}

		if req.Email != "" {
			if u, ok := emailToUser[req.Email]; ok {
				addConflict(u, "email")
			}
		}
		if req.Phone != "" {
			if u, ok := phoneToUser[req.Phone]; ok {
				addConflict(u, "phone")
			}
		}
		if req.Username != "" {
			for i := range allUsers {
				u := &allUsers[i]
				if u.Username == req.Username {
					addConflict(u, "username")
					break
				}
			}
		}

		for i := range conflicts {
			conflicts[i].MatchedFields = matchedFields[conflicts[i].ID]
		}

		if len(conflicts) > 0 {
			log.Printf("[IAMClient] CreateOrGetUser: conflicts found, returning existing user %s", conflicts[0].ID)
			return &CreateUserResult{
				UserID:        conflicts[0].ID,
				Conflict:      true,
				ExistingUsers: conflicts,
			}, nil
		}
	}

	respBody, statusCode, err := c.doRequestWithToken("POST", "/api/v1/users", token, req)
	if err != nil {
		return nil, fmt.Errorf("CreateUser request failed: %w", err)
	}

	if statusCode == http.StatusConflict {
		var existing ExistingUserInfo
		if err := json.Unmarshal(respBody, &existing); err == nil && existing.ID != "" {
			return &CreateUserResult{
				UserID:        existing.ID,
				Conflict:      true,
				ExistingUsers: []ExistingUserInfo{existing},
			}, nil
		}
		return nil, fmt.Errorf("user already exists: %s", string(respBody))
	}

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("CreateUser returned status %d: %s", statusCode, string(respBody))
	}

	var result CreateUserResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		var wrapped struct {
			Data CreateUserResult `json:"data"`
		}
		if err2 := json.Unmarshal(respBody, &wrapped); err2 == nil {
			result = wrapped.Data
		} else {
			return nil, fmt.Errorf("failed to parse CreateUser response: %w", err)
		}
	}

	log.Printf("[IAMClient] Created user via CreateOrGetUser: user_id=%s", result.UserID)
	if result.UserID == "" {
		return nil, fmt.Errorf("CreateOrGetUser: IAM response missing user_id")
	}
	return &result, nil
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

func (c *IAMClient) ListUsersWithToken(token string) ([]User, error) {
	respBody, statusCode, err := c.doRequestWithToken("GET", "/api/v1/users", token, nil)
	if err != nil {
		log.Printf("[IAMClient] ListUsersWithToken: doRequest error: %v", err)
		return nil, fmt.Errorf("ListUsersWithToken request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		log.Printf("[IAMClient] ListUsersWithToken: non-200 response: %s", string(respBody))
		return nil, fmt.Errorf("ListUsersWithToken returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		Users []User `json:"users"`
		Data  []User `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[IAMClient] ListUsersWithToken: unmarshal error: %v, response: %s", err, string(respBody))
		return nil, fmt.Errorf("failed to parse ListUsersWithToken response: %w", err)
	}

	if len(result.Users) > 0 {
		return result.Users, nil
	}
	return result.Data, nil
}

// GetUser retrieves a single user from IAM by ID using service auth
func (c *IAMClient) GetUser(userID string) (*User, error) {
	path := fmt.Sprintf("/api/v1/users/%s", userID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("GetUser request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("GetUser returned status %d: %s", statusCode, string(respBody))
	}
	var result struct {
		User *User `json:"user"`
		Data *User `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse GetUser response: %w", err)
	}
	if result.User != nil && result.User.ID != "" {
		return result.User, nil
	}
	if result.Data != nil && result.Data.ID != "" {
		return result.Data, nil
	}
	return nil, fmt.Errorf("GetUser: user %s not found in response", userID)
}

// GetUserEmailStatus fetches email confirmation timestamps from IAM
// Handles flat user response format: {"id": "...", "email": "...", ...}
func (c *IAMClient) GetUserEmailStatus(userID string) (*time.Time, *time.Time, error) {
	path := fmt.Sprintf("/api/v1/users/%s", userID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("GetUserEmailStatus request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GetUserEmailStatus returned status %d: %s", statusCode, string(respBody))
	}

	// Try flat user response first
	var flatUser struct {
		EmailSentAt     *time.Time `json:"email_sent_at,omitempty"`
		EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
	}
	if err := json.Unmarshal(respBody, &flatUser); err == nil {
		return flatUser.EmailSentAt, flatUser.EmailConfirmedAt, nil
	}

	// Fallback: try nested formats
	var nested struct {
		User *struct {
			EmailSentAt     *time.Time `json:"email_sent_at,omitempty"`
			EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
		} `json:"user"`
		Data *struct {
			EmailSentAt     *time.Time `json:"email_sent_at,omitempty"`
			EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &nested); err == nil {
		if nested.User != nil {
			return nested.User.EmailSentAt, nested.User.EmailConfirmedAt, nil
		}
		if nested.Data != nil {
			return nested.Data.EmailSentAt, nested.Data.EmailConfirmedAt, nil
		}
	}

	return nil, nil, fmt.Errorf("GetUserEmailStatus: email fields not found in response")
}

// ResendEmailConfirmation requests IAM to resend the email confirmation email
func (c *IAMClient) ResendEmailConfirmation(userID string) error {
	req := map[string]interface{}{
		"user_ids": []string{userID},
	}
	respBody, statusCode, err := c.doRequest("POST", "/api/v1/users/resend-confirmation", req)
	if err != nil {
		return fmt.Errorf("ResendEmailConfirmation request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("ResendEmailConfirmation returned status %d: %s", statusCode, string(respBody))
	}
	return nil
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
	if err := json.Unmarshal(respBody, &result); err != nil || result.Data.UserID == "" {
		var direct CreateUserResponse
		if err2 := json.Unmarshal(respBody, &direct); err2 == nil {
			if direct.UserID != "" {
				return &direct, nil
			}
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
	if err := json.Unmarshal(respBody, &result); err != nil || result.Data.UserID == "" {
		var direct CreateUserResponse
		if err2 := json.Unmarshal(respBody, &direct); err2 == nil {
			if direct.UserID != "" {
				return &direct, nil
			}
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

func (c *IAMClient) UpdateUserPassword(userID, newPassword string) error {
	return c.UpdateUser(userID, &UpdateUserRequest{
		Password: newPassword,
	})
}

type BindUserRequest struct {
	Action     string `json:"action"`
	Role       string `json:"role,omitempty"`
	OperatorID string `json:"operator_id,omitempty"`
}

func (c *IAMClient) BindUserToOrganization(userID, orgID, role, operatorID string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID)
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

	// Increment perm_version to notify clients of permission change
	c.IncrementPermVersion()

	return nil
}

// CheckMembership calls BindUser and returns whether the user is already bound to the org.
// Returns (isBound, error). When isBound=true, the user is already an active member.
// When false, a new bind task was queued (user not yet in org).
func (c *IAMClient) CheckMembership(userID, orgID string) (bool, error) {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID)
	req := &BindUserRequest{
		Action: "bind",
		Role:   "OWNER",
	}
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return false, fmt.Errorf("CheckMembership request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		return false, fmt.Errorf("CheckMembership returned status %d: %s", statusCode, string(respBody))
	}

	if strings.Contains(string(respBody), "bound") {
		log.Printf("[IAMClient] User %s is already bound to org %s", userID, orgID)
		return true, nil
	}

	log.Printf("[IAMClient] User %s not yet bound to org %s (task queued)", userID, orgID)
	return false, nil
}

func (c *IAMClient) BindUserToOrganizationWithToken(token, userID, orgID, role, operatorID string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID)
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

	// Increment perm_version to notify clients of permission change
	c.IncrementPermVersion()

	return nil
}

func (c *IAMClient) UnbindUserFromOrganization(userID, orgID, operatorID string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID)
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

	// Increment perm_version to notify clients of permission change
	c.IncrementPermVersion()

	return nil
}

func (c *IAMClient) UpdateUserRoleInOrg(orgID, userID, role string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/role?role=%s", orgID, userID, role)
	respBody, statusCode, err := c.doRequest("PUT", path, nil)
	if err != nil {
		return fmt.Errorf("UpdateUserRole request failed: %w", err)
	}

	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user or organization not found: user=%s org=%s", userID, orgID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UpdateUserRole returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Updated user %s role to %s in org %s", userID, role, orgID)

	// Increment perm_version to notify clients of permission change
	c.IncrementPermVersion()

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

func (c *IAMClient) ResetPasswordWithToken(userToken string, userIDs []string, redirectURL string, culture string) (*ResetPasswordResult, error) {
	req := map[string]interface{}{
		"user_ids":      userIDs,
		"redirect_url":  redirectURL,
		"culture":       culture,
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

func (c *IAMClient) RequestPasswordReset(userID string) error {
	req := map[string]interface{}{
		"user_ids": []string{userID},
	}
	respBody, statusCode, err := c.doRequest("POST", "/api/v1/users/reset-password", req)
	if err != nil {
		return fmt.Errorf("RequestPasswordReset failed: %w", err)
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("RequestPasswordReset returned status %d: %s", statusCode, string(respBody))
	}
	return nil
}

// --- WithToken variants (for user identity scoping) ---

func (c *IAMClient) DeleteOrganizationWithToken(token, orgID string) error {
	nsID, err := c.getNamespaceID()
	if err != nil {
		return fmt.Errorf("DeleteOrganizationWithToken: failed to resolve namespace: %w", err)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/organizations/%s", nsID, orgID)
	_, statusCode, err := c.doRequestWithToken("DELETE", path, token, nil)
	if err != nil {
		return fmt.Errorf("DeleteOrganizationWithToken request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusNoContent {
		return fmt.Errorf("DeleteOrganizationWithToken returned status %d", statusCode)
	}
	log.Printf("[IAMClient] Deleted organization with user token: org_id=%s", orgID)
	return nil
}

func (c *IAMClient) UpdateUserWithToken(token, userID string, req *UpdateUserRequest) error {
	path := fmt.Sprintf("/api/v1/users/%s", userID)
	respBody, statusCode, err := c.doRequestWithToken("PUT", path, token, req)
	if err != nil {
		return fmt.Errorf("UpdateUserWithToken request failed: %w", err)
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user not found: %s", userID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UpdateUserWithToken returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Updated user with user token: user_id=%s", userID)
	return nil
}

func (c *IAMClient) UnbindUserFromOrganizationWithToken(token, userID, orgID, operatorID string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID)
	req := &BindUserRequest{
		Action:     "unbind",
		OperatorID: operatorID,
	}
	respBody, statusCode, err := c.doRequestWithToken("PUT", path, token, req)
	if err != nil {
		return fmt.Errorf("UnbindUserWithToken request failed: %w", err)
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user or organization not found: user=%s org=%s", userID, orgID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UnbindUserWithToken returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Unbound user with user token: user=%s org=%s", userID, orgID)
	c.IncrementPermVersion()
	return nil
}

func (c *IAMClient) UpdateUserRoleInOrgWithToken(token, orgID, userID, role string) error {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/role?role=%s", orgID, userID, role)
	respBody, statusCode, err := c.doRequestWithToken("PUT", path, token, nil)
	if err != nil {
		return fmt.Errorf("UpdateUserRoleWithToken request failed: %w", err)
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user or organization not found: user=%s org=%s", userID, orgID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("UpdateUserRoleWithToken returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Updated user role with user token: user=%s role=%s org=%s", userID, role, orgID)
	c.IncrementPermVersion()
	return nil
}

func (c *IAMClient) DeleteUserWithToken(token, iamUserID string) error {
	path := fmt.Sprintf("/api/v1/users/%s", iamUserID)
	respBody, statusCode, err := c.doRequestWithToken("DELETE", path, token, nil)
	if err != nil {
		return fmt.Errorf("DeleteUserWithToken request failed: %w", err)
	}
	if statusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied: %s", string(respBody))
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("user not found: %s", iamUserID)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("DeleteUserWithToken returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Deleted user with user token: user_id=%s", iamUserID)
	return nil
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

// PermissionDef represents a customer permission.
type PermissionDef struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	BitCode     int    `json:"-"`
}

// PermissionMapping represents a registered customer permission with bit code.
type PermissionMapping struct {
	Code     string `json:"code"`
	BitCode  int    `json:"bit_code"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

type setPermissionCodesReq struct {
	PermissionCodes []string `json:"permission_codes,omitempty"`
	RawBits         bool     `json:"raw_bits,omitempty"`
	CusPerm         int64    `json:"cus_perm,omitempty"`
	CusPermExt      []byte   `json:"cus_perm_ext,omitempty"`
}

// SetUserCustomerPermissions sets the cus_perm bitmap for a user in an organization.
// Sends raw bit values — no code→bit mapping needed (beaconiam #293 Phase 1).
func (c *IAMClient) SetUserCustomerPermissions(orgID, userID string, cusPerm int64, cusPermExt []byte) error {
	req := setPermissionCodesReq{RawBits: true, CusPerm: cusPerm, CusPermExt: cusPermExt}
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/customer-permissions", orgID, userID)

	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SetUserCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SetUserCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}

	// Increment perm_version to notify clients of permission change
	c.IncrementPermVersion()

	return nil
}

// GetUserCustomerPermissions retrieves a user's customer permissions in an org.
func (c *IAMClient) GetUserCustomerPermissions(orgID, userID string) (map[string]interface{}, int64, error) {
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/customer-permissions", orgID, userID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("GetUserCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("GetUserCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		CusPerm    int64              `json:"cus_perm"`
		CusPermExt string             `json:"cus_perm_ext"`
		OrgID      string             `json:"org_id"`
		UserID     string             `json:"user_id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	resp := map[string]interface{}{
		"cus_perm":  result.CusPerm,
		"org_id":    result.OrgID,
		"user_id":   result.UserID,
	}
	return resp, result.CusPerm, nil
}

// SetUserCustomerPermissionsCodes sets the cus_perm via permission codes (backward compatibility).
// Used before beaconiam #293 Phase 1 raw_bits support is deployed.
func (c *IAMClient) SetUserCustomerPermissionsCodes(orgID, userID string, permCodes []string) error {
	req := setPermissionCodesReq{PermissionCodes: permCodes}
	path := fmt.Sprintf("/api/v1/organizations/%s/users/%s/customer-permissions", orgID, userID)

	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SetUserCustomerPermissions request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SetUserCustomerPermissions returned status %d: %s", statusCode, string(respBody))
	}

	c.IncrementPermVersion()
	return nil
}

// GetNamespace returns the configured namespace name.
func (c *IAMClient) GetNamespace() string {
	return c.namespace
}

// IncrementPermVersion increments the global permission version in IAM.
func (c *IAMClient) IncrementPermVersion() error {
	path := "/api/v1/perm-version/increment"
	respBody, statusCode, err := c.doRequest("POST", path, nil)
	if err != nil {
		return fmt.Errorf("IncrementPermVersion request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("IncrementPermVersion returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Incremented perm_version")
	return nil
}


// SyncRoleTemplateSysPerm syncs the sys_perm for a role template in IAM.
func (c *IAMClient) SyncRoleTemplateSysPerm(namespaceID, roleCode string, sysPermBits []int) error {
	roleTemplates, err := c.ListRoleTemplates(namespaceID)
	if err != nil {
		return fmt.Errorf("SyncRoleTemplateSysPerm: failed to list role templates: %w", err)
	}

	var targetID string
	for _, rt := range roleTemplates {
		if rt.Code == roleCode {
			targetID = rt.ID
			break
		}
	}
	if targetID == "" {
		return fmt.Errorf("SyncRoleTemplateSysPerm: role template not found for code %s", roleCode)
	}

	sysPerm := int64(0)
	for _, b := range sysPermBits {
		if b >= 0 && b < 64 {
			sysPerm |= 1 << b
		}
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/role-templates/%s", namespaceID, targetID)
	req := map[string]interface{}{
		"sys_perm": sysPerm,
	}
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SyncRoleTemplateSysPerm request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SyncRoleTemplateSysPerm returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Synced sys_perm for role %s: bits=%v → %d", roleCode, sysPermBits, sysPerm)
	return nil
}

// ListRoleTemplates returns the role templates for a namespace.
func (c *IAMClient) ListRoleTemplates(namespaceID string) ([]struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/role-templates", namespaceID)
	respBody, statusCode, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("ListRoleTemplates request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("ListRoleTemplates returned status %d: %s", statusCode, string(respBody))
	}

	var result []struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	// Try wrapped format first, then plain array
	var wrapped struct {
		Templates []struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"role_templates"`
	}
	if err := json.Unmarshal(respBody, &wrapped); err == nil && len(wrapped.Templates) > 0 {
		return wrapped.Templates, nil
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse ListRoleTemplates response: %w", err)
	}
	return result, nil
}

// SyncRoleTemplateCusPerm syncs the cus_perm bitmap for a role template in IAM.
func (c *IAMClient) SyncRoleTemplateCusPerm(namespaceID, roleCode string, cusPerm int64, cusPermExt []byte) error {
	roleTemplates, err := c.ListRoleTemplates(namespaceID)
	if err != nil {
		return fmt.Errorf("SyncRoleTemplateCusPerm: failed to list role templates: %w", err)
	}

	var targetID string
	for _, rt := range roleTemplates {
		if rt.Code == roleCode {
			targetID = rt.ID
			break
		}
	}
	if targetID == "" {
		return fmt.Errorf("SyncRoleTemplateCusPerm: role template not found for code %s", roleCode)
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/role-templates/%s", namespaceID, targetID)
	req := map[string]interface{}{
		"cus_perm":     cusPerm,
		"cus_perm_ext": cusPermExt,
	}
	respBody, statusCode, err := c.doRequest("PUT", path, req)
	if err != nil {
		return fmt.Errorf("SyncRoleTemplateCusPerm request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("SyncRoleTemplateCusPerm returned status %d: %s", statusCode, string(respBody))
	}

	log.Printf("[IAMClient] Synced cus_perm for role %s", roleCode)
	return nil
}

// CreateRoleTemplate creates a new custom role template in IAM.
// IAM defaults sys_perm=0 for custom roles created via this endpoint.
// Returns the created template ID.
func (c *IAMClient) CreateRoleTemplate(namespaceID, code, name string, cusPerm int64, cusPermExt []byte) (string, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/role-templates", namespaceID)
	req := map[string]interface{}{
		"code":         code,
		"name":         name,
		"sys_perm":     0,
		"cus_perm":     cusPerm,
		"cus_perm_ext": cusPermExt,
	}
	respBody, statusCode, err := c.doRequest("POST", path, req)
	if err != nil {
		return "", fmt.Errorf("CreateRoleTemplate request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return "", fmt.Errorf("CreateRoleTemplate returned status %d: %s", statusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("CreateRoleTemplate: failed to parse response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("CreateRoleTemplate: IAM response missing template id")
	}

	log.Printf("[IAMClient] Created role template: code=%s id=%s", code, result.ID)
	return result.ID, nil
}

// AssignRoleTemplateToUser assigns a functional role template to a user in IAM.
func (c *IAMClient) AssignRoleTemplateToUser(userID, templateID string) error {
	path := fmt.Sprintf("/api/v1/users/%s/roles", userID)
	req := map[string]string{"role_template_id": templateID}
	respBody, statusCode, err := c.doRequest("POST", path, req)
	if err != nil {
		return fmt.Errorf("AssignRoleTemplateToUser request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("AssignRoleTemplateToUser returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Assigned role template %s to user %s", templateID, userID)
	return nil
}

// AssignRoleTemplateToUserWithToken assigns a functional role template using the caller's token.
func (c *IAMClient) AssignRoleTemplateToUserWithToken(token, userID, orgID, roleCode string) error {
	path := fmt.Sprintf("/api/v1/users/%s/roles", userID)
	req := map[string]interface{}{"role_ids": []string{roleCode}, "org_id": orgID}
	respBody, statusCode, err := c.doRequestWithToken("POST", path, token, req)
	if err != nil {
		return fmt.Errorf("AssignRoleTemplateToUserWithToken request failed: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("AssignRoleTemplateToUserWithToken returned status %d: %s", statusCode, string(respBody))
	}
	log.Printf("[IAMClient] Assigned role template %s to user %s (via user token)", roleCode, userID)
	return nil
}

// NamespaceAppResponse represents a registered OAuth app returned by IAM.
type NamespaceAppResponse struct {
	AppID        string   `json:"app_id"`
	AppType      string   `json:"app_type"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	IsActive     bool     `json:"is_active"`
}

// RegisterNamespaceApp creates or returns an existing OAuth app for a namespace.
// Uses X-Namespace-Secret (from IAM_SECRET env) for authentication, not OAuth token.
func (c *IAMClient) RegisterNamespaceApp(namespaceID, appType, redirectURIs string) (*NamespaceAppResponse, error) {
	nsSecret := os.Getenv("IAM_SECRET")
	if nsSecret == "" {
		return nil, fmt.Errorf("RegisterNamespaceApp: IAM_SECRET not set")
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/apps", namespaceID)
	body, err := json.Marshal(map[string]interface{}{
		"type":           appType,
		"redirect_uris": []string{redirectURIs},
	})
	if err != nil {
		return nil, fmt.Errorf("RegisterNamespaceApp: failed to marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", c.baseURL, path), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("RegisterNamespaceApp: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Namespace-Secret", nsSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RegisterNamespaceApp request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("RegisterNamespaceApp: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("RegisterNamespaceApp returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var appResp NamespaceAppResponse
	if err := json.Unmarshal(respBody, &appResp); err != nil {
		return nil, fmt.Errorf("RegisterNamespaceApp: failed to parse response: %w", err)
	}
	if appResp.AppID == "" {
		return nil, fmt.Errorf("RegisterNamespaceApp: IAM response missing app_id")
	}
	return &appResp, nil
}

// CreateAdminUser creates an admin user for a namespace (cold start).
// Uses X-Namespace-Secret (from IAM_SECRET env) for authentication, not OAuth token.
// IAM generates a random password and sends it via email.
// Idempotent: if the email already exists, returns the existing user ID.
func (c *IAMClient) CreateAdminUser(namespaceID, email, name string) (string, error) {
	nsSecret := os.Getenv("IAM_SECRET")
	if nsSecret == "" {
		return "", fmt.Errorf("CreateAdminUser: IAM_SECRET not set")
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/admin", namespaceID)
	body, err := json.Marshal(map[string]string{
		"email": email,
		"name":  name,
	})
	if err != nil {
		return "", fmt.Errorf("CreateAdminUser: failed to marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", c.baseURL, path), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("CreateAdminUser: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Namespace-Secret", nsSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("CreateAdminUser request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("CreateAdminUser: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("CreateAdminUser returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Extract user ID from nested response: {"user": {"id": "..."}, ...}
	var result struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("CreateAdminUser: failed to parse response: %w", err)
	}
	if result.User.ID == "" {
		return "", fmt.Errorf("CreateAdminUser: IAM response missing user id")
	}
	return result.User.ID, nil
}

// ExchangeCode exchanges an OAuth authorization code for a token using explicit app credentials.
// Used by the callback handler instead of namespace credentials.
func ExchangeCode(clientID, clientSecret, code, redirectURI string) (*TokenResponse, error) {
	iamURL := GetIAMInternalURL()
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	if redirectURI != "" {
		payload["redirect_uri"] = redirectURI
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ExchangeCode: marshal failed: %w", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/api/v1/auth/token", iamURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("ExchangeCode: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ExchangeCode: read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ExchangeCode returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("ExchangeCode: parse failed: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("ExchangeCode: IAM response missing access_token")
	}
	return &tokenResp, nil
}

type AppRegistration struct {
	AppType      string   `json:"type"`
	RedirectURIs []string `json:"redirect_uris"`
	IsDefault    bool     `json:"is_default,omitempty"`
}

type ActivateNamespaceResponse struct {
	NamespaceID string                 `json:"namespace_id"`
	Status      string                 `json:"status"`
	OrgID       string                 `json:"org_id"`
	Apps        []NamespaceAppResponse `json:"apps"`
}

// ActivateNamespace activates a namespace and creates OAuth apps + same-name org.
// Uses X-Namespace-Secret auth. Requires beaconiam #169 + #177.
func (c *IAMClient) ActivateNamespace(namespaceID string, apps []AppRegistration) (*ActivateNamespaceResponse, error) {
	nsSecret := os.Getenv("IAM_SECRET")
	if nsSecret == "" {
		return nil, fmt.Errorf("ActivateNamespace: IAM_SECRET not set")
	}

	body, err := json.Marshal(map[string]interface{}{"apps": apps})
	if err != nil {
		return nil, fmt.Errorf("ActivateNamespace: marshal failed: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/namespaces/%s/activate", c.baseURL, namespaceID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ActivateNamespace: create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Namespace-Secret", nsSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ActivateNamespace: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ActivateNamespace: read failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ActivateNamespace returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var actResp ActivateNamespaceResponse
	if err := json.Unmarshal(respBody, &actResp); err != nil {
		return nil, fmt.Errorf("ActivateNamespace: parse failed: %w", err)
	}
	if actResp.NamespaceID == "" {
		return nil, fmt.Errorf("ActivateNamespace: IAM response missing namespace_id")
	}
	return &actResp, nil
}

// BindUserToOrg binds a user to an organization using client credentials (not namespace secret).
func (c *IAMClient) BindUserToOrg(orgID, userID string) error {
	respBody, statusCode, err := c.doRequest("PUT",
		fmt.Sprintf("/api/v1/organizations/%s/users/%s/bind", orgID, userID),
		map[string]string{"action": "bind", "role": "OWNER"})
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("BindUserToOrg returned status %d: %s", statusCode, string(respBody))
	}
	return nil
}
