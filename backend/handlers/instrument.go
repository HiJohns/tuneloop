package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"tuneloop-backend/internal/service"
)

var pricingService = service.NewPricingService()

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
