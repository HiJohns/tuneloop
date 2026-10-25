package handlers

// #2281 P3 步骤4：运维只读重算端点。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminOrderFinancials_2281(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	orderID := uuid.NewString()
	require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: uuid.NewString(),
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "completed", CashPaid: models.FromYuan(1200)}).Error)
	pr := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: uuid.NewString(), OrderID: &orderID,
		OrderType: "rent", Amount: models.FromYuan(1500), Type: "payment", Status: "paid"}
	require.NoError(t, db.Create(&pr).Error)
	require.NoError(t, db.Create(&models.OrderRefundRecord{ID: uuid.NewString(), TenantID: org, OrderID: &orderID,
		PaymentRecordID: &pr.ID, OutRefundNo: strPtr("admin-ledger"), Amount: models.FromYuan(300), Status: "refunded"}).Error)

	actor := testutil.TestActor{TenantID: "", OrgID: "", UserID: uuid.NewString(), Role: "NAMESPACE_ADMIN", SysPerm: -1}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/admin/orders/:id/financials", AdminOrderFinancials)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/orders/"+orderID+"/financials", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			CashPaid     models.Cents `json:"cash_paid"`
			LedgerPaid   models.Cents `json:"ledger_paid"`
			LedgerRefund models.Cents `json:"ledger_refund"`
			LedgerNet    models.Cents `json:"ledger_net"`
			Diff         models.Cents `json:"diff"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.FromYuan(1200), resp.Data.CashPaid)
	assert.Equal(t, models.FromYuan(1500), resp.Data.LedgerPaid)
	assert.Equal(t, models.FromYuan(300), resp.Data.LedgerRefund)
	assert.Equal(t, models.FromYuan(1200), resp.Data.LedgerNet)
	assert.Equal(t, models.Cents(0), resp.Data.Diff)
}
