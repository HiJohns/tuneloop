package services

// #2360（#2268 子任务）：支付分配账本的权威读取。
//
// R3：押金截留/扣用后，租金现金净实收必须仍等于账本 rent_cents 之和，
//     不得等于「cash_paid_net - deposit_held」反推值（T26 不变量）。
// R5：历史单（无分配行）只允许显式回退：无押金扣用 → legacy_derived_rent_cash
//     并写审计行；有押金扣用 → legacy_policy_unresolved 且不返回反推值。

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func setupAllocation2360DB(t *testing.T) *gorm.DB {
	t.Helper()
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	require.NoError(t, db.AutoMigrate(
		&models.Order{},
		&models.OrderPaymentRecord{},
		&models.OrderRefundRecord{},
		&models.OverdueCharge{},
		&models.OrderLog{},
		&models.OrderPaymentAllocation{},
	))
	return db
}

func makeOrder2360(t *testing.T, db *gorm.DB, deposit models.Cents) (orderID, tenantID, userID string) {
	t.Helper()
	orderID = uuid.New().String()
	tenantID = uuid.New().String()
	userID = uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID: uuid.New().String(), Level: "entry", LeaseTerm: 30,
		MonthlyRent: models.FromYuan(1000), Deposit: deposit,
		Status: "returned", CreatedAt: time.Now(),
	}).Error)
	return orderID, tenantID, userID
}

func addPayment2360(t *testing.T, db *gorm.DB, orderID, tenantID, userID string, amount models.Cents) *models.OrderPaymentRecord {
	t.Helper()
	rec := &models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, UserID: userID, OrderID: &orderID,
		OrderType: "rent", Amount: amount, Type: "payment", Status: "paid",
	}
	require.NoError(t, db.Create(rec).Error)
	return rec
}

func addAllocation2360(t *testing.T, db *gorm.DB, orderID, tenantID, sourceType, sourceID string, rent, deposit, depositedDeducted models.Cents) {
	t.Helper()
	require.NoError(t, db.Create(&models.OrderPaymentAllocation{
		OrderID: orderID, TenantID: tenantID, SourceType: sourceType, SourceID: sourceID,
		RentCents: rent, DepositCents: deposit, DepositDeductedCents: depositedDeducted,
	}).Error)
}

// R3：账本读取 vs 反推公式——押金扣用后结果不同，服务只返回账本值。
func TestPaymentAllocationRead_2360(t *testing.T) {
	db := setupAllocation2360DB(t)
	orderID, tenantID, userID := makeOrder2360(t, db, models.FromYuan(1000))

	pay := addPayment2360(t, db, orderID, tenantID, userID, models.FromYuan(2000))
	// 支付 2000 = 租金 1000 + 押金 1000
	addAllocation2360(t, db, orderID, tenantID, "payment", pay.ID, models.FromYuan(1000), models.FromYuan(1000), 0)
	// 定损从押金扣 300（押金扣用行；不得改写租金）
	addAllocation2360(t, db, orderID, tenantID, "deduction", uuid.New().String(), 0, 0, models.FromYuan(300))

	got, err := ReadOrderCashAccounts(db, orderID)
	require.NoError(t, err)
	assert.Equal(t, CashSourceLedger, got.Source, "有分配行时必须走账本")
	assert.Equal(t, models.FromYuan(1000), got.RentCashPaidNetCents, "租金实收不受押金扣用影响")
	assert.Equal(t, models.FromYuan(1000), got.DepositCashCollectedNetCents)
	assert.Equal(t, models.FromYuan(700), got.DepositHeldCents, "持有押金 = 收取 - 扣用")
	assert.EqualValues(t, 2, got.AllocationRows)

	// 反推公式：cash_paid_net(2000) - deposit_held(700) = 1300 ≠ 账本 1000
	naive := models.FromYuan(2000) - got.DepositHeldCents
	assert.NotEqual(t, got.RentCashPaidNetCents, naive, "反推值必须与账本值不同（证明服务没有反推）")
	assert.Empty(t, got.LegacyMarker, "账本路径不得带 legacy 标记")
}

// R5：历史单（无分配行）。
func TestLegacyRentCashFallback_2360(t *testing.T) {
	db := setupAllocation2360DB(t)

	// 场景 A：租金 1000 + 押金 0，无分配行、无押金扣用 → legacy_derived_rent_cash + 审计行
	orderA, tenantA, userA := makeOrder2360(t, db, 0)
	payA := addPayment2360(t, db, orderA, tenantA, userA, models.FromYuan(1000))

	gotA, err := ReadOrderCashAccounts(db, orderA)
	require.NoError(t, err)
	assert.Equal(t, CashSourceLegacyDerivedRent, gotA.Source)
	assert.Equal(t, CashSourceLegacyDerivedRent, gotA.LegacyMarker)
	assert.Equal(t, models.FromYuan(1000), gotA.RentCashPaidNetCents, "无押金时反推 = 实收租金")
	assert.NotEmpty(t, gotA.AuditLogID, "回退必须写审计行")

	var auditA models.OrderLog
	require.NoError(t, db.Where("id = ?", gotA.AuditLogID).First(&auditA).Error)
	assert.Equal(t, CashSourceLegacyDerivedRent, auditA.Event)
	assert.Equal(t, orderA, auditA.OrderID)

	// 金额不被改写：支付记录与订单字段保持原值
	var payAfter models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", payA.ID).First(&payAfter).Error)
	assert.Equal(t, models.FromYuan(1000), payAfter.Amount, "读取不得改写支付金额")
	var orderAfter models.Order
	require.NoError(t, db.Where("id = ?", orderA).First(&orderAfter).Error)
	assert.EqualValues(t, 0, orderAfter.Deposit, "读取不得改写订单押金")

	// 场景 B：租金 1000 + 押金 1000，已有押金扣用 300 → legacy_policy_unresolved，禁止反推
	orderB, tenantB, userB := makeOrder2360(t, db, models.FromYuan(1000))
	payB := addPayment2360(t, db, orderB, tenantB, userB, models.FromYuan(2000))
	require.NoError(t, db.Create(&models.OverdueCharge{
		ID: uuid.New().String(), OrderID: orderB, ChargeDate: "2026-11-16",
		Amount: models.FromYuan(300), DeductedFromDeposit: models.FromYuan(300), Status: "success",
	}).Error)

	gotB, err := ReadOrderCashAccounts(db, orderB)
	require.NoError(t, err)
	assert.Equal(t, CashSourceLegacyPolicyUnresolved, gotB.Source)
	assert.Equal(t, CashSourceLegacyPolicyUnresolved, gotB.LegacyMarker)
	assert.Zero(t, gotB.RentCashPaidNetCents, "有押金扣用时不得返回反推值")
	assert.NotEmpty(t, gotB.AuditLogID, "不可解析回退也必须留审计行")

	var auditB models.OrderLog
	require.NoError(t, db.Where("id = ?", gotB.AuditLogID).First(&auditB).Error)
	assert.Equal(t, CashSourceLegacyPolicyUnresolved, auditB.Event)

	// 金额不被改写
	var payBAfter models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", payB.ID).First(&payBAfter).Error)
	assert.Equal(t, models.FromYuan(2000), payBAfter.Amount)
}
