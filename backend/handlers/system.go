package handlers

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

func (h *SystemHandler) proxyIAMRequest(c *gin.Context, iamPath string) {
	iamURL := c.GetString("iam_internal_url")
	if iamURL == "" {
		iamURL = "http://localhost:5552"
	}

	proxyURL, _ := url.Parse(iamURL + iamPath)

	// Copy headers
	headers := make(http.Header)
	for k, v := range c.Request.Header {
		headers[k] = v
	}

	// Forward the request
	resp, err := http.DefaultClient.Do(&http.Request{
		Method: c.Request.Method,
		URL:    proxyURL,
		Header: headers,
		Body:   c.Request.Body,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to proxy request to IAM"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers and status
	for k, v := range resp.Header {
		c.Header(k, v[0])
	}
	c.Status(resp.StatusCode)
}

// Client Management APIs
func (h *SystemHandler) GetClients(c *gin.Context) {
	h.proxyIAMRequest(c, "/api/v1/clients")
}

func (h *SystemHandler) CreateClient(c *gin.Context) {
	h.proxyIAMRequest(c, "/api/v1/clients")
}

func (h *SystemHandler) UpdateClient(c *gin.Context) {
	clientID := c.Param("id")
	h.proxyIAMRequest(c, "/api/v1/clients/"+clientID)
}

func (h *SystemHandler) DeleteClient(c *gin.Context) {
	clientID := c.Param("id")
	h.proxyIAMRequest(c, "/api/v1/clients/"+clientID)
}

// Tenant Management APIs
func (h *SystemHandler) GetTenants(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": []gin.H{},
	})
}

func (h *SystemHandler) CreateTenant(c *gin.Context) {
	var req struct {
		Name          string `json:"name" binding:"required"`
		OwnerEmail    string `json:"owner_email" binding:"required"`
		OwnerName     string `json:"owner_name" binding:"required"`
		OwnerPassword string `json:"owner_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	// TODO: Implement tenant creation with IAM API
	// This would call IAM's tenant creation endpoint and create owner account

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"tenant_id":     "new-tenant-id",
			"name":          req.Name,
			"owner_created": true,
		},
	})
}

func (h *SystemHandler) GetTenant(c *gin.Context) {
	tenantID := c.Param("id")
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":   tenantID,
			"name": "Sample Tenant",
		},
	})
}

func (h *SystemHandler) UpdateTenant(c *gin.Context) {
	tenantID := c.Param("id")
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":      tenantID,
			"updated": true,
		},
	})
}

func (h *SystemHandler) DeleteTenant(c *gin.Context) {
	tenantID := c.Param("id")
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":      tenantID,
			"deleted": true,
		},
	})
}
