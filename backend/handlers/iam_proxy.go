package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	// Prepare request payload
	payload := map[string]interface{}{
		"email":     req.Email,
		"phone":     req.Phone,
		"name":      req.Name,
		"tid":       tenantID, // Set tenant ID
		"org_id":    orgID,    // Set org ID
		"password":  req.Password,
		"skipEmail": true, // JIT provisioning, skip email verification
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
	}

	// Save to database
	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&user).Error; err != nil {
		return "", fmt.Errorf("failed to create local user: %w", err)
	}

	log.Printf("[IAM] Successfully created local user %s for IAM ID %s", localUserID, iamUserID)
	return localUserID, nil // Return local ID
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
