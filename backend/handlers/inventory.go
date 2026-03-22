package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryTransfer struct {
	ID          string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AssetID     string     `gorm:"type:uuid;index;not null" json:"asset_id"`
	FromSiteID  string     `gorm:"type:uuid;not null" json:"from_site_id"`
	ToSiteID    string     `gorm:"type:uuid;not null" json:"to_site_id"`
	Reason      string     `gorm:"type:text" json:"reason"`
	Status      string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedBy   string     `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type InventoryHandler struct{}

func NewInventoryHandler() *InventoryHandler {
	return &InventoryHandler{}
}

// GET /api/merchant/inventory - List inventory by site
func (h *InventoryHandler) ListInventory(c *gin.Context) {
	siteID := c.Query("site_id")
	category := c.Query("category")
	status := c.Query("status")

	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "site_id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	query := db.Model(&models.Instrument{}).Where("current_site_id = ?", siteID)

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if status != "" {
		query = query.Where("stock_status = ?", status)
	}

	var instruments []models.Instrument
	var total int64

	query.Count(&total)
	query.Find(&instruments)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  instruments,
			"total": total,
		},
	})
}

// POST /api/merchant/inventory/transfer - Transfer asset between sites
func (h *InventoryHandler) TransferInventory(c *gin.Context) {
	var req struct {
		AssetID    string `json:"asset_id" binding:"required"`
		FromSiteID string `json:"from_site_id" binding:"required"`
		ToSiteID   string `json:"to_site_id" binding:"required"`
		Reason     string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Validate asset exists and is at from_site
	var asset models.Instrument
	if err := db.First(&asset, "id = ?", req.AssetID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "asset not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to validate asset: " + err.Error(),
		})
		return
	}

	// Validate sites exist
	var fromSite, toSite models.Site
	if err := db.First(&fromSite, "id = ?", req.FromSiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "from_site not found",
		})
		return
	}
	if err := db.First(&toSite, "id = ?", req.ToSiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "to_site not found",
		})
		return
	}

	// Create transfer record
	transfer := InventoryTransfer{
		AssetID:    req.AssetID,
		FromSiteID: req.FromSiteID,
		ToSiteID:   req.ToSiteID,
		Reason:     req.Reason,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	tx := db.Begin()
	if err := tx.WithContext(c.Request.Context()).Create(&transfer).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create transfer: " + err.Error(),
		})
		return
	}

	// Update asset current_site_id
	if err := tx.WithContext(c.Request.Context()).Model(&models.Instrument{}).Where("id = ?", req.AssetID).Update("current_site_id", req.ToSiteID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update asset location: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": gin.H{
			"transfer_id": transfer.ID,
			"asset_id":    transfer.AssetID,
			"status":      transfer.Status,
			"created_at":  transfer.CreatedAt,
		},
	})
}

// GET /api/merchant/inventory/transfers - List transfer records
func (h *InventoryHandler) ListTransfers(c *gin.Context) {
	siteID := c.Query("site_id")
	status := c.Query("status")

	db := database.GetDB().WithContext(c.Request.Context())

	var transfers []InventoryTransfer
	query := db.Model(&InventoryTransfer{})

	if siteID != "" {
		query = query.Where("from_site_id = ? OR to_site_id = ?", siteID, siteID)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Order("created_at DESC").Find(&transfers)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"list": transfers},
	})
}
