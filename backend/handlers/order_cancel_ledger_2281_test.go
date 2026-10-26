package handlers

// #2281 审计修复：员工取消订单退款金额以账本净额为准。

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaffCancelOrder_UsesLedgerNet_2281(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testfixtures.SetupWechatPayMock(t)

	org, user, staff := uuid.NewString(), uuid.NewString(), uuid.NewString()
	instID, orderID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instID, TenantID: org, OrgID: &org, SN: "SC-2281", StockStatus: "rented"}).Error)
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: org, OrgID: org, UserID: user, InstrumentID: instID,
		Level: "entry", LeaseTerm: 30, Status: models.OrderStatusPaid,
		CashPaid: models.FromYuan(80), // 缓存虚高：账本净额应为 60
	}).Error)

	payID := uuid.NewString()
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: payID, TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "rent", Amount: models.FromYuan(100), Type: "payment", Status: "paid",
		OutTradeNo: strPtr("SCPAY-" + uuid.NewString()[:8]), CreatedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.OrderRefundRecord{
		ID: uuid.NewString(), TenantID: org, OrderID: &orderID, PaymentRecordID: &payID,
		OutRefundNo: strPtr("SCREF-" + uuid.NewString()[:8]), Amount: models.FromYuan(40),
		Status: "refunded", CreatedAt: time.Now(),
	}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: org, OrgID: org, UserID: staff, Role: "ADMIN"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/warehouse/orders/:id/staff-cancel", StaffCancelOrder)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/warehouse/orders/"+orderID+"/staff-cancel", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	t.Logf("cancel resp=%s", w.Body.String())
	l, lerr := LoadOrderLedger(db, orderID)
	t.Logf("ledger=%+v err=%v", l, lerr)
	var all []models.OrderRefundRecord
	_ = db.Where("order_id = ?", orderID).Find(&all).Error
	for _, rr := range all {
		t.Logf("refund row id=%s no=%v amount=%d status=%s", rr.ID, rr.OutRefundNo, rr.Amount, rr.Status)
	}

	var refund models.OrderRefundRecord
	require.NoError(t, db.Where("order_id = ? AND out_refund_no LIKE 'scn_%'", orderID).Order("created_at desc").First(&refund).Error)
	assert.Equal(t, models.FromYuan(60), refund.Amount, "退款金额必须 = 账本净额（100−40），不得用缓存 80")
}
