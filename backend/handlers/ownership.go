package handlers

import (
	"net/http"
	"tuneloop-backend/internal/engine"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/models"
	"tuneloop-backend/database"

	"github.com/gin-gonic/gin"
)

var ownershipEngine = engine.NewOwnershipEngine()

func GetOwnershipInfo(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order id is required",
		})
		return
	}

	info, err := ownershipEngine.GetOwnershipInfo(orderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "ownership not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": info,
	})
}

func DownloadOwnershipCertificate(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order id is required",
		})
		return
	}

	db := database.GetDB()
	
	var cert models.OwnershipCertificate
	if err := db.First(&cert, "order_id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "certificate not found",
		})
		return
	}

	var instrument models.Instrument
	db.First(&instrument, "id = ?", cert.InstrumentID)
	
	var user models.User
	db.First(&user, "id = ?", cert.UserID)

	certData := &service.CertificateData{
		CertificateID: cert.ID,
		InstrumentName: instrument.Name,
		InstrumentSN:  "SN-" + instrument.ID[:8],
		OwnerName:     user.Name,
		OwnerPhone:    user.Phone,
		TransferDate:  cert.TransferDate,
	}

	pdfBytes, err := service.GenerateOwnershipCertificate(certData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to generate PDF",
		})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename=ownership_certificate_"+orderID+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func TriggerOwnershipTransfer(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order id is required",
		})
		return
	}

	err := ownershipEngine.CheckAndTransfer(orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "transfer failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"transfer_completed": true,
			"transferred_at":     "2026-03-21T00:00:00Z",
		},
	})
}
