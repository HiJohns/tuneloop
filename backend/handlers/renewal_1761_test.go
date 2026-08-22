package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1761 regression tests for renewal pricing:
//   - tier segmentation must start from tier 1 (consumedDays from
//     pricing_breakdown.rent_days, not a stale lease_term)
//   - baseRate must be cents (instrument base_daily_rate / monthly_rent
//     cents /30), never a hardcoded ¥0.50 fallback

// setupRenewalOrderCents creates a renewal order whose lease_term is
// intentionally stale (30) while pricing_breakdown.rent_days = 3 — the
// #1762/#1761 mismatch scenario.
func setupRenewalOrderCents(t *testing.T, tenantID, userID, orgID string) (string, string) {
	t.Helper()
	db := database.GetDB()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "RN-C-" + uuid.New().String()[:8],
		BaseDailyRate: models.ToCentsPtr(float64Ptr(0.01)), // ¥0.01/day = 1 cent
		Pricing:       `{"daily_rent":1,"overdue_daily_fee":15,"tiers":[{"days_max":30,"daily_rate":1},{"days_max":180,"daily_rate":1},{"days_max":-1,"daily_rate":1}]}`,
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	startDate := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	// rent_days=3 (真实 3 天租期) but lease_term=30 (错误 end_date 推导, #1762)
	pricingBreakdown := `{"base_daily_rent":1,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1},{"days_max":180,"discount_percent":0,"daily_rate":1},{"days_max":-1,"discount_percent":0,"daily_rate":1}]}`

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        30, // stale — must not drive consumedDays
		MonthlyRent:      0,  // force instrument base_daily_rate fallback
		Status:           models.OrderStatusInLease,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	return instrument.ID, order.ID
}

// TestRenewal_TierStartsWithTier1_AndCentsPricing (#1761): with rent_days=3
// (consumedDays=3) and 3-day renewal, the breakdown must show tier 1
// (not skip to tier 2), with ¥0.01/day from the instrument pricing.
func TestRenewal_TierStartsWithTier1_AndCentsPricing(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-0000-0000-00000000c101"
	userID := "00000000-0000-0000-0000-00000000c102"
	orgID := "00000000-0000-0000-0000-00000000c103"
	_, orderID := setupRenewalOrderCents(t, tenantID, userID, orgID)

	router := renewalRouter(tenantID, userID)
	body, _ := json.Marshal(map[string]interface{}{"additional_days": 3})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/calculate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DailyRate    float64 `json:"daily_rate"`
			RenewalCost  float64 `json:"renewal_cost"`
			TierBreakdown []struct {
				Tier     int     `json:"tier"`
				Days     int     `json:"days"`
				Rate     float64 `json:"rate"`
				Discount float64 `json:"discount"`
				Subtotal float64 `json:"subtotal"`
			} `json:"tier_breakdown"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// consumedDays=3 → renewal 3 days all inside tier 1 (30-day tier),
	// breakdown starts at tier 1, never skips to tier 2.
	require.Len(t, resp.Data.TierBreakdown, 1, "3-day renewal inside tier 1 → single segment")
	require.Equal(t, 1, resp.Data.TierBreakdown[0].Tier, "segmentation must start at tier 1")
	require.Equal(t, 3, resp.Data.TierBreakdown[0].Days)

	// ¥0.01/day cents contract (not ¥0.50).
	require.Equal(t, 1.0, resp.Data.DailyRate, "daily rate = 1 cent (¥0.01), not 50")
	require.InDelta(t, 3.0, resp.Data.RenewalCost, 0.01, "3 days × 1 cent = 3 cents")
}

// TestRenewal_MonthlyRentFallback_Cents (#1761): when the pricing snapshot
// fails to parse AND the instrument has no base_daily_rate, monthly rent
// (cents) /30 must be used — a ¥30/month instrument → 100 cents/day.
func TestRenewal_MonthlyRentFallback_Cents(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-0000-0000-00000000c201"
	userID := "00000000-0000-0000-0000-00000000c202"
	orgID := "00000000-0000-0000-0000-00000000c203"

	db := database.GetDB()
	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "RN-M-" + uuid.New().String()[:8],
		BaseDailyRate: nil, // no instrument rate → monthly fallback
		Pricing:       `{"daily_rent":1,"overdue_daily_fee":15,"tiers":[{"days_max":30,"daily_rate":1}]}`,
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	startDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	order := models.Order{
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		StartDate:    &startDate,
		EndDate:      &endDate,
		LeaseTerm:    1,
		MonthlyRent:  models.FromYuan(30), // ¥30/month = 3000 cents → /30 = 100 cents/day
		Status:       models.OrderStatusInLease,
		// no PricingBreakdown → snapshot parse fails → monthly fallback path
	}
	require.NoError(t, db.Create(&order).Error)

	router := renewalRouter(tenantID, userID)
	body, _ := json.Marshal(map[string]interface{}{"additional_days": 1})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID(order)+"/renewal/calculate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DailyRate float64 `json:"daily_rate"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 100.0, resp.Data.DailyRate, "¥30/month → 100 cents/day, not 50 (¥0.50)")
}

func orderID(order models.Order) string { return order.ID }
