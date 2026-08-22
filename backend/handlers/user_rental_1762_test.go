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

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1762 regression tests: end_date is recomputed server-side from
// rent_days (authoritative) — the client-supplied end_date is overridden.

// TestCreateOrder_RentDaysOverridesEndDate (#1762): rent_days=3 with a
// bogus 30-day end_date → persisted end_date = start + 2 days.
func TestCreateOrder_RentDaysOverridesEndDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupUserRentalTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	instrumentID := uuid.New().String()

	require.NoError(t, db.Create(&models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}).Error)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders", handler.CreateOrder)

	// Frontend submits rent_days=3 but a wrong (30-day) end_date — the
	// server must recompute end_date = 2024-01-01 + 2 = 2024-01-03.
	reqBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-31", // bogus: 30 days, must be overridden
		"rent_days":     3,
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, 20000, response.Code)

	var order models.Order
	require.NoError(t, db.Where("id = ?", response.Data.OrderID).First(&order).Error)
	require.NotNil(t, order.EndDate)
	require.Contains(t, *order.EndDate, "2024-01-03", "end_date recomputed from rent_days (server authoritative)")
}

// TestCreateOrder_NoRentDays_LegacyPath (#1762): rent_days=0 keeps the
// legacy behavior — days derived from submitted dates, no recompute.
func TestCreateOrder_NoRentDays_LegacyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupUserRentalTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	instrumentID := uuid.New().String()

	require.NoError(t, db.Create(&models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}).Error)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders", handler.CreateOrder)

	reqBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-15",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, 20000, response.Code)

	var order models.Order
	require.NoError(t, db.Where("id = ?", response.Data.OrderID).First(&order).Error)
	require.NotNil(t, order.EndDate)
	require.Contains(t, *order.EndDate, "2024-01-15", "legacy path keeps submitted end_date")
}

// TestBatchCreateOrder_RentDaysOverridesEndDate (#1762): batch path
// recomputes end_date per item from rent_qty.
func TestBatchCreateOrder_RentDaysOverridesEndDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupUserRentalTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	instrumentID1 := uuid.New().String()
	instrumentID2 := uuid.New().String()

	require.NoError(t, db.Create(&models.Instrument{
		ID:          instrumentID1,
		TenantID:    tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID:          instrumentID2,
		TenantID:    tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 12.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}).Error)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders/batch", handler.BatchCreateOrder)

	reqBody := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"instrument_id": instrumentID1,
				"start_date":    "2024-02-01",
				"end_date":      "2024-03-02", // bogus 30 days
				"rent_days":     3,
			},
			{
				"instrument_id": instrumentID2,
				"start_date":    "2024-02-01",
				"end_date":      "2024-02-10", // kept (no rent_days)
			},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/user/orders/batch", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Orders []struct {
				OrderID string `json:"order_id"`
			} `json:"orders"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, 20000, response.Code)
	require.Len(t, response.Data.Orders, 2)

	var order1, order2 models.Order
	require.NoError(t, db.Where("id = ?", response.Data.Orders[0].OrderID).First(&order1).Error)
	require.NoError(t, db.Where("id = ?", response.Data.Orders[1].OrderID).First(&order2).Error)

	// First item: rent_days=3 → end_date recomputed to start+2.
	require.NotNil(t, order1.EndDate)
	require.Contains(t, *order1.EndDate, "2024-02-03", "batch: rent_days overrides end_date")

	// Second item: no rent_days → legacy path keeps submitted end_date.
	require.NotNil(t, order2.EndDate)
	require.Contains(t, *order2.EndDate, "2024-02-10", "batch: no rent_days keeps submitted end_date")
}
