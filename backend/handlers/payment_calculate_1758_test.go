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

// #1758 regression tests: loadRentPayment must produce a pure cents amount
// (no yuan/cents mixing — 7d5cadd7: 3 分 + 0.01 元 = 3.01 元 → 多扣 ¥2.97).

// TestCalculatePayment_Rent_Cents (#1758): rent calculate returns cents
// total = pb.total_amount (cents) + deposit + shipping (cents).
func TestCalculatePayment_Rent_Cents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "pay-calc-rent", Status: "active",
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-04"),
		LeaseTerm:    3,
		Status:       models.OrderStatusReserved,
		Deposit:      models.Cents(1), // 1 cent deposit
		// total_amount = 3 cents (3 days × 1 cent)
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1}],"tier_segments":[{"tier":1,"days":3,"rate":1,"discount":1,"subtotal":3}],"total_amount":3}`),
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
			Amount float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	// 3 cents rent + 1 cent deposit = 4 cents — never 3.01 (yuan mixing).
	require.Equal(t, 4.0, resp.Data.Amount, "total = 3 分 + 1 分 = 4 分 (not 3.01)")
}

// TestCalculatePayment_Rent_Cents_WithShipping (#1758): shipping fee cents
// added directly, still pure cents.
func TestCalculatePayment_Rent_Cents_WithShipping(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "pay-calc-ship", Status: "active",
	}).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-04"),
		LeaseTerm:    3,
		Status:       models.OrderStatusReserved,
		Deposit:      models.FromYuan(100), // 10000 cents
		ShippingFee:  models.FromYuan(50),  // 5000 cents
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
			Amount float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 45000.0, resp.Data.Amount, "30000 + 10000 + 5000 = 45000 分 = ¥450.00")
}
