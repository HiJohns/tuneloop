package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SiteMemberHandler struct{}

func NewSiteMemberHandler() *SiteMemberHandler {
	return &SiteMemberHandler{}
}

// ListMembers GET /api/sites/:id/members - List site members
func (h *SiteMemberHandler) ListMembers(c *gin.Context) {
	siteID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check site existence and permission
	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	// Get members with user details
	var members []struct {
		UserID    string    `json:"user_id"`
		UserName  string    `json:"user_name"`
		UserEmail string    `json:"user_email"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
	}

	db.Table("site_members").
		Select("site_members.user_id, users.name as user_name, users.email as user_email, site_members.role, site_members.created_at").
		Joins("JOIN users ON users.id = site_members.user_id").
		Where("site_members.site_id = ? AND site_members.tenant_id = ?", siteID, tenantID).
		Scan(&members)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": members,
		},
	})
}

// AddMember POST /api/sites/:id/members - Add member to site
func (h *SiteMemberHandler) AddMember(c *gin.Context) {
	siteID := c.Param("id")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check site access
	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var input struct {
		UserID string `json:"user_id" binding:"required"`
		Role   string `json:"role" default:"Staff"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	// Check if already a member
	var count int64
	db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, input.UserID).
		Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "User is already a member of this site",
		})
		return
	}

	// Create site member
	member := models.SiteMember{
		TenantID: tenantID,
		SiteID:   siteID,
		UserID:   input.UserID,
		Role:     input.Role,
	}

	result := db.Create(&member)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to add member: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": member,
	})
}

// UpdateMemberRole PUT /api/sites/:id/members/:uid - Update member role
func (h *SiteMemberHandler) UpdateMemberRole(c *gin.Context) {
	siteID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check site access
	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	var input struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid input: " + err.Error(),
		})
		return
	}

	// Check if trying to update to Manager
	if input.Role == "Manager" {
		// Check protection rule - users can be promoted to Manager
		// Only Staff -> Manager is allowed
	} else if input.Role == "Staff" {
		// Check if this would leave no Managers
		if isLastManager(db, tenantID, siteID, userID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "Cannot demote the last Manager",
			})
			return
		}
	}

	// Update member role
	result := db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, userID).
		Update("role", input.Role)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update member role: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"site_id": siteID,
			"user_id": userID,
			"new_role": input.Role,
		},
	})
}

// RemoveMember DELETE /api/sites/:id/members/:uid - Remove member from site
func (h *SiteMemberHandler) RemoveMember(c *gin.Context) {
	siteID := c.Param("id")
	userID := c.Param("uid")
	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Check site access
	if !hasSiteAccess(db, tenantID, siteID, c) {
		return
	}

	// Protection: Cannot remove the last Manager
	if isLastManager(db, tenantID, siteID, userID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Cannot remove the last Manager",
		})
		return
	}

	// Delete member
	result := db.Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, userID).
		Delete(&models.SiteMember{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to remove member: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Member not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Member removed successfully",
	})
}

// Helper Functions

// hasSiteAccess checks if user has access to site
func hasSiteAccess(db *gorm.DB, tenantID, siteID string, c *gin.Context) bool {
	var count int64
	result := db.Model(&models.Site{}).
		Where("id = ? AND tenant_id = ?", siteID, tenantID).
		Count(&count)

	if result.Error != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Site not found",
		})
		return false
	}
	return true
}

// isLastManager checks if user is the last Manager in site
func isLastManager(db *gorm.DB, tenantID, siteID, userID string) bool {
	var managerCount int64
	db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND role = ?", tenantID, siteID, "Manager").
		Count(&managerCount)

	if managerCount == 0 {
		return false
	}

	// Check if this user is a Manager
	var userRole string
	db.Model(&models.SiteMember{}).
		Select("role").
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", tenantID, siteID, userID).
		Scan(&userRole)

	return userRole == "Manager" && managerCount == 1
}
