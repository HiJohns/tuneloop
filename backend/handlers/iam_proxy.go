package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IAMProxyHandler struct {
	baseURL string
}

func NewIAMProxyHandler() *IAMProxyHandler {
	return &IAMProxyHandler{
		baseURL: services.GetIAMInternalURL(),
	}
}

// GET /api/iam/users/lookup?identifier=xxx - Query user by phone or email
func (h *IAMProxyHandler) LookupUser(c *gin.Context) {
	identifier := c.Query("identifier")
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "identifier is required",
		})
		return
	}

	// Forward request to IAM
	url := fmt.Sprintf("%s/api/v1/users/lookup?email=%s", h.baseURL, identifier)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create request: " + err.Error(),
		})
		return
	}

	// Add authentication header if available
	if token, err := c.Cookie("token"); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to call IAM: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to read response: " + err.Error(),
		})
		return
	}

	// Wrap IAM response with unified format
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		// If IAM returns non-JSON error, wrap it
		c.JSON(http.StatusOK, gin.H{
			"code":    50000,
			"message": string(body),
		})
		return
	}

	// Map IAM status codes to our code format
	var code int
	switch resp.StatusCode {
	case http.StatusOK:
		code = 20000
	case http.StatusNotFound:
		code = 40400
	case http.StatusBadRequest:
		code = 40000
	default:
		code = 50000
	}

	// Preserve IAM response in data if it was successful
	if code == 20000 {
		c.JSON(http.StatusOK, gin.H{
			"code": code,
			"data": response,
		})
	} else {
		// For errors, wrap IAM message
		message := "user not found"
		if msg, ok := response["message"].(string); ok && msg != "" {
			message = msg
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": message,
		})
	}
}

// GET /api/iam/users/search?q=xxx&limit=10&merchant_id=xxx - Fuzzy search users
func (h *IAMProxyHandler) SearchUsers(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "q (search query) is required",
		})
		return
	}

	// Get limit parameter with default 10
	limitStr := c.DefaultQuery("limit", "10")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
		limit = l
	}

	// Get tenant ID from context
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40100,
			"message": "tenant not authenticated",
		})
		return
	}

	merchantID := c.Query("merchant_id")

	// Perform fuzzy search in local users table (across all tenants)
	db := database.GetDB()
	var users []models.User

	// Build fuzzy search query
	query := db.Where("deleted_at IS NULL")

	// Search in name, email, and phone fields
	searchPattern := "%" + q + "%"
	query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?",
		searchPattern, searchPattern, searchPattern)

	// Limit results
	query = query.Limit(limit)

	// Execute query
	if err := query.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to search users: " + err.Error(),
		})
		return
	}

	// Build response
	var result []gin.H
	for _, user := range users {
		// Determine which field matched
		matchedField := ""
		if user.Name != "" && containsIgnoreCase(user.Name, q) {
			matchedField = "name"
		} else if user.Email != "" && containsIgnoreCase(user.Email, q) {
			matchedField = "email"
		} else if user.Phone != "" && containsIgnoreCase(user.Phone, q) {
			matchedField = "phone"
		}

		// Check if user is associated with merchant
		associated := false
		if merchantID != "" {
			var count int64
			db.Model(&models.SiteMember{}).
				Where("tenant_id = ? AND user_id = ? AND site_id IN (SELECT id FROM sites WHERE org_id = ?)",
					tenantID, user.ID, merchantID).
				Count(&count)
			associated = count > 0
		}

		result = append(result, gin.H{
			"id":            user.ID,
			"name":          user.Name,
			"email":         user.Email,
			"phone":         user.Phone,
			"matched_field": matchedField,
			"associated":    associated,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"users": result,
		},
	})
}

