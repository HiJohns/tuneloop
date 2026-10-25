package handlers

// #2278 返工：无 rent 支付单时不得产生悬挂 pending；重算幂等（order_id 锚）。

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortfallExcess_NoRentPayment_2278(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "returned",
		CashPaid: models.FromYuan(3500)}).Error)
	require.NoError(t, db.Create(&models.Settlement{OrderID: orderID, ActualRentAmount: models.FromYuan(3000), RefundStatus: "pending"}).Error)

	require.NoError(t, applyShortfallSettlement(db, orderID))
	require.NoError(t, applyShortfallSettlement(db, orderID))

	var refunds []models.OrderRefundRecord
	require.NoError(t, db.Where("reason = ?", "补缴超收退款").Find(&refunds).Error)
	require.Len(t, refunds, 1, "无 rent 支付单时也必须幂等（仅 1 条）")
	assert.Equal(t, "refunded", refunds[0].Status, "无支付单→无需微信退款，直接 refunded")
	require.NotNil(t, refunds[0].OrderID)
	assert.Equal(t, orderID, *refunds[0].OrderID)

	var st models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&st).Error)
	assert.Equal(t, "completed", st.RefundStatus, "无支付单→结算 completed（不得 pending/refunded 悬挂）")
}
