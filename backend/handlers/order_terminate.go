package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func TerminateOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var order models.Order
	if err := db.First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	tx := db.Begin()

	order.Status = models.OrderStatusInStore
	if err := tx.WithContext(c.Request.Context()).Save(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to terminate order",
		})
		return
	}

	var instrument models.Instrument
	if err := tx.WithContext(c.Request.Context()).First(&instrument, "id = ?", order.InstrumentID).Error; err == nil {
		instrument.StockStatus = "available"
		if err := tx.WithContext(c.Request.Context()).Save(&instrument).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to release inventory",
			})
			return
		}
	}

	if order.DepositMode == "standard" && order.Deposit > 0 {
		order.DepositRefunded = true
	}
	if err := tx.WithContext(c.Request.Context()).Save(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to persist deposit refund"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":           order.ID,
			"status":             models.OrderStatusInStore,
			"deposit_refunded":   order.DepositRefunded,
			"inventory_released": true,
		},
	})
}
