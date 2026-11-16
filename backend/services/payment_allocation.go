package services

// #2360（#2268 子任务）：租金/押金现金实收的权威读取。
//
// v2.2 §3.1：`rent_cash_paid_net_cents` 只能来自支付分配账本
// （order_payment_allocations），禁止用 `cash_paid_net - deposit_held` 反推——
// 押金被扣用后反推值会把「押金已用」误算成「租金少收」，导致账户串账。
//
// 历史单（账本上线前）没有分配行，只允许两种显式回退（D1）：
//   - legacy_derived_rent_cash：无押金扣用 → 允许反推，且逐单写审计行；
//   - legacy_policy_unresolved：检测到押金扣用（或无法确认）→ 不返回反推值，
//     只标记，等人工/迁移补齐账本。
//
// 本文件只读，不修改任何金额（R5 negative：静默改写金额 → FAIL）。

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

const (
	CashSourceLedger                 = "ledger"
	CashSourceLegacyDerivedRent      = "legacy_derived_rent_cash"
	CashSourceLegacyPolicyUnresolved = "legacy_policy_unresolved"

	// settlementLedgerAuditOperator 写入 order_logs 的审计操作者标识。
	settlementLedgerAuditOperator = "system:settlement-ledger-read"
)

// CashAccounts 是结算读取的租金/押金账户视图。
//
// LegacyMarker 非空表示该单走了历史回退（Source 亦为对应标记值）；
// AuditLogID 为本次回退写入的 order_logs 审计行（逐单可追溯）。
type CashAccounts struct {
	OrderID                      string       `json:"order_id"`
	RentCashPaidNetCents         models.Cents `json:"rent_cash_paid_net_cents"`
	DepositCashCollectedNetCents models.Cents `json:"deposit_cash_collected_net_cents"`
	DepositHeldCents             models.Cents `json:"deposit_held_cents"`
	Source                       string       `json:"source"`
	LegacyMarker                 string       `json:"legacy_marker,omitempty"`
	AuditLogID                   string       `json:"audit_log_id,omitempty"`
	AllocationRows               int64        `json:"allocation_rows"`
}

// ReadOrderCashAccounts 读取订单的租金/押金现金账户。
//
// 账本优先：只要存在分配行，租金实收 = SUM(rent_cents)、押金净收 =
// SUM(deposit_cents)、持有押金 = SUM(deposit_cents) - SUM(deposit_deducted_cents)。
// 押金扣用不会改变 rent_cents（T26 不变量）。
func ReadOrderCashAccounts(db *gorm.DB, orderID string) (*CashAccounts, error) {
	if db == nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: nil db")
	}
	if orderID == "" {
		return nil, fmt.Errorf("ReadOrderCashAccounts: empty order id")
	}

	var allocations []models.OrderPaymentAllocation
	if err := db.Where("order_id = ?", orderID).Find(&allocations).Error; err != nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: load allocations: %w", err)
	}

	if len(allocations) > 0 {
		out := &CashAccounts{OrderID: orderID, Source: CashSourceLedger, AllocationRows: int64(len(allocations))}
		for _, a := range allocations {
			out.RentCashPaidNetCents += a.RentCents
			out.DepositCashCollectedNetCents += a.DepositCents
			out.DepositHeldCents += a.DepositCents - a.DepositDeductedCents
		}
		return out, nil
	}

	return readLegacyCashAccounts(db, orderID)
}

// readLegacyCashAccounts 处理账本上线前的历史单（无分配行）。
func readLegacyCashAccounts(db *gorm.DB, orderID string) (*CashAccounts, error) {
	// 押金扣用检测：逾期扣款从押金扣除 → 反推公式必然失真（spec §3.1.1）。
	var depositDeductions int64
	if err := db.Model(&models.OverdueCharge{}).
		Where("order_id = ? AND deducted_from_deposit > 0", orderID).
		Count(&depositDeductions).Error; err != nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: check deposit deductions: %w", err)
	}

	if depositDeductions > 0 {
		out := &CashAccounts{
			OrderID:      orderID,
			Source:       CashSourceLegacyPolicyUnresolved,
			LegacyMarker: CashSourceLegacyPolicyUnresolved,
		}
		out.AuditLogID = writeLegacyAudit(db, orderID, CashSourceLegacyPolicyUnresolved,
			fmt.Sprintf("deposit deductions detected (%d overdue charge rows); derived rent is forbidden", depositDeductions))
		return out, nil
	}

	// 聚合结果按 int64 扫描：models.Cents.Scan 对 numeric/文本会按「元」换算，
	// 直接扫进 Cents 会把分再乘 100（#2360 R5 实测）。先取原始分，再显式转换。
	var paidNetRaw int64
	if err := db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND type = ? AND status = ?", orderID, "payment", "paid").
		Select("COALESCE(SUM(amount), 0)").Scan(&paidNetRaw).Error; err != nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: sum payments: %w", err)
	}
	var refundedRaw int64
	if err := db.Model(&models.OrderRefundRecord{}).
		Where("order_id = ? AND status = ?", orderID, "refunded").
		Select("COALESCE(SUM(amount), 0)").Scan(&refundedRaw).Error; err != nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: sum refunds: %w", err)
	}
	var depositRaw int64
	if err := db.Model(&models.Order{}).
		Where("id = ?", orderID).
		Select("deposit").Scan(&depositRaw).Error; err != nil {
		return nil, fmt.Errorf("ReadOrderCashAccounts: load order deposit: %w", err)
	}

	paidNet := models.Cents(paidNetRaw)
	refunded := models.Cents(refundedRaw)
	deposit := models.Cents(depositRaw)
	derivedRent := paidNet - refunded - deposit
	out := &CashAccounts{
		OrderID:                      orderID,
		RentCashPaidNetCents:         derivedRent,
		DepositCashCollectedNetCents: paidNet - refunded, // 历史单无分账户，仅用于展示
		DepositHeldCents:             deposit,
		Source:                       CashSourceLegacyDerivedRent,
		LegacyMarker:                 CashSourceLegacyDerivedRent,
	}
	out.AuditLogID = writeLegacyAudit(db, orderID, CashSourceLegacyDerivedRent,
		fmt.Sprintf("legacy fallback: paid_net=%d deposit=%d derived_rent=%d (no deposit deductions, no allocation rows)",
			paidNet-refunded, deposit, derivedRent))
	return out, nil
}

// writeLegacyAudit 写逐单审计行（order_logs）；失败只记录日志不阻塞读取，
// 但返回值让调用方知道有没有落审计（R5 断言审计行存在）。
func writeLegacyAudit(db *gorm.DB, orderID, event, note string) string {
	operator := settlementLedgerAuditOperator
	logRow := models.OrderLog{
		OrderID:      orderID,
		Event:        event,
		OperatorName: &operator,
	}
	if err := db.Create(&logRow).Error; err != nil {
		log.Printf("[PaymentAllocation] legacy audit insert failed for order %s (%s): %v", orderID, note, err)
		return ""
	}
	log.Printf("[PaymentAllocation] legacy audit %s for order %s: %s", event, orderID, note)
	return logRow.ID
}
