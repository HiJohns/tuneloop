package handlers

// #2293：维修师在详情页报价 → 状态 pending_payment → 顾客可见报价（真实 handler+DB）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepairServiceQuoteFlow_2293(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	tech := testutil.MakeSiteMember(f.orgID, f.orgID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "琴键松动", "technician_id": f.techID})
	require.Equal(t, float64(20000), resp["code"], resp)
	id := svcData(t, resp)["id"].(string)

	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	require.Equal(t, float64(20000), resp["code"], resp)

	// 技师报价（与详情页面板同一 API/字段）
	code, resp := svcPost(t, f, tech, "/repair-services/"+id+"/quote", gin.H{
		"quote_repair_cents": 20000, "quote_material_cents": 3000, "quote_logistics_cents": 5000,
	})
	require.Equal(t, http.StatusOK, code, resp)
	require.Equal(t, float64(20000), resp["code"], resp)

	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusPendingPay, stored.Status, "报价后状态必须转待付款")
	require.NotNil(t, stored.QuoteRepairCents)
	assert.Equal(t, models.Cents(20000), *stored.QuoteRepairCents)
	require.NotNil(t, stored.QuoteMaterialCents)
	assert.Equal(t, models.Cents(3000), *stored.QuoteMaterialCents)
	require.NotNil(t, stored.QuoteLogisticsCents)
	assert.Equal(t, models.Cents(5000), *stored.QuoteLogisticsCents)

	// 顾客侧详情可见报价
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+id, nil)
	req = req.WithContext(customer.InjectContext(req.Context()))
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := svcData(t, resp)
	rr, _ := data["repair"].(map[string]interface{})
	require.NotNil(t, rr, "detail.repair missing: %v", data)
	assert.Equal(t, models.RepairReqStatusPendingPay, rr["status"])
	assert.Equal(t, float64(20000), rr["quote_repair_cents"])
	assert.Equal(t, float64(3000), rr["quote_material_cents"])
	assert.Equal(t, float64(5000), rr["quote_logistics_cents"])
}
