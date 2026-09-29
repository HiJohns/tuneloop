package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// #2110 回归：订单类支付必须携带 order_id——
//  1. prepay：空 order_id → 40002，且不产生孤儿支付记录（此前会走通用分支
//     生成 order_id 为空的 waived 记录并"支付成功"）
//  2. calculate：空/非法 id → 40002（此前空串致 SQLSTATE 22P02）
func TestPrepay_OrderIDRequired_2110(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	user := models.User{
		ID:       "21100000-0000-4000-8000-000000000001",
		IAMSub:   "21100000-0000-4000-8000-000000000001",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "empty-order-guard",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	customer := testutil.MakeCustomer("", user.IAMSub)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/pay/prepay", PrepayOrder)
	r.POST("/api/pay/calculate", CalculatePayment)

	var before int64
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Count(&before).Error)

	post := func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		t.Helper()
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// prepay：repair_service（别名→repair）空 order_id → 40002
	code, resp := post("/api/pay/prepay", map[string]interface{}{"order_id": "", "order_type": "repair_service", "amount": 3})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, float64(40002), resp["code"])

	// prepay：rent 空 order_id → 40002
	code, resp = post("/api/pay/prepay", map[string]interface{}{"order_id": "", "order_type": "rent", "amount": 3})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, float64(40002), resp["code"])

	// 无孤儿支付记录
	var after int64
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Count(&after).Error)
	assert.Equal(t, before, after, "空 order_id 不得产生支付记录")

	// calculate：空 / 非法 id → 40002（不再 22P02）
	code, resp = post("/api/pay/calculate", map[string]interface{}{"type": "repair_service", "id": ""})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, float64(40002), resp["code"])

	code, resp = post("/api/pay/calculate", map[string]interface{}{"type": "repair_service", "id": "not-a-uuid"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, float64(40002), resp["code"])
}
