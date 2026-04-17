package handlers

import (
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MaintenanceWorkerHandler struct{}

func NewMaintenanceWorkerHandler() *MaintenanceWorkerHandler {
	return &MaintenanceWorkerHandler{}
}

// POST /api/maintenance/workers - Create maintenance worker
func (h *MaintenanceWorkerHandler) CreateWorker(c *gin.Context) {
	var req struct {
		Name  string `json:"name" binding:"required"`
		Phone string `json:"phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	worker := models.MaintenanceWorker{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     orgID,
		Name:      req.Name,
		Phone:     req.Phone,
		JoinDate:  &time.Time{},
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&worker).Error; err != nil {
		c.JSON(500, gin.H{"code": 50000, "message": "failed to create worker: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"code":    20000,
		"message": "success",
		"data":    worker,
	})
}

// GET /api/maintenance/workers - List maintenance workers with filters
func (h *MaintenanceWorkerHandler) ListWorkers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	name := c.Query("name")
	phone := c.Query("phone")
	siteID := c.Query("site_id")

	db := database.GetDB().WithContext(ctx)
	query := db.Model(&models.MaintenanceWorker{}).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if phone != "" {
		query = query.Where("phone LIKE ?", "%"+phone+"%")
	}
	if siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}

	var workers []models.MaintenanceWorker
	if err := query.Find(&workers).Error; err != nil {
		c.JSON(500, gin.H{"code": 50000, "message": "failed to query workers: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": workers,
		},
	})
}

// GET /api/maintenance/workers/:id - Get worker details with history
func (h *MaintenanceWorkerHandler) GetWorker(c *gin.Context) {
	workerID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Get worker info
	var worker models.MaintenanceWorker
	if err := db.Where("id = ? AND tenant_id = ?", workerID, tenantID).First(&worker).Error; err != nil {
		c.JSON(404, gin.H{"code": 40400, "message": "worker not found"})
		return
	}

	// Get worker's recent orders (simplified - in real implementation would join with maintenance sessions)
	type WorkerDetail struct {
		models.MaintenanceWorker
		RecentOrders []gin.H `json:"recent_orders"`
	}

	detail := WorkerDetail{
		MaintenanceWorker: worker,
		RecentOrders:      []gin.H{},
	}

	c.JSON(200, gin.H{
		"code": 20000,
		"data": detail,
	})
}

// DELETE /api/maintenance/workers/:id - Soft delete worker
func (h *MaintenanceWorkerHandler) DeleteWorker(c *gin.Context) {
	workerID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Soft delete
	if err := db.Model(&models.MaintenanceWorker{}).
		Where("id = ? AND tenant_id = ?", workerID, tenantID).
		Update("deleted_at", time.Now()).Error; err != nil {
		c.JSON(500, gin.H{"code": 50000, "message": "failed to delete worker: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"code":    20000,
		"message": "success",
		"data":    gin.H{"id": workerID, "deleted": true},
	})
}
