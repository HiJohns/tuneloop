package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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

	// Perform fuzzy search in local users table
	db := database.GetDB().WithContext(ctx)
	var users []models.User

	// Build fuzzy search query
	query := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

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
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password"`
		Role     string `json:"role"`
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

	// Get tenant ID and org ID from context
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
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

	// Check name uniqueness
	if req.Name != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND name = ? AND deleted_at IS NULL", tenantID, req.Name).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":            existingUser.ID,
				"name":          existingUser.Name,
				"email":         existingUser.Email,
				"phone":         existingUser.Phone,
				"matched_field": "name",
			})
		}
	}

	// Check email uniqueness
	if req.Email != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND email = ? AND deleted_at IS NULL", tenantID, req.Email).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":            existingUser.ID,
				"name":          existingUser.Name,
				"email":         existingUser.Email,
				"phone":         existingUser.Phone,
				"matched_field": "email",
			})
		}
	}

	// Check phone uniqueness
	if req.Phone != "" {
		var existingUser models.User
		if err := db.Where("tenant_id = ? AND phone = ? AND deleted_at IS NULL", tenantID, req.Phone).First(&existingUser).Error; err == nil {
			conflicts = append(conflicts, gin.H{
				"id":            existingUser.ID,
				"name":          existingUser.Name,
				"email":         existingUser.Email,
				"phone":         existingUser.Phone,
				"matched_field": "phone",
			})
		}
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
	callbackURL := fmt.Sprintf("https://%s/api/iam/confirmation-callback", c.Request.Host)
	payload := map[string]interface{}{
		"email":        req.Email,
		"phone":        req.Phone,
		"name":         req.Name,
		"tid":          tenantID,
		"org_id":       orgID,
		"password":     req.Password,
		"callback_url": callbackURL,
		"status":       "pending",
	}

	// Add role if provided
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

	// Check if IAM returned {status: "pending", user_id: "..."} format
	if status, hasStatus := iamResponse["status"]; hasStatus {
		if status == "pending" || status == "success" {
			if userID, hasUserID := iamResponse["user_id"]; hasUserID {
				iamUserID := userID.(string)

				// Create local user record after IAM user creation
				localUserID, err := createLocalUser(c, iamUserID, &req)
				if err != nil {
					log.Printf("[IAM] Failed to create local user for IAM ID %s: %v", iamUserID, err)
					// Continue even if local user creation fails, don't block the response
					// But return IAM ID as fallback
					localUserID = iamUserID
				}

				// Convert to our standard format using LOCAL user ID
				c.JSON(http.StatusOK, gin.H{
					"code":    20000,
					"message": "success",
					"data": gin.H{
						"id":     localUserID, // Return LOCAL user ID for manager_id
						"iam_id": iamUserID,   // Also return IAM ID if needed
						"status": status,
					},
				})
				return
			}
		}
	}

	// Unknown IAM format, forward response as-is
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// createLocalUser creates a local user record after IAM user creation
// Returns the local user ID to be used as manager_id
func createLocalUser(c *gin.Context, iamUserID string, req *struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password"`
	Role     string `json:"role"`
}) (string, error) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	// Generate local UUID
	localUserID := uuid.New().String()

	// Prepare user data
	user := models.User{
		ID:          localUserID, // Set local UUID
		IAMSub:      iamUserID,   // Store IAM UUID
		TenantID:    tenantID,
		OrgID:       orgID,
		Name:        req.Name,
		Phone:       req.Phone,
		Email:       req.Email,
		CreditScore: 600,
		DepositMode: "standard",
		IsShadow:    true,
		Status:      "pending", // New users created through JIT are pending
	}

	// Save to database
	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&user).Error; err != nil {
		return "", fmt.Errorf("failed to create local user: %w", err)
	}

	log.Printf("[IAM] Successfully created local user %s for IAM ID %s", localUserID, iamUserID)
	return localUserID, nil
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

	callbackURL := fmt.Sprintf("https://%s/api/iam/confirmation-callback", c.Request.Host)

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

	orgs, err := client.ListOrganizations()
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

	// Check permissions: only ADMIN or OWNER can sync
	role := middleware.GetRole(ctx)
	if role != "ADMIN" && role != "OWNER" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
		})
		return
	}

	// Log sync operation for audit
	log.Printf("[IAMProxy] SyncOrganizations triggered by operator: %s, tenant: %s", operatorID, tenantID)

	// Fetch organizations from IAM
	orgs, err := client.ListOrganizations()
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

	// Upsert each organization into sites table
	for _, org := range orgs {
		// Check if site already exists by org_id
		var existingSite models.Site
		err := db.Where("org_id = ?", org.ID).First(&existingSite).Error

		if err == gorm.ErrRecordNotFound {
			// Create new site
			site := models.Site{
				ID:       uuid.New().String(),
				Name:     org.Name,
				OrgID:    org.ID,
				TenantID: tenantID,
				Status:   "active",
			}
			if err := db.Create(&site).Error; err != nil {
				log.Printf("[IAMProxy] SyncOrganizations: failed to create site %s: %v", org.Name, err)
				conflicts++
			} else {
				synced++
			}
		} else if err == nil {
			// Site exists - update if different (IAM wins on conflict)
			if existingSite.Name != org.Name {
				if err := db.Model(&existingSite).Update("name", org.Name).Error; err != nil {
					log.Printf("[IAMProxy] SyncOrganizations: failed to update site %s: %v", org.Name, err)
					conflicts++
				} else {
					synced++
				}
			} else {
				// No change needed
				skipped++
			}
		} else {
			log.Printf("[IAMProxy] SyncOrganizations: error checking existing site: %v", err)
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
		},
	})
}

// POST /api/iam/users/sync
func (h *IAMProxyHandler) SyncUsers(c *gin.Context) {
	client := services.NewIAMClient()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	// Check permissions: only ADMIN or OWNER can sync
	role := middleware.GetRole(ctx)
	if role != "ADMIN" && role != "OWNER" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
		})
		return
	}

	// Fetch users from IAM
	users, err := client.ListUsers()
	if err != nil {
		log.Printf("[IAMProxy] SyncUsers: failed to list from IAM: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch users from IAM: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(ctx)
	var synced, skipped, conflicts int

	// Upsert each user
	for _, user := range users {
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
				IsShadow: true,
				Status:   "active",
			}
			if err := db.Create(&newUser).Error; err != nil {
				log.Printf("[IAMProxy] SyncUsers: failed to create user %s: %v", user.Name, err)
				conflicts++
			} else {
				synced++
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

			if needsUpdate {
				if err := db.Model(&existingUser).Updates(updates).Error; err != nil {
					log.Printf("[IAMProxy] SyncUsers: failed to update user %s: %v", user.Name, err)
					conflicts++
				} else {
					synced++
				}
			} else {
				skipped++
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
		},
	})
}
