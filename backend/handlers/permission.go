package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Role struct {
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	IsSystem    bool   `gorm:"default:false" json:"is_system"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Permission struct {
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Category    string `gorm:"type:varchar(50);not null" json:"category"`
	Description string `gorm:"type:text" json:"description"`
	CreatedAt   string `json:"created_at"`
}

type RolePermission struct {
	RoleID       string `gorm:"type:uuid;not null" json:"role_id"`
	PermissionID string `gorm:"type:uuid;not null" json:"permission_id"`
}

type PermissionHandler struct {
	db *gorm.DB
}

func NewPermissionHandler(db *gorm.DB) *PermissionHandler {
	return &PermissionHandler{db: db}
}

func (h *PermissionHandler) GetPermissions(c *gin.Context) {
	var permissions []Permission
	if err := h.db.Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch permissions: " + err.Error(),
		})
		return
	}

	grouped := make(map[string][]Permission)
	for _, p := range permissions {
		grouped[p.Category] = append(grouped[p.Category], p)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"data":    permissions,
		"grouped": grouped,
	})
}

func (h *PermissionHandler) GetRoles(c *gin.Context) {
	var roles []Role
	if err := h.db.Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch roles: " + err.Error(),
		})
		return
	}

	type RoleWithCount struct {
		Role
		PermissionCount int      `json:"permission_count"`
		Permissions     []string `json:"permissions"`
	}

	result := make([]RoleWithCount, len(roles))
	for i, role := range roles {
		var permissionNames []string
		h.db.Model(&Permission{}).
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ?", role.ID).
			Pluck("permissions.name", &permissionNames)

		result[i] = RoleWithCount{
			Role:            role,
			PermissionCount: len(permissionNames),
			Permissions:     permissionNames,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}

func (h *PermissionHandler) GetRolePermissions(c *gin.Context) {
	roleID := c.Param("id")

	var role Role
	if err := h.db.Where("id = ?", roleID).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch role: " + err.Error(),
		})
		return
	}

	var permissionNames []string
	h.db.Model(&Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Pluck("permissions.name", &permissionNames)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"role":        role,
			"permissions": permissionNames,
		},
	})
}

func (h *PermissionHandler) UpdateRolePermissions(c *gin.Context) {
	roleID := c.Param("id")

	var role Role
	if err := h.db.Where("id = ?", roleID).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch role: " + err.Error(),
		})
		return
	}

	var req struct {
		Permissions []string `json:"permissions" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40000,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	tx := h.db.Begin()
	if err := tx.Delete(&RolePermission{}, "role_id = ?", roleID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to clear permissions: " + err.Error(),
		})
		return
	}

	for _, permName := range req.Permissions {
		var permission Permission
		if err := tx.Where("name = ?", permName).First(&permission).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40000,
				"message": "Invalid permission: " + permName,
			})
			return
		}

		if err := tx.Create(&RolePermission{
			RoleID:       roleID,
			PermissionID: permission.ID,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to assign permission: " + err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to commit transaction: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Permissions updated successfully",
	})
}

func (h *PermissionHandler) CreateRole(c *gin.Context) {
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40000,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	role := Role{
		Name:        req.Name,
		Description: req.Description,
		IsSystem:    false,
	}

	tx := h.db.Begin()

	if err := tx.Create(&role).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create role: " + err.Error(),
		})
		return
	}

	for _, permName := range req.Permissions {
		var permission Permission
		if err := tx.Where("name = ?", permName).First(&permission).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40000,
				"message": "Invalid permission: " + permName,
			})
			return
		}

		if err := tx.Create(&RolePermission{
			RoleID:       role.ID,
			PermissionID: permission.ID,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to assign permission: " + err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to commit transaction: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":          role.ID,
			"name":        role.Name,
			"description": role.Description,
			"permissions": req.Permissions,
		},
	})
}

func (h *PermissionHandler) DeleteRole(c *gin.Context) {
	roleID := c.Param("id")

	var role Role
	if err := h.db.Where("id = ?", roleID).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch role: " + err.Error(),
		})
		return
	}

	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "Cannot delete system role",
		})
		return
	}

	if err := h.db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to delete role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Role deleted successfully",
	})
}
