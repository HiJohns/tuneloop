package handlers

// #2291：优惠码定价纯函数 + /coupons/validate 契约 + prepay 一致性。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCouponPricing_Pure_2291(t *testing.T) {
	cases := []struct {
		name      string
		coupon    models.Coupon
		base      models.Cents
		wantFinal models.Cents
		wantDisc  models.Cents
		wantErrIs error
	}{
		{"waive", models.Coupon{Type: "waive"}, 12345, 0, 12345, nil},
		{"percent_10", models.Coupon{Type: "percent", Value: 10}, 12345, 123, 12222, nil},
		{"percent_1000", models.Coupon{Type: "percent", Value: 1000}, 12345, 12345, 0, nil},
		{"percent_zero", models.Coupon{Type: "percent", Value: 0}, 12345, 0, 0, ErrCouponConfig},
		{"percent_over", models.Coupon{Type: "percent", Value: 1001}, 12345, 0, 0, ErrCouponConfig},
		{"unknown_type", models.Coupon{Type: "bogus"}, 12345, 0, 0, ErrCouponType},
		{"negative_base", models.Coupon{Type: "waive"}, -1, 0, 0, ErrCouponAmount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCouponPricing(tc.coupon, tc.base)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantFinal, got.FinalCents)
			assert.Equal(t, tc.wantDisc, got.DiscountCents)
			assert.Equal(t, tc.base, got.FinalCents+got.DiscountCents, "final+discount 必等于原价")
		})
	}
}

func TestCouponValidate_API_2291(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.Coupon{ID: uuid.NewString(), Code: "PAY90", Type: "percent", Value: 900, Active: true}).Error)
	require.NoError(t, db.Create(&models.Coupon{ID: uuid.NewString(), Code: "WAIVEALL", Type: "waive", Active: true}).Error)
	require.NoError(t, db.Create(&models.Coupon{ID: uuid.NewString(), Code: "DISABLED", Type: "percent", Value: 10, Active: false}).Error)
	require.NoError(t, db.Create(&models.Coupon{ID: uuid.NewString(), Code: "BADCFG", Type: "percent", Value: 0, Active: true}).Error)

	r := gin.New()
	r.POST("/coupons/validate", ValidateCoupon)
	call := func(code string, amount int64) (int, map[string]interface{}) {
		body, _ := json.Marshal(map[string]interface{}{"code": code, "amount_cents": amount})
		req := httptest.NewRequest(http.MethodPost, "/coupons/validate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	code, resp := call("pay90", 10000)
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(20000), resp["code"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "PAY90", data["code"])
	assert.Equal(t, "percent", data["type"])
	assert.Equal(t, float64(1000), data["discount_cents"])
	assert.Equal(t, float64(9000), data["final_amount_cents"])

	code, resp = call("WAIVEALL", 10000)
	require.Equal(t, http.StatusOK, code, resp)
	data = resp["data"].(map[string]interface{})
	assert.Equal(t, float64(10000), data["discount_cents"])
	assert.Equal(t, float64(0), data["final_amount_cents"])

	code, resp = call("PAY90", 0)
	require.Equal(t, http.StatusOK, code, resp)
	data = resp["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["discount_cents"])
	assert.Equal(t, float64(0), data["final_amount_cents"])

	for _, tc := range []struct{ code, wantMsg string }{
		{"DISABLED", "优惠码无效"},
		{"NOPE", "优惠码无效"},
		{"BADCFG", "优惠码配置异常"},
	} {
		code, resp := call(tc.code, 10000)
		assert.Equal(t, http.StatusBadRequest, code, tc.code)
		assert.Equal(t, float64(40002), resp["code"], tc.code)
		assert.Contains(t, resp["message"], tc.wantMsg, tc.code)
	}
}

func TestPrepayCoupon_Consistency_2291(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

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

	require.NoError(t, db.Create(&models.Coupon{ID: uuid.NewString(), Code: "TEST10", Type: "percent", Value: 10, Active: true}).Error)
	user := models.User{
		ID: uuid.NewString(), IAMSub: "6d1e2c3a-0000-4000-8000-000000002291",
		TenantID: "00000000-0000-0000-0000-000000000000", OrgID: "00000000-0000-0000-0000-000000000000",
		Name: "CouponConsistency", Phone: "13800138291", Role: "USER", Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)

	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(customer.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/coupons/validate", ValidateCoupon)
	router.POST("/pay/prepay", PrepayOrder)

	// validate：100 元（10000 分）× 10‰ → 100 分
	vBody, _ := json.Marshal(map[string]interface{}{"code": "TEST10", "amount_cents": 10000})
	vReq := httptest.NewRequest(http.MethodPost, "/coupons/validate", bytes.NewBuffer(vBody))
	vReq.Header.Set("Content-Type", "application/json")
	vW := httptest.NewRecorder()
	router.ServeHTTP(vW, vReq)
	require.Equal(t, http.StatusOK, vW.Code, vW.Body.String())
	var vResp struct {
		Code int `json:"code"`
		Data struct {
			Final int64 `json:"final_amount_cents"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(vW.Body.Bytes(), &vResp))
	require.Equal(t, 20000, vResp.Code)

	// prepay：同码同金额 → 记账金额必须等于 validate.final
	pBody, _ := json.Marshal(map[string]interface{}{
		"order_id": uuid.NewString(), "order_type": "rent", "amount": 100.0,
		"coupon_code": "TEST10", "open_id": "mock-openid-2291",
	})
	pReq := httptest.NewRequest(http.MethodPost, "/pay/prepay", bytes.NewBuffer(pBody))
	pReq.Header.Set("Content-Type", "application/json")
	pW := httptest.NewRecorder()
	router.ServeHTTP(pW, pReq)
	require.Equal(t, http.StatusOK, pW.Code, pW.Body.String())
	var pResp struct {
		Code int `json:"code"`
		Data struct {
			Data struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(pW.Body.Bytes(), &pResp))
	require.Equal(t, 20000, pResp.Code)

	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", pResp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, models.Cents(vResp.Data.Final), record.Amount, "validate 与 prepay 记账金额必须一致")
	assert.Equal(t, models.Cents(100), record.Amount)
}
