package handlers

import (
	"context"
	"fmt"
	"log"
	"sync"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// #2327 S3b / D14: 结算时的支付分收口。
//
// 现金押金与支付分担保是两条独立通道：
//   - 现金：只退实付押金（computeSettlement → initiateCashRefund）；
//   - 支付分：只做 CompleteOrder / CancelOrder，绝不发起现金退款。
//
// provider 由 main 注入（开发/未获商务准入时是 mock，零网络请求）。
var (
	settlementPayscoreMu        sync.RWMutex
	settlementPayscoreProvider  wechatpay.PayscoreProvider
	settlementPayscoreServiceID string
)

// SetPayscoreSettlementProvider 注入结算侧使用的支付分 provider。
func SetPayscoreSettlementProvider(p wechatpay.PayscoreProvider, serviceID string) {
	settlementPayscoreMu.Lock()
	defer settlementPayscoreMu.Unlock()
	settlementPayscoreProvider = p
	settlementPayscoreServiceID = serviceID
}

func currentPayscoreProvider() (wechatpay.PayscoreProvider, string) {
	settlementPayscoreMu.RLock()
	defer settlementPayscoreMu.RUnlock()
	return settlementPayscoreProvider, settlementPayscoreServiceID
}

// settlePayscoreGuarantee 按结算动作收口支付分订单（幂等、失败只记日志）。
//
// 注意：本函数不得触发任何现金退款——支付分担保不是现金押金。
func settlePayscoreGuarantee(ctx context.Context, order models.Order, action string) error {
	if action == services.PayscoreActionNone || order.CreditServiceOrderNo == "" {
		return nil
	}
	provider, serviceID := currentPayscoreProvider()
	if provider == nil {
		return nil
	}
	var err error
	switch action {
	case services.PayscoreActionComplete:
		err = provider.CompleteOrder(ctx, serviceID, order.CreditServiceOrderNo)
	case services.PayscoreActionCancel:
		err = provider.CancelOrder(ctx, serviceID, order.CreditServiceOrderNo)
	default:
		return fmt.Errorf("payscore settlement: unknown action %q", action)
	}
	if err != nil {
		return fmt.Errorf("payscore settlement (%s) for order %s: %w", action, order.ID, err)
	}
	log.Printf("[payscore] order %s settled action=%s service_order=%s", order.ID, action, order.CreditServiceOrderNo)
	return nil
}

// payscoreActionForOrder 根据订单终态推导收口动作（重试路径用）。
// 已取消 → cancel；其余结算场景（归还/完成/定损）→ complete。
func payscoreActionForOrder(order models.Order) string {
	if !services.RequiresCreditApprovalForOrder(order.DepositTier, order.DepositCreditPath) {
		return services.PayscoreActionNone
	}
	if order.CreditState == "" || order.CreditServiceOrderNo == "" {
		return services.PayscoreActionNone
	}
	if order.Status == models.OrderStatusCancelled {
		return services.PayscoreActionCancel
	}
	return services.PayscoreActionComplete
}
