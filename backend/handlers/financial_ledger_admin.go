package handlers

// #2281 P3 步骤4：运维只读重算入口（账本 vs 聚合）。

import (
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func AdminOrderFinancials(c *gin.Context) {
	orderID := c.Param("id")
	ctx := c.Request.Context()
	// 平台运维入口：显式按 id 取单（不做租户过滤），鉴权由路由 SysPerm 承担
	db := database.GetDB().WithContext(database.IdentityCtx(ctx))
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}
	paid, refunded, net, err := RecomputeOrderFinancials(db, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "recompute failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"order_id":      order.ID,
		"cash_paid":     order.CashPaid,
		"ledger_paid":   paid,
		"ledger_refund": refunded,
		"ledger_net":    net,
		"diff":          order.CashPaid - net,
	}})
}
