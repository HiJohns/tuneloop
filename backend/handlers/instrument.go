package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

var pricingService = service.NewPricingService()

type CreateInstrumentRequest struct {
	Name           string                 `json:"name" binding:"required"`
	Brand          string                 `json:"brand"`
	Level          string                 `json:"level" binding:"required"`
	Pricing        map[string]interface{} `json:"pricing"`
	Description    string                 `json:"description"`
	Images         []string               `json:"images"`
	Video          string                 `json:"video"`
	Specifications map[string]interface{} `json:"specifications"`
}

func GetInstrumentPricing(c *gin.Context) {
	instrumentID := c.Param("id")

	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	pricing := pricingService.GetInstrumentPricing(instrumentID)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": pricing,
	})
}

func CreateInstrument(c *gin.Context) {
	db := database.GetDB()
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40100,
			"message": "missing tenant_id in context",
		})
		return
	}

	var req CreateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	instrument := models.Instrument{
		TenantID:    tenantID,
		Name:        req.Name,
		Brand:       req.Brand,
		Level:       req.Level,
		Description: req.Description,
		StockStatus: "available",
	}

	if req.Images != nil {
		instrument.Images = "{\"images\": [\"" + "\"]}"
	}

	if req.Specifications != nil {
		instrument.Specifications = "{}"
	}

	if req.Pricing != nil {
		instrument.Pricing = "{}"
	}

	if err := db.Create(&instrument).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create instrument",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id":    instrument.ID,
			"name":  instrument.Name,
			"brand": instrument.Brand,
			"level": instrument.Level,
		},
	})
}
