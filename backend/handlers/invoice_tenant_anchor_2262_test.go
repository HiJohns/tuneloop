package handlers

// #2262 B4：发票归属由订单派生，禁止客户端指定他商户 tenant。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceSubmit_TenantDerivedFromOrders_2262(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setup := func(t *testing.T) (*gin.Engine, *models.Order, *models.Order, string, string) {
		t.Helper()
		db := testfixtures.SetupTestDB(t)
		tenantA := uuid.NewString()
		tenantB := uuid.NewString()
		userID := uuid.NewString()
		testutil.SeedVerifiedCustomer(t, db, tenantA, tenantA, userID)

		mk := func(tenant string, applied bool) *models.Order {
			o := &models.Order{
				ID: uuid.NewString(), TenantID: tenant, OrgID: tenant, UserID: userID,
				InstrumentID: uuid.NewString(), Level: "standard", LeaseTerm: 30,
				Status: models.OrderStatusCompleted, InvoiceApplied: applied,
			}
			require.NoError(t, db.Create(o).Error)
			return o
		}
		oA1 := mk(tenantA, false)
		oA2 := mk(tenantA, false)
		oB := mk(tenantB, false)

		customer := testutil.TestActor{TenantID: "", OrgID: "", UserID: userID, Role: "USER"}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
			c.Next()
		})
		r.POST("/user/invoices", (&InvoiceHandler{}).Submit)
		return r, oA1, oA2, oB.TenantID, tenantA
	}

	post := func(r *gin.Engine, body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/user/invoices", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	t.Run("client_tenant_mismatch_rejected", func(t *testing.T) {
		r, oA1, oA2, tenantB, _ := setup(t)
		w, _ := post(r, map[string]interface{}{"groups": []map[string]interface{}{{
			"order_ids": []string{oA1.ID, oA2.ID}, "tenant_id": tenantB, "invoice_type": "普通", "title": "个人",
		}}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var n int64
		require.NoError(t, database.GetDB().Model(&models.InvoiceApplication{}).Count(&n).Error)
		assert.Zero(t, n, "拒绝时不得写入申请")
	})

	t.Run("tenant_derived_from_orders", func(t *testing.T) {
		r, oA1, oA2, _, tenantA := setup(t)
		w, resp := post(r, map[string]interface{}{"groups": []map[string]interface{}{{
			"order_ids": []string{oA1.ID, oA2.ID}, "invoice_type": "普通", "title": "个人",
		}}})
		require.Equal(t, http.StatusOK, w.Code, "%v", resp)
		var app models.InvoiceApplication
		require.NoError(t, database.GetDB().Order("created_at desc").First(&app).Error)
		assert.Equal(t, tenantA, app.TenantID, "归属必须来自订单")
	})

	t.Run("cross_merchant_merge_rejected", func(t *testing.T) {
		r, oA1, _, _, _ := setup(t)
		db := database.GetDB()
		oB2 := &models.Order{ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: uuid.NewString(), UserID: oA1.UserID,
			InstrumentID: uuid.NewString(), Level: "standard", LeaseTerm: 30, Status: models.OrderStatusCompleted}
		require.NoError(t, db.Create(oB2).Error)
		w, _ := post(r, map[string]interface{}{"groups": []map[string]interface{}{{
			"order_ids": []string{oA1.ID, oB2.ID}, "invoice_type": "普通", "title": "个人",
		}}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
