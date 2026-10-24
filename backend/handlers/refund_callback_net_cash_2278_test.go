package handlers

// #2278：退款回调必须按净额回写 cash_paid，且重复回调幂等。

import (
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundCallback_NetCashPaid_2268(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	org := uuid.NewString()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: userID, IAMSub: userID, TenantID: org, OrgID: org, Status: "active"}).Error)
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: userID,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
		CashPaid: models.FromYuan(3500),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Settlement{OrderID: order.ID, ActualRentAmount: models.FromYuan(3000), RefundStatus: "pending"}).Error)
	pay := models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: userID, OrderID: &order.ID,
		OrderType: "rent", Amount: models.FromYuan(3500), Type: "payment", Status: "paid",
	}
	require.NoError(t, db.Create(&pay).Error)
	refund := models.OrderRefundRecord{
		ID: uuid.NewString(), TenantID: org, PaymentRecordID: &pay.ID,
		OutRefundNo: strPtr("refund2278"), Amount: models.FromYuan(500), Status: "pending",
	}
	require.NoError(t, db.Create(&refund).Error)

	cb := func() bool {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/wechatpay/refund-notify", nil)
		return processRefundCallback(c, &wechatpay.CallbackResult{OutRefundNo: "refund2278", RefundID: "wx-refund-1"})
	}

	require.True(t, cb())
	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3000), after.CashPaid, "退款后 cash_paid 必须按净额减少")

	// 幂等重放：不得再次扣减
	require.True(t, cb())
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3000), after.CashPaid, "重复回调不得重复扣减")
}
