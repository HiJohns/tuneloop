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

// OrderLedger 是单一账本快照（#2281 P3）：支付/退款流水为唯一事实源。
type OrderLedger struct {
	Paid      models.Cents
	Refunded  models.Cents
	Net       models.Cents
	HasLedger bool // 至少一条 paid 流水
}

// LoadOrderLedger 读取账本；无 paid 流水（旧单）时 HasLedger=false。
func LoadOrderLedger(db *gorm.DB, orderID string) (OrderLedger, error) {
	paid, refunded, net, err := RecomputeOrderFinancials(db, orderID)
	if err != nil {
		return OrderLedger{}, err
	}
	return OrderLedger{Paid: paid, Refunded: refunded, Net: net, HasLedger: paid > 0}, nil
}

// orderLedgerCash 返回账本净额（仅当存在 paid 流水时有效）。
func orderLedgerCash(db *gorm.DB, orderID string) (models.Cents, bool) {
	l, err := LoadOrderLedger(db, orderID)
	if err != nil || !l.HasLedger {
		return 0, false
	}
	return l.Net, true
}

// effectiveCashPaid 供读路径统一取值：有账本 → 净额；旧单（无流水）→ 缓存值。
func effectiveCashPaid(db *gorm.DB, order models.Order) models.Cents {
	if l, err := LoadOrderLedger(db, order.ID); err == nil && l.HasLedger {
		return l.Net
	}
	return order.CashPaid
}

// #2281 P3 步骤3：聚合 cash_paid 的唯一写入口（回调/续租/退款共用）。
func setOrderCashPaid(tx *gorm.DB, orderID string, amount models.Cents) error {
	return tx.Model(&models.Order{}).Where("id = ?", orderID).Update("cash_paid", amount).Error
}

func addOrderCashPaid(tx *gorm.DB, orderID string, delta models.Cents) error {
	return tx.Model(&models.Order{}).Where("id = ?", orderID).
		Update("cash_paid", gorm.Expr("GREATEST(cash_paid + ?, 0)", delta)).Error
}