// Helper function for case-insensitive substring check
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(containsIgnoreCaseHelper(s, substr) || containsIgnoreCaseHelper(reverse(s), reverse(substr))))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// Simple case-insensitive check for ASCII
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// POST /api/iam/users - Create IAM user (JIT provisioning)
func (h *IAMProxyHandler) CreateUser(c *gin.Context) {
	var req struct {
		Email          string `json:"email"`
		Phone          string `json:"phone"`
		Name           string `json:"name" binding:"required"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		Role           string `json:"role"`
		SkipActivation bool   `json:"skip_activation"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Validate that at least email or phone is provided
	if req.Email == "" && req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "email or phone is required",
		})
		return
	}

	// Get tenant ID from context (used for uniqueness check scope only)
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40100,
			"message": "tenant not authenticated",
		})
		return
	}

	// Check uniqueness: name, email, or phone
	db := database.GetDB().WithContext(ctx)
	var conflicts []gin.H
	seen := make(map[string]*models.User)      // userID -> user
	matchedFields := make(map[string][]string) // userID -> matched_fields

	addConflict := func(user *models.User, field string) {
		if _, exists := seen[user.ID]; !exists {
			seen[user.ID] = user
		}
		matchedFields[user.ID] = append(matchedFields[user.ID], field)
	}

	// Check name uniqueness
	if req.Name != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND name = ? AND deleted_at IS NULL", tenantID, req.Name).First(&existingUser).Error; err == nil {
			addConflict(&existingUser, "name")
		}
	}

	// Check email uniqueness
	if req.Email != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND email = ? AND deleted_at IS NULL", tenantID, req.Email).First(&existingUser).Error; err == nil {
			addConflict(&existingUser, "email")
		}
	}

	// Check phone uniqueness
	if req.Phone != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND phone = ? AND deleted_at IS NULL", tenantID, req.Phone).First(&existingUser).Error; err == nil {
			addConflict(&existingUser, "phone")
		}
	}

	for userID, user := range seen {
		conflicts = append(conflicts, gin.H{
			"id":             userID,
			"name":           user.Name,
			"email":          user.Email,
			"phone":          user.Phone,
			"matched_fields": matchedFields[userID],
		})
	}

	// Return conflicts if any
	if len(conflicts) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "user with same name, email, or phone already exists",
			"data": gin.H{
				"conflicts": conflicts,
			},
		})
		return
	}

	// Prepare request payload
	var initialPassword string
	callbackURL := os.Getenv("EXTERNAL_WEB_URL")
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://%s", c.Request.Host)
	}
	reason := "您已被设为管理员"
	payload := map[string]interface{}{
		"email":    req.Email,
		"phone":    req.Phone,
		"name":     req.Name,
		"username": req.Username,
		"reason":   reason,
	}

	if req.SkipActivation {
		pwd := generatePassword()
		payload["password"] = pwd
		payload["skip_activation"] = true
		payload["send_notification_email"] = true
		payload["notification_lang"] = middleware.GetCulture(c)
		initialPassword = pwd
	} else {
		payload["callback_url"] = callbackURL
		payload["status"] = "pending"
	}

	if req.Password != "" {
		payload["password"] = req.Password
	}
	if req.Role != "" {
		payload["role"] = req.Role
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to marshal payload: " + err.Error(),
		})
		return
	}

	// Forward to IAM
	url := fmt.Sprintf("%s/api/v1/users", h.baseURL)
	iamReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create request: " + err.Error(),
		})
		return
	}

	// Set headers
	iamReq.Header.Set("Content-Type", "application/json")
	if token, err := c.Cookie("token"); err == nil {
		iamReq.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(iamReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to call IAM: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to read response: " + err.Error(),
		})
		return
	}

	// Try to parse IAM response to determine format
	var iamResponse map[string]interface{}
	if err := json.Unmarshal(body, &iamResponse); err != nil {
		// If can't parse, forward as-is
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	// Check if IAM returned our expected format
	if _, exists := iamResponse["code"]; exists {
		// Already in our format, forward as-is
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	// Check if IAM returned {status: "pending", user_id: "..."} format (or active for skip_activation)
	if status, hasStatus := iamResponse["status"]; hasStatus {
		if status == "pending" || status == "success" || status == "active" {
			if userID, hasUserID := iamResponse["user_id"]; hasUserID {
				iamUserID, ok := userID.(string)
				if !ok || iamUserID == "" {
					log.Printf("[IAM] IAM response has invalid or empty user_id field")
					c.JSON(http.StatusInternalServerError, gin.H{
						"code":    50000,
						"message": "IAM returned invalid user_id",
					})
					return
				}

				// Create local user record after IAM user creation
				localStatus := "pending"
				if req.SkipActivation {
					localStatus = "active"
				}
				localUserID, err := createLocalUserWithStatus(c, iamUserID, &req, localStatus)
				if err != nil {
					log.Printf("[IAM] Failed to create local user for IAM ID %s: %v", iamUserID, err)
					localUserID = iamUserID
				}

				respData := gin.H{
					"id":     localUserID,
					"iam_id": iamUserID,
					"status": localStatus,
				}
				if initialPassword != "" {
					respData["initial_password"] = initialPassword
				}

				c.JSON(http.StatusOK, gin.H{
					"code":    20000,
					"message": "success",
					"data":    respData,
				})
				return
			}
		}
	}

	// Unknown IAM format, forward response as-is
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// createLocalUserWithStatus creates a local user record after IAM user creation.
func createLocalUserWithStatus(c *gin.Context, iamUserID string, req *struct {
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Name           string `json:"name" binding:"required"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Role           string `json:"role"`
	SkipActivation bool   `json:"skip_activation"`
}, status string) (string, error) {
	nilUUID := "00000000-0000-0000-0000-000000000000"
	localUserID := uuid.New().String()

	user := models.User{
		ID:          localUserID,
		IAMSub:      iamUserID,
		TenantID:    nilUUID,
		OrgID:       nilUUID,
		Name:        req.Name,
		Phone:       req.Phone,
		Email:       req.Email,
		CreditScore: 600,
		DepositMode: "standard",
		IsShadow:    true,
		Status:      status,
	}

	db := database.GetDB().WithContext(c.Request.Context())
	if err := db.Create(&user).Error; err != nil {
		return "", fmt.Errorf("failed to create local user: %w", err)
	}

	log.Printf("[IAM] Successfully created local user %s for IAM ID %s", localUserID, iamUserID)
	return localUserID, nil
}

// createLocalUser creates a local user record with "pending" status (backward compat).
func createLocalUser(c *gin.Context, iamUserID string, req *struct {
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Name           string `json:"name" binding:"required"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Role           string `json:"role"`
	SkipActivation bool   `json:"skip_activation"`
}) (string, error) {
	return createLocalUserWithStatus(c, iamUserID, req, "pending")
}

// UpdateIAMUser PUT /api/iam/users/:id - Update user via IAM
func (h *IAMProxyHandler) UpdateIAMUser(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "user id is required",
		})
		return
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	callbackURL := os.Getenv("EXTERNAL_WEB_URL")
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://%s", c.Request.Host)
	}

	iamClient := services.NewIAMClient()
	iamReq := &services.UpdateUserRequest{
		Name:        req.Name,
		Email:       req.Email,
		Phone:       req.Phone,
		Password:    req.Password,
		CallbackURL: callbackURL,
		OperatorID:  middleware.GetUserID(ctx),
	}

	if err := iamClient.UpdateUser(userID, iamReq); err != nil {
		log.Printf("[UpdateIAMUser] IAM UpdateUser failed for %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update user in IAM: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	localUpdates := map[string]interface{}{}
	if req.Name != "" {
		localUpdates["name"] = req.Name
	}
	if req.Phone != "" {
		localUpdates["phone"] = req.Phone
	}
	if len(localUpdates) > 0 {
		db.Model(&models.User{}).Where("iam_sub = ? AND tenant_id = ?", userID, tenantID).Updates(localUpdates)
	}

	emailChanged := req.Email != ""
	responseData := gin.H{
		"id":     userID,
		"status": "success",
	}
	if emailChanged {
		responseData["email_confirmation"] = "pending"
		responseData["message"] = "Email change requires confirmation. IAM will send a confirmation email."
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    responseData,
	})
}

