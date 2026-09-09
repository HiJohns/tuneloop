package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1856 regression tests: GET /orders/:id 收支记录契约。
// - payment_records 条目必须透传 coupon_code / coupon_discount（waived 优惠码
//   减免金额在订单详情页可见，而非仅显示 ¥0.00 支付行）；
// - refund_records 只含实际发生退款的 settlement（零退款不生成噪音行）。

// TestGetOrder_PaymentRecords_WaivedCoupon (#1856): completed 订单含
// jsapi 实付记录 + OREZ waived 记录 → payment_records 两条，waived 条目
// 带 coupon_code="OREZ"、coupon_discount=65，jsapi 条目 coupon_discount=0。
func TestGetOrder_PaymentRecords_WaivedCoupon(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "waived-coupon", Name: "Waived User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-WAIVED-COUPON", StockStatus: "available",
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		Status:       models.OrderStatusCompleted,
		Deposit:      0,
		CashPaid:     models.FromYuan(0.72),
	}
	require.NoError(t, db.Create(&order).Error)

	// jsapi 实付 ¥0.36（rent）
	outTradeNo1 := "waived-otn-jsapi"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo1,
		Amount: 36, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CouponDiscount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	// OREZ waived ¥0.65 补缴（amount=0 + coupon_discount=65）
	outTradeNo2 := "waived-otn-orez"
	orez := "OREZ"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "payment_shortfall", OutTradeNo: &outTradeNo2,
		Amount: 0, Type: "payment", Status: "paid", Method: str1743Ptr("waived"),
		CouponCode: &orez, CouponDiscount: 65, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	// completed 订单 settle：零退款 → refund_records 为空（#1856 滤零）。
	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: order.ID,
		RefundMethod: "wechat_pay", RefundStatus: "completed",
		CashRefundable: 0, PrepaidRefunded: 0, GiftPointsRefunded: 0,
		Breakdown: "{}", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			PaymentRecords []struct {
				ID             string  `json:"id"`
				Amount         float64 `json:"amount"`
				Method         string  `json:"method"`
				Status         string  `json:"status"`
				CouponCode     string  `json:"coupon_code"`
				CouponDiscount int64   `json:"coupon_discount"`
			} `json:"payment_records"`
			RefundRecords []struct {
				ID     string  `json:"id"`
				Amount float64 `json:"amount"`
			} `json:"refund_records"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	require.Len(t, resp.Data.PaymentRecords, 2, "jsapi + waived 两条 paid 记录")

	// 按 method 定位两条记录（created_at 相同，顺序不稳定，不做下标假设）。
	var jsapiEntry, waivedEntry *struct {
		ID             string  `json:"id"`
		Amount         float64 `json:"amount"`
		Method         string  `json:"method"`
		Status         string  `json:"status"`
		CouponCode     string  `json:"coupon_code"`
		CouponDiscount int64   `json:"coupon_discount"`
	}
	for i := range resp.Data.PaymentRecords {
		pr := &resp.Data.PaymentRecords[i]
		switch pr.Method {
		case "jsapi":
			jsapiEntry = pr
		case "waived":
			waivedEntry = pr
		}
	}
	require.NotNil(t, jsapiEntry, "jsapi paid record present")
	require.NotNil(t, waivedEntry, "waived paid record present")

	require.Equal(t, float64(36), jsapiEntry.Amount)
	require.Equal(t, "", jsapiEntry.CouponCode, "jsapi 无优惠码")
	require.Equal(t, int64(0), jsapiEntry.CouponDiscount)

	require.Equal(t, float64(0), waivedEntry.Amount, "waived amount=0")
	require.Equal(t, "OREZ", waivedEntry.CouponCode, "waived 透传优惠码")
	require.Equal(t, int64(65), waivedEntry.CouponDiscount, "waived 透传减免分")

	require.Empty(t, resp.Data.RefundRecords, "零退款 settlement 不生成 refund 行")
}

// TestGetOrder_RefundRecords_OnlyNonZero (#1856): settlement 含实际退款 →
// refund_records 返回对应行（金额 50 分）；同时验证滤零只过滤零退款行。
func TestGetOrder_RefundRecords_OnlyNonZero(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "refund-nonzero", Name: "Refund User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-REFUND-NONZERO", StockStatus: "available",
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		Status:       models.OrderStatusCompleted,
		Deposit:      models.FromYuan(1),
		CashPaid:     models.FromYuan(1),
	}
	require.NoError(t, db.Create(&order).Error)

	outTradeNo := "refund-otn-1"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: 100, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CouponDiscount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	// 两条 settlement：早期零退款（应滤除）+ 最新实际退款 50 分（应保留）。
	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: order.ID,
		RefundMethod: "prepaid", RefundStatus: "pending",
		CashRefundable: 0, PrepaidRefunded: 0, GiftPointsRefunded: 0,
		Breakdown: "{}", CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-time.Hour),
	}).Error)
	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: order.ID,
		RefundMethod: "wechat_pay", RefundStatus: "completed",
		CashRefundable: 50, PrepaidRefunded: 0, GiftPointsRefunded: 0,
		Breakdown: "{}", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			RefundRecords []struct {
				Amount float64 `json:"amount"`
				Method string  `json:"method"`
				Status string  `json:"status"`
			} `json:"refund_records"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	require.Len(t, resp.Data.RefundRecords, 1, "零退款 settlement 滤除，仅保留实际退款")
	require.Equal(t, float64(50), resp.Data.RefundRecords[0].Amount)
	require.Equal(t, "wechat_pay", resp.Data.RefundRecords[0].Method)
	require.Equal(t, "completed", resp.Data.RefundRecords[0].Status)
}
