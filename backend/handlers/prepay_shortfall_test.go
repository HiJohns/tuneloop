package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestPrepayShortfall_OREZ 验证 #1838：补缴（order_type=payment_shortfall）
// 使用 OREZ（waive 100%）→ 金额 0 → waived 记账 + 补缴闭环完成。
// 回归点：out_trade_no 原生成 17+18=35 字符超 varchar(32) → INSERT 500，
// 修复后前缀截断 ≤32（普通补缴支付同样依赖该修复）。
func TestPrepayShortfall_OREZ(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f1",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "ShortfallOREZ",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))
	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "OREZ", Type: "waive", Value: 0, Active: true,
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: user.TenantID, OrgID: user.OrgID,
		UserID: user.ID, InstrumentID: uuid.New().String(),
		Status: models.OrderStatusReturning,
	}
	require.NoError(t, db.Create(&order).Error)

	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_id":    order.ID,
		"order_type":  "payment_shortfall",
		"amount":      0.28,
		"coupon_code": "OREZ",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.True(t, resp.Data.Success)

	outTradeNo := resp.Data.Data.OutTradeNo
	assert.LessOrEqual(t, len(outTradeNo), 32, "out_trade_no must fit varchar(32)")

	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", outTradeNo).First(&record).Error)
	assert.Equal(t, models.Cents(0), record.Amount, "OREZ re-priced shortfall to 0")
	assert.Equal(t, "paid", record.Status)
	assert.Equal(t, "waived", *record.Method)
	assert.Equal(t, "payment_shortfall", record.OrderType)

	// 补缴闭环：订单 → completed。
	var orderAfter models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&orderAfter).Error)
	assert.Equal(t, models.OrderStatusCompleted, orderAfter.Status, "shortfall waive completes the order")
}