// InviteUserToMerchant POST /api/iam/users/:user_id/invite - Invite user to join merchant
func (h *IAMProxyHandler) InviteUserToMerchant(c *gin.Context) {
	userID := c.Param("user_id")
	merchantID := c.Query("merchant_id")

	if userID == "" || merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "user_id and merchant_id are required",
		})
		return
	}

	// In production: Call IAM API to associate user with organization
	// For now: return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Invitation sent successfully",
		"data": gin.H{
			"user_id":     userID,
			"merchant_id": merchantID,
			"status":      "pending_invitation",
		},
	})
}

// GET /api/iam/organizations
func (h *IAMProxyHandler) ListOrganizations(c *gin.Context) {
	client := services.NewIAMClient()
	// Make sure user is authenticated
	userToken := services.ExtractUserToken(c)
	if userToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40100,
			"message": "authentication required",
		})
		return
	}

	orgs, err := client.ListOrganizationsWithToken(userToken)
	if err != nil {
		log.Printf("[IAMProxy] ListOrganizations failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch organizations: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"data":    gin.H{"list": orgs},
		"message": "success",
	})
}

// POST /api/iam/organizations/sync
func (h *IAMProxyHandler) SyncOrganizations(c *gin.Context) {
	client := services.NewIAMClient()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	operatorID := middleware.GetUserID(ctx)
	userToken := services.ExtractUserToken(c)

	// Check permissions: merchant admin (tid == oid) can sync
	tid := middleware.GetTenantID(ctx)
	oid := middleware.GetOrgID(ctx)
	if tid != oid || tid == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
		})
		return
	}

	// Log sync operation for audit
	log.Printf("[IAMProxy] SyncOrganizations triggered by operator: %s, tenant: %s", operatorID, tenantID)

	// Fetch organizations from IAM
	orgs, err := client.ListOrganizationsWithToken(userToken)
	if err != nil {
		log.Printf("[IAMProxy] SyncOrganizations: failed to list from IAM: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch organizations from IAM: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	var synced, skipped, conflicts int
	var details []gin.H

	// Map: IAM org_id -> local site_id (used for parent resolution)
	orgToSite := make(map[string]string)

	// Pass 1: Upsert each organization into sites table (without parent_id)
	userOrgID := middleware.GetOrgID(ctx)
	for _, org := range orgs {
		// Skip the merchant org itself (it's not a site)
		if org.ID == userOrgID {
			skipped++
			details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "merchant", "result": "skipped"})
			continue
		}
		// Only sync orgs under the current user's merchant org
		if org.ParentID == nil || *org.ParentID != userOrgID {
			skipped++
			details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "unknown", "result": "skipped"})
			continue
		}
		// Check if site already exists by org_id
		var existingSite models.Site
		err := db.Where("org_id = ?", org.ID).First(&existingSite).Error

		if err == gorm.ErrRecordNotFound {
			// Create new site
			site := models.Site{
				ID:       uuid.New().String(),
				Name:     org.Name,
				OrgID:    org.ID,
				TenantID: userOrgID,
				Status:   "active",
			}
			if err := db.Create(&site).Error; err != nil {
				log.Printf("[IAMProxy] SyncOrganizations: failed to create site %s: %v", org.Name, err)
				conflicts++
				continue
			}
			orgToSite[org.ID] = site.ID
			synced++
			details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "site", "result": "added"})
		} else if err == nil {
			// Site exists - update if different (IAM wins on conflict)
			needsUpdate := false
			if existingSite.Name != org.Name {
				needsUpdate = true
			}
			if existingSite.TenantID != userOrgID {
				needsUpdate = true
			}
			if needsUpdate {
				updates := map[string]interface{}{}
				if existingSite.Name != org.Name {
					updates["name"] = org.Name
				}
				if existingSite.TenantID != userOrgID {
					updates["tenant_id"] = userOrgID
				}
				if err := db.Model(&existingSite).Updates(updates).Error; err != nil {
					log.Printf("[IAMProxy] SyncOrganizations: failed to update site %s: %v", org.Name, err)
					conflicts++
					continue
				}
				synced++
				details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "site", "result": "updated"})
			} else {
				skipped++
				details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "site", "result": "existing"})
			}
		} else {
			log.Printf("[IAMProxy] SyncOrganizations: error checking existing site: %v", err)
			conflicts++
			details = append(details, gin.H{"id": org.ID, "name": org.Name, "parent_id": org.ParentID, "kind": "site", "result": "error"})
		}
	}

	// Pass 2: Resolve parent_id for all orgs that have one
	for _, org := range orgs {
		if org.ParentID == nil || *org.ParentID == "" {
			continue
		}

		// Find local site_id for the parent org
		parentSiteID, ok := orgToSite[*org.ParentID]
		if !ok {
			// Parent not in this sync batch, try DB lookup
			var parentSite models.Site
			if err := db.Where("org_id = ?", *org.ParentID).First(&parentSite).Error; err == nil {
				parentSiteID = parentSite.ID
			} else {
				log.Printf("[IAMProxy] SyncOrganizations: parent org %s not found for site %s", *org.ParentID, org.Name)
				continue
			}
		}

		parentUUID, err := uuid.Parse(parentSiteID)
		if err != nil {
			log.Printf("[IAMProxy] SyncOrganizations: invalid parent site ID %s: %v", parentSiteID, err)
			continue
		}

		if err := db.Model(&models.Site{}).Where("org_id = ?", org.ID).Update("parent_id", parentUUID).Error; err != nil {
			log.Printf("[IAMProxy] SyncOrganizations: failed to update parent_id for site %s: %v", org.Name, err)
		}
	}

	// Cleanup: mark stale local sites as deleted and remove duplicates
	var iamOrgIDs []string
	for _, org := range orgs {
		if org.ID != userOrgID && org.ParentID != nil && *org.ParentID == userOrgID {
			iamOrgIDs = append(iamOrgIDs, org.ID)
		}
	}
	query := db.Model(&models.Site{}).Where("tenant_id = ? AND status = 'active'", tenantID)
	if len(iamOrgIDs) > 0 {
		query = query.Where("org_id NOT IN ?", iamOrgIDs)
	}
	staleResult := query.Update("status", "inactive")
	if staleResult.RowsAffected > 0 {
		log.Printf("[IAMProxy] SyncOrganizations: marked %d stale sites as inactive", staleResult.RowsAffected)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "sync completed",
		"data": gin.H{
			"synced":    synced,
			"skipped":   skipped,
			"conflicts": conflicts,
			"details":   details,
		},
	})
}

