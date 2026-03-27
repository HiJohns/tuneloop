package handlers

import (
	"github.com/gin-gonic/gin"
	"log"
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
	orgID := middleware.GetOrgID(ctx)
	userID := middleware.GetUserID(ctx)

	log.Printf("[DEBUG CreateInstrument] tenantID=%s, orgID=%s, userID=%s", tenantID, orgID, userID)

	if tenantID == "" {
		log.Printf("[DEBUG CreateInstrument] WARNING: tenantID is empty, orgID=%s", orgID)
		if orgID != "" {
			tenantID = orgID
			log.Printf("[DEBUG CreateInstrument] Using orgID as tenantID: %s", tenantID)
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing tenant_id in context",
			})
			return
		}
	}

	var req CreateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": err.Error(),
		})
		return
	}

	log.Printf("[DEBUG CreateInstrument] Creating instrument: name=%s, brand=%s, level=%s", req.Name, req.Brand, req.Level)

	instrument := models.Instrument{
		TenantID:    tenantID,
		OrgID:       tenantID,
		Name:        req.Name,
		Brand:       req.Brand,
		Level:       req.Level,
		Description: req.Description,
		StockStatus: "available",
	}

	if req.Images != nil && len(req.Images) > 0 {
		instrument.Images = "[]"
	} else {
		instrument.Images = "[]"
	}

	if req.Specifications != nil {
		instrument.Specifications = "{}"
	} else {
		instrument.Specifications = "{}"
	}

	if req.Pricing != nil {
		instrument.Pricing = "{}"
	} else {
		instrument.Pricing = "{}"
	}

	log.Printf("[DEBUG CreateInstrument] Before DB Create: tenantID=%s", instrument.TenantID)

	if err := db.Create(&instrument).Error; err != nil {
		log.Printf("[ERROR CreateInstrument] DB Error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create instrument: " + err.Error(),
		})
		return
	}

	log.Printf("[DEBUG CreateInstrument] Success: instrument.ID=%s", instrument.ID)

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
