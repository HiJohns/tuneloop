package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1781 regression tests for renewal pricing:
//   - snapshot base_daily_rent=0 with pricing_tiers[0].daily_rate=1 (cents)
//     must recover daily rate from tiers (CV-08 scenario)
//   - ConfirmRenewal must not apply FromYuan (×100) to already-cent values

// setupRenewalOrderZeroSnapshot creates an order matching CV-08 production data:
// instrument base_daily_rate=NULL, snapshot base_daily_rent=0, monthly_rent=0,
// but pricing_tiers[0].daily_rate=1 (cents, P3-migrated).
func setupRenewalOrderZeroSnapshot(t *testing.T, tenantID, userID, orgID string) (string, string) {
	t.Helper()
	db := database.GetDB()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "CV08-" + uuid.New().String()[:8],
		BaseDailyRate: nil, // CV-08: base_daily_rate not configured
		Pricing:       `{"daily_rent":0.01,"overdue_daily_fee":15,"tiers":[{"days_max":30,"daily_rate":1},{"days_max":-1,"daily_rate":1}]}`,
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	startDate := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	// CV-08 snapshot: base_daily_rent=0 (bdr not configured at creation),
	// pricing_tiers[0].daily_rate=1 (cents, P3-migrated)
	pricingBreakdown := `{"base_daily_rent":0,"rent_days":3,"monthly_rent":0,"pricing_tiers":[{"days_max":30,"daily_rate":1}],"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1},{"days_max":-1,"discount_percent":0,"daily_rate":1}]}`

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        30,
		MonthlyRent:      0,  // CV-08: monthly_rent=0
		Status:           models.OrderStatusInLease,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	return instrument.ID, order.ID
}

// TestRenewal_ZeroSnapshot_TierFallback (#1781): CV-08 reproduction —
// snapshot base_daily_rent=0, instrument base_daily_rate=NULL, monthly_rent=0,
// but pricing_tiers[0].daily_rate=1 (cent). Must recover daily rate from
// pricing_tiers and return renewal_cost=1 (1 day × 1 cent).
func TestRenewal_ZeroSnapshot_TierFallback(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-0000-0000-00000000e101"
	userID := "00000000-0000-0000-0000-00000000e102"
	orgID := "00000000-0000-0000-0000-00000000e103"
	_, orderID := setupRenewalOrderZeroSnapshot(t, tenantID, userID, orgID)

	router := renewalRouter(tenantID, userID)
	body, _ := json.Marshal(map[string]interface{}{"additional_days": 1})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/calculate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DailyRate   float64 `json:"daily_rate"`
			RenewalCost float64 `json:"renewal_cost"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())

	// Daily rate must be 1 cent (from pricing_tiers[0].daily_rate), not 0.
	require.Equal(t, 1.0, resp.Data.DailyRate, "daily rate = 1 cent from pricing_tiers fallback")
	// 1 day × 1 cent = 1 cent (not ¥0.00)
	require.InDelta(t, 1.0, resp.Data.RenewalCost, 0.01, "1 day × 1 cent = 1 cent")
}

// TestRenewal_Confirm_AmountCents (#1781): ConfirmRenewal with the same
// zero-snapshot scenario must create a payment record with Amount=1 (cent),
// NOT Amount=100 (which would result from FromYuan(1) = 1×100 = 100).
func TestRenewal_Confirm_AmountCents(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	testfixtures.SetupWechatPayMock(t)

	db := database.GetDB()
	// ConfirmRenewal creates order_payment_records — ensure table exists.
	if !db.Migrator().HasTable(&models.OrderPaymentRecord{}) {
		require.NoError(t, db.Migrator().CreateTable(&models.OrderPaymentRecord{}))
	}
	// Clean any stale records from prior test runs.
	db.Exec("DELETE FROM order_payment_records WHERE tenant_id = ?", "00000000-0000-0000-0000-00000000e201")

	tenantID := "00000000-0000-0000-0000-00000000e201"
	userID := "00000000-0000-0000-0000-00000000e202"
	orgID := "00000000-0000-0000-0000-00000000e203"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "cv08-confirm", WxOpenid: "mock-openid-cv08", Status: "active",
	}).Error)

	_, orderID := setupRenewalOrderZeroSnapshot(t, tenantID, userID, orgID)
	router := renewalRouter(tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"additional_days": 1,
		"open_id":         "mock-openid-cv08",
	})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	t.Logf("confirm response body: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Success bool `json:"success"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())

	// Verify payment record: Amount must be 1 cent, NOT 100.
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND type = 'payment'", orderID).First(&record).Error)
	t.Logf("payment record: id=%s amount=%d status=%s", record.ID, record.Amount, record.Status)
	// Also check raw DB value.
	var rawAmount int64
	db.Raw("SELECT amount FROM order_payment_records WHERE order_id = ? AND type = 'payment'", orderID).Scan(&rawAmount)
	t.Logf("raw DB amount: %d", rawAmount)
	require.Equal(t, models.Cents(1), record.Amount, "payment record Amount = 1 cent (not 100 from FromYuan misuse)")
}