// POST /api/iam/users/sync
func (h *IAMProxyHandler) SyncUsers(c *gin.Context) {
	client := services.NewIAMClient()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userToken := services.ExtractUserToken(c)

	log.Printf("[IAMProxy] SyncUsers: tenantID=%s", tenantID)

	role := middleware.GetRole(ctx)
	if role != "ADMIN" && role != "OWNER" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
		})
		return
	}

	users, err := client.ListUsersWithToken(userToken)
	if err != nil {
		log.Printf("[IAMProxy] SyncUsers: failed to list from IAM: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch users from IAM: " + err.Error(),
		})
		return
	}

	log.Printf("[IAMProxy] SyncUsers: got %d users from IAM", len(users))

	db := database.GetDB().WithContext(ctx)
	var synced, skipped, conflicts int
	var details []gin.H

	// Build valid org scope: caller's merchant org + direct child orgs
	// Filters out users that don't belong to this merchant's hierarchy
	userOrgID := middleware.GetOrgID(ctx)
	validOrgIDs := map[string]bool{userOrgID: true}
	if orgs, err := client.ListOrganizationsWithToken(userToken); err == nil {
		for _, org := range orgs {
			if org.ParentID != nil && *org.ParentID == userOrgID {
				validOrgIDs[org.ID] = true
			}
		}
	}

	// Upsert each user
	for _, user := range users {
		// Determine matched org and role from user_org_relations (via include_orgs=true)
		// Prefer child orgs over the merchant org itself (the merchant org has no site).
		matchedOrgID := ""
		matchedRole := user.Role
		for _, org := range user.Organizations {
			if org.IsActive && validOrgIDs[org.ID] && org.ID != userOrgID {
				matchedOrgID = org.ID
				matchedRole = org.Role
				break
			}
		}
		// Fallback: merchant org itself (for users without a child org, e.g. merchant admin)
		if matchedOrgID == "" {
			for _, org := range user.Organizations {
				if org.IsActive && validOrgIDs[org.ID] {
					matchedOrgID = org.ID
					matchedRole = org.Role
					break
				}
			}
		}
		if matchedOrgID == "" {
			skipped++
			details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": user.OrgID, "result": "skipped"})
			continue
		}
		var existingUser models.User
		err := db.Where("iam_sub = ?", user.ID).Or("email = ?", user.Email).First(&existingUser).Error

		if err == gorm.ErrRecordNotFound {
			// Create new shadow user
			newUser := models.User{
				ID:       uuid.New().String(),
				IAMSub:   user.ID,
				Name:     user.Name,
				Email:    user.Email,
				Phone:    user.Phone,
				TenantID: tenantID,
				OrgID:    matchedOrgID,
				Role:     matchedRole,
				IsShadow: true,
				Status:   user.Status,
			}
			if err := db.Create(&newUser).Error; err != nil {
				log.Printf("[IAMProxy] SyncUsers: failed to create user %s: %v", user.Name, err)
				conflicts++
				details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": matchedOrgID, "result": "error"})
			} else {
				synced++
				ensureSiteMember(db, newUser.ID, matchedOrgID, matchedRole, tenantID)
				details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": matchedOrgID, "result": "added"})
			}
		} else if err == nil {
			// User exists - update if different (IAM wins)
			needsUpdate := false
			updates := map[string]interface{}{}

			if existingUser.Name != user.Name {
				updates["name"] = user.Name
				needsUpdate = true
			}
			if existingUser.Email != user.Email {
				updates["email"] = user.Email
				needsUpdate = true
			}
			if existingUser.Phone != user.Phone {
				updates["phone"] = user.Phone
				needsUpdate = true
			}
			if existingUser.Role != matchedRole {
				updates["role"] = matchedRole
				needsUpdate = true
			}
			if existingUser.Status != user.Status {
				updates["status"] = user.Status
				needsUpdate = true
			}
			if matchedOrgID != "" && existingUser.OrgID != matchedOrgID {
				updates["org_id"] = matchedOrgID
				needsUpdate = true
			}
			if existingUser.TenantID != tenantID {
				updates["tenant_id"] = tenantID
				needsUpdate = true
			}

			if needsUpdate {
				if err := db.Model(&existingUser).Updates(updates).Error; err != nil {
					log.Printf("[IAMProxy] SyncUsers: failed to update user %s: %v", user.Name, err)
					conflicts++
					details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": matchedOrgID, "result": "error"})
				} else {
					synced++
					ensureSiteMember(db, existingUser.ID, matchedOrgID, matchedRole, tenantID)
					details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": matchedOrgID, "result": "updated"})
				}
			} else {
				skipped++
				if ensureSiteMember(db, existingUser.ID, matchedOrgID, matchedRole, tenantID) {
					synced++
					skipped--
				}
				details = append(details, gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "org_id": matchedOrgID, "result": "existing"})
			}
		} else {
			log.Printf("[IAMProxy] SyncUsers: error checking existing user: %v", err)
			conflicts++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "sync completed",
		"data": gin.H{
			"synced":    synced,
			"skipped":   skipped,
			"conflicts": conflicts,
			"details":   details,
		},
	})
}

