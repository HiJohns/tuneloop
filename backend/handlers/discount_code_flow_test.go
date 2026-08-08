package handlers

import (
	"bytes"
	"context"
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
	"tuneloop-backend/testutil"
)

// TestDiscountCodeFlow covers TC #1554: discount code apply + usage tracking.
// Uses the real CreateOrder handler path so the discount flows through
// CalculatePricingBreakdown (rent reduction) and the usage recording in
// CreateOrder (DiscountCodeUsage row + usage_count increment).
func TestDiscountCodeFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	orgID := tenantID
	instrumentID := uuid.New().String()

	// Instrument: daily 100, deposit 500. Lease 10 days → rent 1000.
	baseRate := 100.0
	require.NoError(t, db.Create(&models.Instrument{
		ID:            instrumentID,
		TenantID:      tenantID,
		OrgID:         &orgID,
		StockStatus:   models.StockStatusAvailable,
		BaseDailyRate: &baseRate,
		Pricing:       `{"daily_rent":100.0,"monthly_rent":3000.0,"deposit":500.0}`,
	}).Error)

	// Default pricing template (no discount): 30-day tier at 100/day.
	require.NoError(t, db.Create(&models.PricingTemplate{
		ID:              uuid.New().String(),
		Code:            "default",
		Name:            "Default",
		IsSystemDefault: true,
		IsActive:        true,
		ConfigSchema:    `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20}],"deposit_ratio":0}`,
	}).Error)

	// Customer actor (guest: tid derived from instrument).
	customer := testutil.MakeCustomer("", userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	userRentalHandler := NewUserRentalHandler()
	router.POST("/api/user/orders", userRentalHandler.CreateOrder)

	// Helper: create an order with an optional discount code. Resets the
	// instrument to available first so multiple scenarios can reuse it
	// (CreateOrder marks the instrument rented).
	createOrder := func(code string) *httptest.ResponseRecorder {
		require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", instrumentID).
			Update("stock_status", models.StockStatusAvailable).Error)
		body := map[string]interface{}{
			"instrument_id": instrumentID,
			"start_date":    "2026-07-01",
			"end_date":      "2026-07-10", // 10 days
			"rent_days":     10,
		}
		if code != "" {
			body["discount_code"] = code
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// ------------------------------------------------------------------
	// Fixture: discount policy (10% off, no cap) + code (max_uses 2).
	// ------------------------------------------------------------------
	policy := models.DiscountPolicy{
		ID:           uuid.New().String(),
		Name:         "TEST-10PCT",
		RentDiscount: 0.9, // 10% off
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&policy).Error)

	validCode := models.DiscountCode{
		ID:        uuid.New().String(),
		Code:      "SAVE10",
		PolicyID:  policy.ID,
		MaxUses:   2,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&validCode).Error)

	// ------------------------------------------------------------------
	// Scenario 1: valid code → 10% rent discount applied.
	// ------------------------------------------------------------------
	t.Run("valid_code_applies_discount", func(t *testing.T) {
		w := createOrder("SAVE10")
		require.Equal(t, http.StatusCreated, w.Code, "create with valid code: %s", w.Body.String())

		var resp struct {
			Code int `json:"code"`
			Data struct {
				OrderID string `json:"order_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)

		// Rent 10 days × 100 = 1000, discount 10% → 900; + deposit 500 = 1400.
		var order models.Order
		require.NoError(t, db.Where("id = ?", resp.Data.OrderID).First(&order).Error)
		require.Equal(t, 1400.0, order.CashPaid, "cash = discounted rent 900 + deposit 500")

		// Pricing breakdown reflects the 0.9 discount factor.
		require.NotNil(t, order.PricingBreakdown)
		var pb struct {
			TierSegments []struct {
				Discount float64 `json:"discount"`
			} `json:"tier_segments"`
		}
		require.NoError(t, json.Unmarshal([]byte(*order.PricingBreakdown), &pb))
		require.Len(t, pb.TierSegments, 1)
		assert.InDelta(t, 0.9, pb.TierSegments[0].Discount, 0.0001, "tier discount includes 0.9 code factor")
	})

	// ------------------------------------------------------------------
	// Scenario 2: usage tracking — usage_count increments, usage row created.
	// ------------------------------------------------------------------
	t.Run("usage_tracking", func(t *testing.T) {
		w := createOrder("SAVE10")
		require.Equal(t, http.StatusCreated, w.Code, "second use: %s", w.Body.String())

		var dc models.DiscountCode
		require.NoError(t, db.Where("code = ?", "SAVE10").First(&dc).Error)
		require.Equal(t, 2, dc.UsageCount, "usage_count incremented to 2 after two orders")

		var usageCount int64
		require.NoError(t, db.Model(&models.DiscountCodeUsage{}).
			Where("code_id = ?", validCode.ID).Count(&usageCount).Error)
		require.Equal(t, int64(2), usageCount, "two DiscountCodeUsage rows")
	})

	// ------------------------------------------------------------------
	// Scenario 3: max_uses exhausted → discount no longer applied.
	// ------------------------------------------------------------------
	t.Run("max_uses_exhausted", func(t *testing.T) {
		w := createOrder("SAVE10")
		require.Equal(t, http.StatusCreated, w.Code, "third use still creates order")

		// No further usage recorded; full price charged.
		var usageCount int64
		require.NoError(t, db.Model(&models.DiscountCodeUsage{}).
			Where("code_id = ?", validCode.ID).Count(&usageCount).Error)
		require.Equal(t, int64(2), usageCount, "no new usage row when max_uses exhausted")

		// Order created at full price: rent 1000 + deposit 500 = 1500.
		var resp struct {
			Data struct {
				OrderID string `json:"order_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var order models.Order
		require.NoError(t, db.Where("id = ?", resp.Data.OrderID).First(&order).Error)
		require.Equal(t, 1500.0, order.CashPaid, "full price when code exhausted")
	})

	// ------------------------------------------------------------------
	// Scenario 4: invalid code → ignored (order created at full price).
	// ------------------------------------------------------------------
	t.Run("invalid_code_ignored", func(t *testing.T) {
		w := createOrder("NOPE99")
		require.Equal(t, http.StatusCreated, w.Code, "invalid code still creates order")

		var resp struct {
			Data struct {
				OrderID string `json:"order_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var order models.Order
		require.NoError(t, db.Where("id = ?", resp.Data.OrderID).First(&order).Error)
		require.Equal(t, 1500.0, order.CashPaid, "full price when code invalid")
	})

	// ------------------------------------------------------------------
	// Scenario 5: expired code → ignored.
	// ------------------------------------------------------------------
	t.Run("expired_code_ignored", func(t *testing.T) {
		expired := time.Now().Add(-24 * time.Hour)
		expiredCode := models.DiscountCode{
			ID:        uuid.New().String(),
			Code:      "EXPIRED1",
			PolicyID:  policy.ID,
			MaxUses:   0,
			ExpiresAt: &expired,
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, db.Create(&expiredCode).Error)

		w := createOrder("EXPIRED1")
		require.Equal(t, http.StatusCreated, w.Code)

		var resp struct {
			Data struct {
				OrderID string `json:"order_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var order models.Order
		require.NoError(t, db.Where("id = ?", resp.Data.OrderID).First(&order).Error)
		require.Equal(t, 1500.0, order.CashPaid, "full price when code expired")
	})
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
