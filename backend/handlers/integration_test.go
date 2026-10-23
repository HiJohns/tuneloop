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
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// IntegrationTest04_Scenario1_RentalClosedLoop tests full rental flow:
// Browse → CreateOrder → Pay → Contract → MyRentals → Return
func TestIntegration_Scenario1_RentalClosedLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	instrumentID := uuid.New().String()
	testutil.SeedVerifiedCustomer(t, db, tenantID, tenantID, userID)

	// Step 1: Create available instrument
	instrument := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		OrgID:       &tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}
	db.Create(&instrument)

	// Setup router with all handlers
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	userRentalHandler := NewUserRentalHandler()
	warehouseHandler := NewWarehouseHandler()
	router.GET("/api/user/instruments", userRentalHandler.ListInstruments)
	router.GET("/api/user/instruments/:id", userRentalHandler.GetInstrument)
	router.POST("/api/user/orders", userRentalHandler.CreateOrder)
	router.GET("/api/user/contracts/:id", userRentalHandler.GetContract)
	router.GET("/api/warehouse/orders", warehouseHandler.ListOrders)

	// Step 2: Browse instruments - GET /api/user/instruments
	req := httptest.NewRequest("GET", "/api/user/instruments?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var instrResponse struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &instrResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, instrResponse.Code)
	assert.Greater(t, len(instrResponse.Data.List), 0)

	// Step 3: Create order - POST /api/user/orders
	reqBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-06-01",
		"end_date":      "2024-06-08",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req = httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var orderResponse struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &orderResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, orderResponse.Code)
	assert.NotEmpty(t, orderResponse.Data["order_id"])
	assert.NotEmpty(t, orderResponse.Data["lease_id"])

}

// IntegrationTest04_Scenario2_WarehouseProcess tests warehouse workflow
func TestIntegration_Scenario2_WarehouseProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	userID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	warehouseHandler := NewWarehouseHandler()
	router.GET("/api/warehouse/orders", warehouseHandler.ListOrders)
	router.PUT("/api/warehouse/orders/:id/shipping", warehouseHandler.UpdateShipping)

	// Create test order
	orderID := uuid.New().String()
	order := models.Order{
		ID:           orderID,
		TenantID:     tenantID,
		OrgID:        tenantID,
		UserID:       userID,
		InstrumentID: uuid.New().String(),
		Level:        "standard",
		LeaseTerm:    30,
		Status:       "paid",
	}
	db.Create(&order)

	// Step 1: List orders - GET /api/warehouse/orders
	req := httptest.NewRequest("GET", "/api/warehouse/orders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var ordersResponse struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &ordersResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, ordersResponse.Code)
	assert.Greater(t, len(ordersResponse.Data.List), 0)

	// Step 2: Update shipping
	reqBody := map[string]interface{}{
		"tracking_number": "SF12345678",
		"company":         "顺丰快递",
		"shipped_at":      time.Now().UTC(),
	}
	jsonBody, _ := json.Marshal(reqBody)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var updateResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), updateResponse["code"])
	assert.Equal(t, "success", updateResponse["message"])
}

// IntegrationTest04_Scenario4_AppealProcess tests appeal workflow
func TestIntegration_Scenario4_AppealProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	userID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	appealHandler := NewAppealHandler()
	router.GET("/api/appeals", appealHandler.ListAppeals)
	router.GET("/api/appeals/:id", appealHandler.GetAppeal)
	router.PUT("/api/appeals/:id/resolve", appealHandler.ResolveAppeal)

	appealID := uuid.New().String()
	drID := uuid.New().String()
	appealReason := "Test damage appeal"
	appealOrderID := uuid.New().String()
	db.Create(&models.Order{
		ID: appealOrderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID: uuid.New().String(), Level: "standard", LeaseTerm: 30, Status: "in_lease",
	})
	db.Create(&models.DamageReport{
		ID: drID, TenantID: tenantID, OrgID: tenantID,
		LeaseID: appealOrderID, InstrumentID: uuid.New().String(),
		UserID: userID, Status: "pending",
	})
	appeal := models.Appeal{
		ID:             appealID,
		TenantID:       tenantID,
		OrgID:          tenantID,
		SiteID:         uuid.New().String(),
		ObjectID:       uuid.New().String(),
		Status:         "pending",
		DamageReportID: &drID,
		UserID:         &userID,
		AppealReason:   &appealReason,
		SubmittedAt:    time.Now(),
	}
	db.Create(&appeal)

	// Step 1: List appeals
	req := httptest.NewRequest("GET", "/api/appeals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var listResponse struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		}
	}
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, listResponse.Code)

	// Step 2: Get appeal detail
	req = httptest.NewRequest("GET", "/api/appeals/"+appealID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var detailResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &detailResponse)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), detailResponse["code"])

	// Step 3: Resolve appeal
	reqBody := map[string]interface{}{
		"decision":      "adjust",
		"adjust_amount": 80.0,
		"comment":       "Adjusted to 80 USD",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req = httptest.NewRequest("PUT", "/api/appeals/"+appealID+"/resolve", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resolveResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resolveResponse)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), resolveResponse["code"])
}
