package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListMerchantRepairRequests returns all repair requests for the merchant's sites.
func ListMerchantRepairRequests(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	status := c.Query("status")
	siteID := c.Query("site_id")

	var requests []models.RepairRequest
	query := db.Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}
	query.Order("created_at DESC").Find(&requests)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": requests}})
}

// ListAppeals returns appeals for the current user or admin.
func ListAppeals(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	role := middleware.GetRole(c.Request.Context())

	var appeals []models.Appeal
	query := db.Model(&models.Appeal{})

	if role == "USER" {
		query = query.Where("appellant_id = ?", userID)
	} else {
		tenantID := middleware.GetTenantID(ctx)
		if tenantID != "" {
			query = query.Where("tenant_id = ?", tenantID)
		}
	}

	query.Order("created_at DESC").Find(&appeals)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": appeals}})
}

// CreateAppeal creates a new appeal.
func CreateAppeal(c *gin.Context) {
	var req struct {
		Category    string   `json:"category"`
		ObjectType  string   `json:"object_type"`
		ObjectID    string   `json:"object_id"`
		Description string   `json:"description"`
		Images      []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	imagesJSON, _ := json.Marshal(req.Images)

	appeal := models.Appeal{
		ID:          uuid.New().String(),
		TenantID:    middleware.GetTenantID(ctx),
		AppellantID: userID,
		Category:    req.Category,
		ObjectType:  req.ObjectType,
		ObjectID:    req.ObjectID,
		Description: req.Description,
		Images:      string(imagesJSON),
		Status:      "open",
		CreatedAt:   time.Now(),
	}

	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&appeal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": appeal})
}

// CloseAppeal closes an appeal (admin only) and cascades to close linked repair request.
func CloseAppeal(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var appeal models.Appeal
	if err := db.Where("id = ?", id).First(&appeal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	now := time.Now()
	appeal.Status = "closed"
	appeal.ClosedAt = &now
	db.Save(&appeal)

	// Cascade: close linked repair request
	if appeal.ObjectType == "repair_request" && appeal.ObjectID != "" {
		db.Model(&models.RepairRequest{}).Where("id = ?", appeal.ObjectID).
			Updates(map[string]interface{}{"status": models.RepairReqStatusClosed, "closed_at": now})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "appeal closed"})
}
