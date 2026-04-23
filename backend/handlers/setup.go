package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

type SetupHandler struct{}

func NewSetupHandler() *SetupHandler {
	return &SetupHandler{}
}

// GetSetupStatus GET /api/setup/status - Check if system needs initialization
func (h *SetupHandler) GetSetupStatus(c *gin.Context) {
	db := database.GetDB().WithContext(c.Request.Context())

	// Check if users table has any records
	var count int64
	result := db.Model(&models.User{}).Count(&count)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to check system status: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"requires_setup": count == 0,
			"user_count":     count,
		},
	})
}

// InitializeSystem POST /api/setup/init - Create the first system admin
func (h *SetupHandler) InitializeSystem(c *gin.Context) {
	db := database.GetDB().WithContext(c.Request.Context())

	// Check if already initialized
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "System already initialized",
		})
		return
	}

	var input struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	// TODO: Call IAM to create user with Project Admin role
	// For now, create a local shadow user
	// In production, this should call IAM API
	
	user := models.User{
		IAMSub:        "system-admin-" + input.Email, // Placeholder
		TenantID:      "default-tenant",              // Will be set properly by middleware
		OrgID:         "default-org",                 // Will be set properly by middleware
		Name:          "System Administrator",
		Email:         input.Email,
		IsSystemAdmin: true,
		IsShadow:      false, // This is a real admin, not a shadow
	}

	result := db.Create(&user)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create system admin: " + result.Error.Error(),
		})
		return
	}

	// Return OIDC URL for immediate authentication
	// In production, generate proper OIDC authorization URL
	oidcURL := "/api/auth/login" // Placeholder

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"user_id":  user.ID,
			"oidc_url": oidcURL,
			"message":  "System admin created. Redirecting to authentication...",
		},
	})
}
