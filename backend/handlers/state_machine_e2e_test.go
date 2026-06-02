package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScenarioA: Standard Merchant Closed Loop
// Flow: CreateOrder (reserved) → Pay (paid) → Ship (shipped) → ConfirmDelivery (in_lease) → ReturnOrder (returning) → InspectReturn (in_store)
func TestScenarioA_StandardClosedLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	// Setup tables
	_ = db.Migrator().DropTable(&models.Instrument{}, &models.Order{}, &models.LeaseSession{}, &models.OrderStatusHistory{}, &models.DamageAssessment{}, &models.Notification{})
	require.NoError(t, db.Migrator().CreateTable(&models.Instrument{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Order{}))
	require.NoError(t, db.Migrator().CreateTable(&models.LeaseSession{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderStatusHistory{}))
	require.NoError(t, db.Migrator().CreateTable(&models.DamageAssessment{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Notification{}))

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	orgID := tenantID

	// Create test data: instrument and lease session
	instrumentID := uuid.New().String()
	db.Create(&models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: models.StockStatusAvailable,
		Pricing:     `[{"monthly_rent": 100.0, "deposit": 500.0}]`,
	})

	orderID := uuid.New().String()
	db.Create(&models.Order{
		ID:        orderID,
		TenantID:  tenantID,
		OrgID:     orgID,
		UserID:    userID,
		Status:    models.OrderStatusReserved,
		StartDate: strPtr("2026-06-01"),
		EndDate:   strPtr("2026-07-01"),
	})

	db.Create(&models.LeaseSession{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		OrderID:      orderID,
		UserID:       userID,
		InstrumentID: instrumentID,
		Status:       models.LeaseStatusActive,
	})

	actor := testutil.MakeSiteMember(tenantID, orgID, userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		ctx = context.WithValue(ctx, middleware.ContextKeyGid, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	warehouseHandler := NewWarehouseHandler()
	router.PUT("/api/warehouse/orders/:id/shipping", warehouseHandler.UpdateShipping)
	router.PUT("/api/warehouse/orders/:id/delivery", warehouseHandler.ConfirmDelivery)
	router.PUT("/api/warehouse/orders/:id/return-inspect", warehouseHandler.InspectReturn)

	router.POST("/api/orders/:id/pay", PayOrder)
	router.POST("/api/orders/:id/return", ReturnOrder)

	// Step 1: Pay (reserved → paid)
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusPaid)

	// Step 2: Ship (paid → shipped)
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
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusShipped)
	assert.True(t, testutil.AssertStateHistoryContains(t, orderID, models.OrderStatusPaid, models.OrderStatusShipped))

	// Step 3: Confirm delivery as customer (shipped → in_lease)
	reqBody2 := map[string]interface{}{
		"delivered_at": time.Now().UTC(),
	}
	jsonBody2, _ := json.Marshal(reqBody2)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(jsonBody2))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusInLease)

	// Step 4: Initiate return (in_lease → returning)
	reqBody3 := map[string]interface{}{
		"courier_company": "顺丰快递",
		"tracking_number": "SF87654321",
	}
	jsonBody3, _ := json.Marshal(reqBody3)
	req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(jsonBody3))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusReturning)

	// Step 5: Inspect return with good condition (returning → in_store)
	reqBody4 := map[string]interface{}{
		"instrument_sn": "SN-12345",
		"scan_time":     time.Now().UTC(),
		"condition":     "good",
		"notes":         "完好归还",
	}
	jsonBody4, _ := json.Marshal(reqBody4)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody4))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusInStore)
	assert.True(t, testutil.AssertStateHistoryContains(t, orderID, models.OrderStatusReturning, models.OrderStatusInStore))

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, models.OrderStatusInStore, data["status"])
}

func strPtr(s string) *string {
	return &s
}
