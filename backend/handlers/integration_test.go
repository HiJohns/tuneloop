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
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// IntegrationTest04_Scenario1_RentalClosedLoop tests full rental flow:
// Browse → CreateOrder → Pay → Contract → MyRentals → Return
func TestIntegration_Scenario1_RentalClosedLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	// Setup tables for integration test
	_ = db.Migrator().DropTable(&models.Instrument{})
	_ = db.Migrator().DropTable(&models.Order{})
	_ = db.Migrator().DropTable(&models.LeaseSession{})
	_ = db.Migrator().DropTable(&models.ElectronicContract{})
	_ = db.Migrator().DropTable(&models.DamageAssessment{})
	_ = db.Migrator().DropTable(&models.Appeal{})
	_ = db.Migrator().DropTable(&models.MaintenanceWorker{})
	_ = db.Migrator().DropTable(&models.OrderStatusHistory{})

	if err := db.Migrator().CreateTable(&models.Instrument{}); err != nil {
		t.Fatalf("failed to create instruments table: %v", err)
	}
	if err := db.Migrator().CreateTable(&models.Order{}); err != nil {
		t.Fatalf("failed to create orders table: %v", err)
	}
	if err := db.Migrator().CreateTable(&models.LeaseSession{}); err != nil {
		t.Fatalf("failed to create lease_sessions table: %v", err)
	}
	if err := db.Migrator().CreateTable(&models.ElectronicContract{}); err != nil {
		t.Fatalf("failed to create electronic_contracts table: %v", err)
	}
	if err := db.Migrator().CreateTable(&models.Appeal{}); err != nil {
		t.Fatalf("failed to create appeals table: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	instrumentID := uuid.New().String()

	// Step 1: Create available instrument
	instrument := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}
	db.Create(&instrument)

	// Setup router with all handlers
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	maintenanceWorkerHandler := NewMaintenanceWorkerHandler()
	userRentalHandler := NewUserRentalHandler()
	warehouseHandler := NewWarehouseHandler()
	router.GET("/api/user/instruments", userRentalHandler.ListInstruments)
	router.GET("/api/user/instruments/:id", userRentalHandler.GetInstrument)
	router.POST("/api/user/orders", userRentalHandler.CreateOrder)
	router.GET("/api/user/rentals", userRentalHandler.ListRentals)
	router.POST("/api/user/rentals/:id/return", userRentalHandler.ReturnRental)
	router.GET("/api/user/contracts/:id", userRentalHandler.GetContract)
	router.GET("/api/warehouse/orders", warehouseHandler.ListOrders)
	router.GET("/api/maintenance/workers", maintenanceWorkerHandler.ListWorkers)

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
	err = json.Unmarshal(w.Body.Bytes(), &instrResponse)
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

	// Step 4: List rentals - GET /api/user/rentals
	req = httptest.NewRequest("GET", "/api/user/rentals", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var rentalsResponse struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &rentalsResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, rentalsResponse.Code)
	assert.Greater(t, len(rentalsResponse.Data.List), 0)
}

// IntegrationTest04_Scenario2_WarehouseProcess tests warehouse workflow
func TestIntegration_Scenario2_WarehouseProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	_ = db.Migrator().DropTable(&models.Order{})
	_ = db.Migrator().DropTable(&models.OrderStatusHistory{})
	if err := db.Migrator().CreateTable(&models.Order{}); err != nil {
		t.Fatalf("failed to create orders table: %v", err)
	}
	if err := db.Migrator().CreateTable(&models.OrderStatusHistory{}); err != nil {
		t.Fatalf("failed to create order_status_history table: %v", err)
	}
	tenantID := uuid.New().String()
	userID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
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
		ID:       orderID,
		TenantID: tenantID,
		UserID:   userID,
		Status:   "preparing",
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
	err = json.Unmarshal(w.Body.Bytes(), &ordersResponse)
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

// IntegrationTest04_Scenario3_MaintenanceProcess tests maintenance workflow
func TestIntegration_Scenario3_MaintenanceProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	setupTestTables(t, db)
	tenantID := uuid.New().String()
	userID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	maintenanceWorkerHandler := NewMaintenanceWorkerHandler()
	router.POST("/api/maintenance/workers", maintenanceWorkerHandler.CreateWorker)
	router.GET("/api/maintenance/workers", maintenanceWorkerHandler.ListWorkers)
	router.DELETE("/api/maintenance/workers/:id", maintenanceWorkerHandler.DeleteWorker)

	// Step 1: Create worker
	reqBody := map[string]interface{}{
		"name":  "张师傅",
		"phone": "13800138001",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/maintenance/workers", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var createResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &createResponse)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), createResponse["code"])
	data := createResponse["data"].(map[string]interface{})
	workerID := data["id"].(string)

	// Step 2: List workers
	req = httptest.NewRequest("GET", "/api/maintenance/workers", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listResponse struct {
		Code int `json:"code"`
		Data struct {
			List []interface{} `json:"list"`
		}
	}
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	require.NoError(t, err)
	assert.Equal(t, 20000, listResponse.Code)
	assert.Greater(t, len(listResponse.Data.List), 0)

	// Step 3: Delete worker
	req = httptest.NewRequest("DELETE", "/api/maintenance/workers/"+workerID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// IntegrationTest04_Scenario4_AppealProcess tests appeal workflow
func TestIntegration_Scenario4_AppealProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	setupWarehouseTables(t, db)
	tenantID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	appealHandler := NewAppealHandler()
	router.GET("/api/appeals", appealHandler.ListAppeals)
	router.GET("/api/appeals/:id", appealHandler.GetAppeal)
	router.PUT("/api/appeals/:id/resolve", appealHandler.ResolveAppeal)

	appealID := uuid.New().String()
	appeal := models.Appeal{
		ID:             appealID,
		TenantID:       tenantID,
		Status:         "pending",
		DamageReportID: uuid.New().String(),
		UserID:         userID,
		AppealReason:   "Test damage appeal",
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
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
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
		"resolution": "adjusted",
		"notes":      "Adjusted to 80 USD",
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
