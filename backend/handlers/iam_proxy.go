package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
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
	url := fmt.Sprintf("%s/api/v1/users/lookup?identifier=%s", h.baseURL, identifier)
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

	// Forward IAM response with original status code
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
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

	// Forward response
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
