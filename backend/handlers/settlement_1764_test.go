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

// #1764 regression tests for the fee_items directionized breakdown and the
// GetOrder merchant_name field (return settlement page).

// feeItemsFrom parses breakdown["fee_items"] into a lookup map keyed by item.
func feeItemsFrom(t *testing.T, breakdown map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	raw, ok := breakdown["fee_items"].([]map[string]interface{})
	require.True(t, ok, "breakdown must contain fee_items array")
	result := make(map[string]map[string]interface{}, len(raw))
	for _, it := range raw {
		item, _ := it["item"].(string)
		result[item] = it
	}
	return result
}

// TestComputeSettlement_FeeItems_Direction (#1764): per-item amounts follow
// paid-minus-payable; positive → refund, negative → pay, zero present.
func TestComputeSettlement_FeeItems_Direction(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) // within lease → no overdue
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-10"),
		LeaseTerm:    10,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      models.FromYuan(100),
		CashPaid:     models.FromYuan(300), // rent paid 200 + deposit 100
		// 10-day lease at ¥20/day; returned same day → rent payable ¥20.
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":2000,"rent_days":10,"tiers":[{"days_max":10,"discount_percent":0,"daily_rate":2000}],"tier_segments":[{"tier":1,"days":10,"rate":2000,"discount":1,"subtotal":20000}],"total_amount":20000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEE-DIR", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)
	feeItems := feeItemsFrom(t, result.Breakdown)

	// rent: paid rent (200) − payable (20) = +180 → refund 18000 cents
	require.Equal(t, "refund", feeItems["rent"]["direction"], "rent overpaid → refund")
	require.Equal(t, int64(18000), feeItems["rent"]["amount"], "rent refund = 20000 − 2000 cents")

	// deposit: 100 − (0 overdue + 0 damage + 0 shipping) = +100 → refund 10000
	require.Equal(t, "refund", feeItems["deposit"]["direction"], "deposit untouched → refund")
	require.Equal(t, int64(10000), feeItems["deposit"]["amount"])

	// no shipping/overdue/damage → amount 0 with direction pay (frontend hides)
	require.Equal(t, int64(0), feeItems["shipping_fee"]["amount"])
	require.Equal(t, int64(0), feeItems["overdue_fee"]["amount"])
	require.Equal(t, int64(0), feeItems["damage"]["amount"])
}

// TestComputeSettlement_FeeItems_Shortfall (#1764): when due exceeds paid,
// fee items flip to negative (pay direction) and match the aggregate
// payable_shortfall sign.
func TestComputeSettlement_FeeItems_Shortfall(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) // 5 days used
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-31"),
		LeaseTerm:    30,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      models.FromYuan(100),
		CashPaid:     models.FromYuan(300), // rent paid 200 + deposit 100
		ShippingFee:  models.FromYuan(50),  // shipping charged to deposit
		// 30-day lease at ¥20/day; 5 days used → payable rent ¥100.
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":2000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":2000}],"tier_segments":[{"tier":1,"days":30,"rate":2000,"discount":1,"subtotal":60000}],"total_amount":60000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEE-SHORT", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)
	feeItems := feeItemsFrom(t, result.Breakdown)

	// rent: 200 − 100 = +100 → refund
	require.Equal(t, "refund", feeItems["rent"]["direction"])
	require.Equal(t, int64(10000), feeItems["rent"]["amount"])

	// deposit: 100 − (0 + 0) = +100 → refund 10000 (shipping is separate line, #1784)
	require.Equal(t, "refund", feeItems["deposit"]["direction"])
	require.Equal(t, int64(10000), feeItems["deposit"]["amount"])

	// shipping_fee: 0 − 50 = −50 → pay 5000 (待补缴)
	require.Equal(t, "pay", feeItems["shipping_fee"]["direction"])
	require.Equal(t, int64(-5000), feeItems["shipping_fee"]["amount"])
}

// TestGetOrder_MerchantName (#1764): GetOrder resolves merchant_name from
// the order's tenant_id; absent merchant → empty string (frontend fallback).
func TestGetOrder_MerchantName(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "merch-name", Name: "Test User", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		Name: "Test Music Shop", Status: "active", AdminUID: userID,
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReserved,
	}
	require.NoError(t, db.Create(&order).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			MerchantName string `json:"merchant_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "Test Music Shop", resp.Data.MerchantName, "merchant_name resolved by tenant_id")
}

// TestGetOrder_MerchantName_Absent (#1764): no merchant row → empty string.
func TestGetOrder_MerchantName_Absent(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "merch-none", Name: "No Shop User", Status: "active",
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReserved,
	}
	require.NoError(t, db.Create(&order).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			MerchantName string `json:"merchant_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "", resp.Data.MerchantName, "absent merchant → empty (frontend falls back)")
}
