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

// #1756 regression tests: GetOrder fee_summary three-block structure
// (paid / payable / expected), all cents, server-computed.

// TestGetOrder_FeeSummary_Unsettled (#1756): returning order with a paid
// record and shipping fee → paid block = rent payment, payable block =
// actual rent + shipping, expected = refund (paid − payable).
func TestGetOrder_FeeSummary_Unsettled(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "feesum-unsettled", Name: "Fee User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEESUM", StockStatus: "rented",
	}).Error)

	start := "2026-08-01"
	end := "2026-08-04"
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    &start, EndDate: &end,
		DeliveredAt: &deliveredAt, ReturnedAt: &returnedAt,
		LeaseTerm:    3,
		Status:       models.OrderStatusReturning,
		Deposit:     models.FromYuan(1), // 1 元
		CashPaid:    models.FromYuan(4), // 租金 3 元 + 押金 1 元 = 4 元 (400 分)
		ShippingFee: models.FromYuan(1), // 1 元未付
		// total_amount = 300 分（3 天 × 1 元）
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":100,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":3,"rate":100,"discount":1,"subtotal":300}],"total_amount":300}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Paid payment record (order_type=rent, 400 分 = rent 300 + deposit 100).
	outTradeNo := "feesum-otn-1"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: 400, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
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
			FeeSummary struct {
				Paid struct {
					Initial []struct {
						Item   string `json:"item"`
						Amount int64  `json:"amount"`
					} `json:"initial"`
					Renewal  []struct {
						Item   string `json:"item"`
						Amount int64  `json:"amount"`
					} `json:"renewal"`
					Subtotal int64 `json:"subtotal"`
				} `json:"paid"`
				Payable struct {
					Items []struct {
						Item   string `json:"item"`
						Amount int64  `json:"amount"`
					} `json:"items"`
					Subtotal int64 `json:"subtotal"`
				} `json:"payable"`
				Expected *struct {
					Direction string `json:"direction"`
					Amount    int64  `json:"amount"`
				} `json:"expected"`
				Settled bool `json:"settled"`
			} `json:"fee_summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	fs := resp.Data.FeeSummary
	// Paid: rent 400 分 (initial), no renewal.
	require.Len(t, fs.Paid.Initial, 1)
	require.Equal(t, "rent", fs.Paid.Initial[0].Item)
	require.Equal(t, int64(400), fs.Paid.Initial[0].Amount)
	require.Len(t, fs.Paid.Renewal, 0)
	require.Equal(t, int64(400), fs.Paid.Subtotal)

	// Payable: actual rent 300 + shipping 100 = 400.
	require.Len(t, fs.Payable.Items, 2)
	require.Equal(t, int64(300), fs.Payable.Items[0].Amount, "actual rent cents")
	require.Equal(t, int64(100), fs.Payable.Items[1].Amount, "shipping cents")
	require.Equal(t, int64(400), fs.Payable.Subtotal)

	// Unsettled (returning) → expected block present.
	require.False(t, fs.Settled)
	require.NotNil(t, fs.Expected)
	require.Equal(t, "refund", fs.Expected.Direction)
	require.Equal(t, int64(0), fs.Expected.Amount, "paid 400 − payable 400 = 0")
}

// TestGetOrder_FeeSummary_Settled (#1756): completed order → expected is
// null (settled), no expected block.
func TestGetOrder_FeeSummary_Settled(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "feesum-settled", Name: "Settled User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEESUM-S", StockStatus: "available",
	}).Error)

	start := "2026-08-01"
	end := "2026-08-04"
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    &start, EndDate: &end,
		DeliveredAt: &deliveredAt, ReturnedAt: &returnedAt,
		LeaseTerm:    3,
		Status:       models.OrderStatusCompleted,
		Deposit:     models.FromYuan(1),
		CashPaid:    models.FromYuan(4),
		ShippingFee: models.FromYuan(1),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":100,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":3,"rate":100,"discount":1,"subtotal":300}],"total_amount":300}`),
	}
	require.NoError(t, db.Create(&order).Error)

	outTradeNo := "feesum-otn-2"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: 400, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
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
			FeeSummary struct {
				Paid struct {
					Subtotal int64 `json:"subtotal"`
				} `json:"paid"`
				Payable struct {
					Subtotal int64 `json:"subtotal"`
				} `json:"payable"`
				Expected *struct {
					Direction string `json:"direction"`
				} `json:"expected"`
				Settled bool `json:"settled"`
			} `json:"fee_summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	fs := resp.Data.FeeSummary
	require.True(t, fs.Settled, "completed order is settled")
	require.Nil(t, fs.Expected, "no expected block when settled")
	require.Equal(t, int64(400), fs.Paid.Subtotal)
	require.Equal(t, int64(400), fs.Payable.Subtotal)
}
