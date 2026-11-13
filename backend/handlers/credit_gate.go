package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// #2327 S3b: 支付分档位闸门的 handler 层封装。
//
// 纯判定在 services/payscore_gate.go（无 DB），这里只负责「按订单号取档位/信用状态」，
// 供 PayOrder / PrepayOrder / CalculatePayment 三个支付入口共用，避免各写一套。
// PAYSCORE_ENFORCE=false（默认）时一律放行，保持线上现有行为（D12）。

// creditGateError 返回该订单在支付前必须满足的信用条件（nil = 放行）。
func creditGateError(db *gorm.DB, orderID string) error {
	if orderID == "" || !wechatpay.PayscoreEnforceEnabled() {
		return nil
	}
	var order models.Order
	if err := db.Select("deposit_tier", "deposit_credit_path", "credit_state").
		Where("id = ?", orderID).First(&order).Error; err != nil {
		// 订单不存在由调用方自己的校验负责（此处不越权改变 404 语义）。
		return nil
	}
	return services.AssertOrderPayableIfEnforced(
		order.DepositTier, order.DepositCreditPath, order.CreditState, true)
}

// failCreditGate 是支付入口共用的错误响应（403 / 40320）。
func failCreditGate(c *gin.Context, err error) {
	c.JSON(http.StatusForbidden, gin.H{"code": 40320, "message": err.Error()})
}
