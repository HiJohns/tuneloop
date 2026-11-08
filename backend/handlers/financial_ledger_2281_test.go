package handlers

// #2281 P3 步骤1：账本重算与聚合对账。

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecomputeOrderFinancials_2281(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	orderID := uuid.NewString()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "completed",
		CashPaid: models.FromYuan(9999), // 故意错值（聚合缓存）
	}).Error)

	pay1 := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "rent", Amount: models.FromYuan(1000), Type: "payment", Status: "paid"}
	pay2 := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "renewal", Amount: models.FromYuan(500), Type: "payment", Status: "paid"}
	pay3 := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "rent", Amount: models.FromYuan(300), Type: "payment", Status: "closed"}
	require.NoError(t, db.Create(&pay1).Error)
	require.NoError(t, db.Create(&pay2).Error)
	require.NoError(t, db.Create(&pay3).Error)
	require.NoError(t, db.Create(&models.OrderRefundRecord{ID: uuid.NewString(), TenantID: org,
		PaymentRecordID: &pay1.ID, OutRefundNo: strPtr("ledger-refund"), Amount: models.FromYuan(200), Status: "refunded"}).Error)
	require.NoError(t, db.Create(&models.OrderRefundRecord{ID: uuid.NewString(), TenantID: org,
		PaymentRecordID: &pay2.ID, OutRefundNo: strPtr("ledger-pending"), Amount: models.FromYuan(50), Status: "pending"}).Error)

	paid, refunded, net, err := RecomputeOrderFinancials(db, orderID)
	require.NoError(t, err)
	assert.Equal(t, models.FromYuan(1500), paid, "仅 paid 支付流水计入")
	assert.Equal(t, models.FromYuan(200), refunded, "仅 refunded 退款计入")
	assert.Equal(t, models.FromYuan(1300), net)

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	assert.NotEqual(t, order.CashPaid, net, "聚合缓存与账本不一致必须可被检出（后续读路径以账本为准）")
}
