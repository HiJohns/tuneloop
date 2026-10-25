package handlers

// #2281 P3：单一账本重算（只读）。
// 事实源：order_payment_records(type=payment,status=paid) − order_refund_records(status=refunded)。
// orders.cash_paid 视为可重算缓存；后续步骤（读路径切换）将使用本函数返回值。

import (
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// RecomputeOrderFinancials returns (paid, refunded, net) in Cents for one order.
func RecomputeOrderFinancials(db *gorm.DB, orderID string) (paid, refunded, net models.Cents, err error) {
	var paidRaw, refundedRaw int64
	if err := db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND type = ? AND status = ?", orderID, "payment", "paid").
		Select("COALESCE(SUM(amount),0)").Scan(&paidRaw).Error; err != nil {
		return 0, 0, 0, err
	}
	if err := db.Table("order_refund_records AS rr").
		Joins("JOIN order_payment_records AS pr ON pr.id = rr.payment_record_id").
		Where("pr.order_id = ? AND rr.status = ?", orderID, "refunded").
		Select("COALESCE(SUM(rr.amount),0)").Scan(&refundedRaw).Error; err != nil {
		return 0, 0, 0, err
	}
	paid, refunded = models.Cents(paidRaw), models.Cents(refundedRaw)
	return paid, refunded, paid - refunded, nil
}

// orderLedgerCash 返回账本净额（仅当存在 paid 流水时有效），供读路径过渡使用。
// 过渡策略：仅当存在已退款流水（net < paid）时用于收敛虚高聚合。
func orderLedgerCash(db *gorm.DB, orderID string) (models.Cents, bool) {
	paid, refunded, net, err := RecomputeOrderFinancials(db, orderID)
	if err != nil || paid <= 0 || refunded <= 0 {
		return 0, false
	}
	return net, true
}
