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
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// #1826: 续期 + 优惠码支付走 /pay/prepay 重建 record 时，必须回填 days
// （续期天数），否则回调 applyRenewalSideEffects 因缺续期天数报
// "invalid renewal metadata"，续期永不生效。
//
// 真实链路：Renewal 页 → /orders/:id/renewal/confirm 创建带 days 的 record
// （前端丢弃 confirm 返回的预付单）→ 支付确认页 → /pay/prepay 重建 record
// （此前的 record 无 days → 回调失败）。
func TestPrepayRenewal_BackfillsDaysFromConfirmRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))

	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "ENO", Type: "percent", Value: 10, Active: true,
	}).Error)

	tenantID := uuid.New().String()
	localUserID := uuid.New().String() // users.id（订单 user_id 引用）
	iamSub := uuid.New().String()      // JWT user_id（iam_sub）
	user := models.User{
		ID:       localUserID,
		IAMSub:   iamSub,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "RenewalPayer",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "renewal_openid_001",
	}
	require.NoError(t, db.Create(&user).Error)

	_, orderID := setupRenewalOrder(t, tenantID, localUserID, tenantID, 5)

	// 模拟 ConfirmRenewal 已创建的带 days 的 renewal record。
	days := 1
	prevRecord := models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     &tenantID,
		UserID:    iamSub,
		OrderID:   &orderID,
		OrderType: "renewal",
		Type:      "payment",
		Status:    "pending",
		Amount:    models.Cents(3600),
		Days:      &days,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&prevRecord).Error)

	router := gin.New()
	customer := testutil.MakeCustomer(tenantID, iamSub)
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "renewal",
		"order_id":    orderID,
		"amount":      36.0, // 全价 36 元 = 3600 分
		"coupon_code": "ENO",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// prepay 重建的 record 必须回填 days，且金额为 ENO 折后（3600 分 × 10‰ = 36 分）。
	var newRecord models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND order_type = ? AND type = ? AND status = ?",
		orderID, "renewal", "payment", "pending").Order("created_at desc").First(&newRecord).Error)
	require.NotNil(t, newRecord.Days, "days must be backfilled from ConfirmRenewal record")
	require.Equal(t, 1, *newRecord.Days)
	require.Equal(t, models.Cents(36), newRecord.Amount, "ENO 1%: 3600 分 × 10‰ = 36 分")
}

// TestPrepayRenewal_NoConfirmRecord_Rejected (#1826): 无带 days 的 confirm
// record（绕过续期页直接 prepay）→ 40002 拒绝，避免创建无 days 的僵尸 record。
func TestPrepayRenewal_NoConfirmRecord_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))

	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "ENO", Type: "percent", Value: 10, Active: true,
	}).Error)

	tenantID := uuid.New().String()
	iamSub := uuid.New().String()
	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   iamSub,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "RenewalPayer",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "renewal_openid_002",
	}
	require.NoError(t, db.Create(&user).Error)

	_, orderID := setupRenewalOrder(t, tenantID, user.ID, tenantID, 5)
	// 不创建带 days 的 confirm record。

	router := gin.New()
	customer := testutil.MakeCustomer(tenantID, iamSub)
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "renewal",
		"order_id":    orderID,
		"amount":      36.0,
		"coupon_code": "ENO",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
}
