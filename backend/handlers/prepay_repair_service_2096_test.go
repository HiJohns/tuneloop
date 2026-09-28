package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// TestPrepayRepairService_Alias_2096：审计 P0 回归（#2096）——标准支付页以
// `order_type=repair_service` 发起 prepay（Payment.jsx 直传 URL type，与
// /pay/calculate 的 type 同源）时，入口必须规范化为 "repair"（#1942 分支：
// 金额权威/状态门/侧效应），不得 400 invalid order_type 阻断支付主链路。
// 同时验证金额取服务端权威（客户端金额不可信）。
func TestPrepayRepairService_Alias_2096(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000a1",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "RepairAliasPrepay",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	repairCents := models.FromYuan(200) // 修理费 200.00
	logistics := models.FromYuan(50)    // 物流费预估 50.00
	rr := models.RepairRequest{
		ID: uuid.New().String(), TenantID: user.TenantID, UserID: user.ID,
		Type: repairServiceTypeVal, Status: models.RepairReqStatusPendingPay,
		Description:         "2096 alias prepay",
		QuoteRepairCents:    &repairCents,
		QuoteLogisticsCents: &logistics,
		CreatedAt:           time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Omit("site_id", "user_instrument_id", "accepted_quote_id").Create(&rr).Error)

	// 注入 stub：无 openid 时走 Native fallback（测试请求非小程序 UA），
	// 避免真实微信调用；结束恢复全局。
	wechatpay.SetClientForTesting(stubTimeoutQueryClient{}, &wechatpay.Config{})
	defer wechatpay.ResetGlobalForTesting()

	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	// 审计 P0 场景：order_type=repair_service（旧实现 → 400 invalid order_type）；
	// amount 故意传错值（9.99）验证服务端权威计价（应取 250.00）。
	body, _ := json.Marshal(map[string]interface{}{
		"order_id":   rr.ID,
		"order_type": "repair_service",
		"amount":     9.99,
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay(repair_service) must not be rejected: %s", w.Body.String())

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

	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", resp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, "repair", record.OrderType, "别名必须规范化为 repair（#1942 分支）")
	assert.Equal(t, models.FromYuan(250), record.Amount, "金额取服务端权威（客户端 9.99 不可信）")

	var rrAfter models.RepairRequest
	require.NoError(t, db.Where("id = ?", rr.ID).First(&rrAfter).Error)
	assert.Equal(t, models.RepairReqStatusPendingPay, rrAfter.Status, "未支付回调前订单保持待支付")
}
