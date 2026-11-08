package handlers

// #2281 P3 步骤2：computeSettlement 读路径以账本净额为上限。

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeSettlement_UsesLedgerNet_2281(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	order := buildMatrixOrder(t, db, matrixCase{baseDaily: 100, days: 30, deposit: 500, discount: 1})

	var pr models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND type = ? AND status = ?", order.ID, "payment", "paid").First(&pr).Error)
	require.NoError(t, db.Create(&models.OrderRefundRecord{
		ID: uuidNew(), TenantID: order.TenantID, PaymentRecordID: &pr.ID,
		OutRefundNo: strPtr("ledger-read-2281"), Amount: models.FromYuan(500), Status: "refunded",
	}).Error)

	// 人为把聚合调高（模拟历史错锚），读路径必须收敛到账本净额
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).Update("cash_paid", models.FromYuan(9999)).Error)
	var loaded models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&loaded).Error)

	net, ok := orderLedgerCash(db, order.ID)
	require.True(t, ok)
	clone := loaded
	clone.CashPaid = net
	want := computeSettlement(clone, db)
	got := computeSettlement(loaded, db)
	assert.Equal(t, want.CashRefundable, got.CashRefundable, "读路径必须与账本净额一致")
	assert.Less(t, got.CashRefundable, 9999.0, "不得使用虚高聚合")
}

func uuidNew() string { return uuid.NewString() }
