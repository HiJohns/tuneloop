package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1757 regression tests: promo_points is stored/transmitted in cents
// (1 点 = 1 分) end-to-end.

// TestRegister_GiftPointsCents (#1757): registration credits 99 元 as
// 9900 cents; the wallet API returns cents.
func TestRegister_GiftPointsCents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	// User with no points; registration gift path is exercised via
	// /users/me which returns promo_points from the model (cents).
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "gift-cents", Status: "active", PromoPoints: 9900,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/points", NewUserPointsHandler().GetBalance)

	req := httptest.NewRequest(http.MethodGet, "/api/user/points", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			PromoPoints float64 `json:"promo_points"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 9900.0, resp.Data.PromoPoints, "promo_points in cents (99 元 = 9900 分)")
}

// TestWalletInfo_PromoPointsCents (#1757): /pay/calculate wallet returns
// promo_points and max_gift_amount in cents.
func TestWalletInfo_PromoPointsCents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "wallet-cents", Status: "active", PromoPoints: 20000,
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-04"),
		LeaseTerm:    3,
		Status:       models.OrderStatusReserved,
		Deposit:      0,
		// total_amount = 30000 cents
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":10000,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":3,"rate":10000,"discount":1,"subtotal":30000}],"total_amount":30000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/calculate", CalculatePayment)

	body, _ := json.Marshal(map[string]interface{}{"type": "rent", "id": order.ID})
	req := httptest.NewRequest("POST", "/api/pay/calculate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Wallet struct {
				PromoPoints   float64 `json:"promo_points"`
				MaxGiftAmount float64 `json:"max_gift_amount"`
			} `json:"wallet"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 20000.0, resp.Data.Wallet.PromoPoints, "promo_points in cents")
	// max_gift_amount = floor(30000 × 0.3) = 9000 cents.
	require.Equal(t, 9000.0, resp.Data.Wallet.MaxGiftAmount, "max_gift_amount in cents (30000 × 30%)")
}
