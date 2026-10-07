package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// GET /system/clients - List all clients
func (h *SystemHandler) GetClients(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var clients []models.Client
	// #2159：服务端分页 + 响应形态 {list,total}（原为裸数组）
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)
	q := db.Model(&models.Client{}).Where("tenant_id = ?", tenantID)
	var total int64
	q.Count(&total)
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&clients).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch clients: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": clients, "total": total, "page": page, "page_size": pageSize},
	})
}

// GET /system/tenants - List all tenants
func (h *SystemHandler) GetTenants(c *gin.Context) {
	db := database.GetDB()

	var tenants []models.Tenant
	// #2160：服务端分页 + 响应形态 {list,total}（原为裸数组）
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(parseInt(c.DefaultQuery("page_size", "20"), 20), 20, maxPageSize)
	q := db.Model(&models.Tenant{})
	var total int64
	q.Count(&total)
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&tenants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch tenants",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": tenants, "total": total, "page": page, "page_size": pageSize},
	})
}
