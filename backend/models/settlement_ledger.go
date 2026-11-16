package models

// #2360（#2268 子任务）：结算数据模型与支付分配账本。
//
// v2.2 §3.1：租金/押金现金实收只能从支付分配账本读取，禁止用
// `cash_paid_net - deposit_held` 反推（押金被扣用后该式失真）。
//
// 本文件的模型由迁移 20261116001_settlement_ledger_2360 创建/扩展；
// 新增列/表必须同步登记 backend/database/db.go 的 defaultMigrationArtifacts
// 与 modelsToValidate，否则预生产会因「版本已记录但工件缺失」FATAL。

import "time"

// OrderPaymentAllocation 支付分配账本：每笔支付/退款/押金扣用按账户拆一行。
//
// 约定（R3/T26 不变量）：
//   - rent_cents：租金现金流入（退款为负），押金扣用不得改写此列；
//   - deposit_cents：押金流入（退款为负）；
//   - deposit_deducted_cents：押金被扣用金额（正数），仅减少「持有押金」，
//     不影响租金实收；
//   - fee_cents / fee_type：逾期罚金 / 定损费 / 物流费 等费用分摊。
//
// 幂等键 = (order_id, source_type, source_id)：支付/退款/扣用各写一次。
type OrderPaymentAllocation struct {
	ID         string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID    string `gorm:"type:uuid;not null;index:idx_opa_order" json:"order_id"`
	TenantID   string `gorm:"type:uuid;index" json:"tenant_id"`
	SourceType string `gorm:"type:varchar(24);not null;uniqueIndex:idx_opa_source" json:"source_type"` // payment | refund | deduction | adjustment
	SourceID   string `gorm:"type:varchar(64);not null;uniqueIndex:idx_opa_source" json:"source_id"`

	Channel string `gorm:"type:varchar(20);not null;default:''" json:"channel"`

	RentCents            Cents  `gorm:"type:bigint;not null;default:0" json:"rent_cents"`
	DepositCents         Cents  `gorm:"type:bigint;not null;default:0" json:"deposit_cents"`
	FeeCents             Cents  `gorm:"type:bigint;not null;default:0" json:"fee_cents"`
	FeeType              string `gorm:"type:varchar(24);not null;default:''" json:"fee_type"` // overdue_penalty | damage_fee | logistics_fee | ''
	DepositDeductedCents Cents  `gorm:"type:bigint;not null;default:0" json:"deposit_deducted_cents"`

	CreatedAt time.Time `json:"created_at"`
}

func (OrderPaymentAllocation) TableName() string { return "order_payment_allocations" }

// SettlementActionLedger 结算子动作账本（退款/费用扣收/乐币恢复等）。
//
// 幂等键在**执行前**预生成（settlement_id + action_type + source_transaction_id），
// 不依赖渠道返回的 channel_txn_id；部分成功按子动作重试，禁止整单重放。
type SettlementActionLedger struct {
	ID                  string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SettlementID        string    `gorm:"type:uuid;not null;index:idx_sal_settlement" json:"settlement_id"`
	OrderID             string    `gorm:"type:uuid;not null;index:idx_sal_order" json:"order_id"`
	SettlementVersion   int64     `gorm:"not null;default:1" json:"settlement_version"`
	ActionType          string    `gorm:"type:varchar(32);not null" json:"action_type"` // rent_refund | deposit_refund | fee_charge | points_restore | reverse
	IdempotencyKey      string    `gorm:"type:varchar(120);not null;uniqueIndex:idx_sal_idempotency" json:"idempotency_key"`
	PaymentChannel      string    `gorm:"type:varchar(20);not null;default:''" json:"payment_channel"`
	ChannelTxnID        *string   `gorm:"type:varchar(64)" json:"channel_txn_id,omitempty"`
	OriginalTxnID       *string   `gorm:"type:varchar(64)" json:"original_txn_id,omitempty"`
	ExecutedAmountCents Cents     `gorm:"type:bigint;not null;default:0" json:"executed_amount_cents"`
	ExecutedPoints      int64     `gorm:"not null;default:0" json:"executed_points"`
	RetryCount          int       `gorm:"not null;default:0" json:"retry_count"`
	Status              string    `gorm:"type:varchar(24);not null;default:'pending';index" json:"status"`
	ErrorMessage        *string   `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (SettlementActionLedger) TableName() string { return "settlement_action_ledger" }