// mapIAMRoleToSiteRole translates IAM user roles to local site_members roles.
func mapIAMRoleToSiteRole(iamRole string) string {
	switch iamRole {
	case "OWNER":
		return "merchant_admin"
	case "ADMIN":
		return "site_admin"
	case "STAFF":
		return "site_member"
	case "repair_technician":
		return "worker"
	default:
		return "site_member"
	}
}

// ensureSiteMember creates a site_members record linking a user to a site.
func ensureSiteMember(db *gorm.DB, userID string, iamOrgID string, iamRole string, tenantID string) bool {
	if iamOrgID == "" || tenantID == "" {
		return false
	}
	var site models.Site
	if err := db.Where("org_id = ? AND status = 'active'", iamOrgID).First(&site).Error; err != nil {
		return false
	}
	var existingRole string
	db.Model(&models.SiteMember{}).Where("user_id = ? AND site_id = ?", userID, site.ID).Select("role").Scan(&existingRole)
	if existingRole != "" {
		siteRole := mapIAMRoleToSiteRole(iamRole)
		if existingRole != siteRole {
			db.Model(&models.SiteMember{}).Where("user_id = ? AND site_id = ?", userID, site.ID).Update("role", siteRole)
			log.Printf("[IAMProxy] SyncUsers: corrected site_member role for user %s site %s: %s -> %s", userID, site.ID, existingRole, siteRole)
			return true
		}
		return false
	}
	siteMember := models.SiteMember{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		SiteID:    site.ID,
		UserID:    userID,
		Role:      mapIAMRoleToSiteRole(iamRole),
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&siteMember).Error; err != nil {
		log.Printf("[IAMProxy] SyncUsers: failed to create site_member for user %s: %v", userID, err)
		return false
	}
	return true
}
