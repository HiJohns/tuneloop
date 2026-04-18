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
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupUserRentalTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.Instrument{},
		&models.Order{},
		&models.LeaseSession{},
		&models.ElectronicContract{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestListUserInstruments(t *testing.T) {
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
	categoryID := uuid.New().String()
	siteID := uuid.New().String()

	// Create test instruments with JSON pricing
	siteUUID := uuid.MustParse(siteID)
	instrument1 := models.Instrument{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		CategoryID:  &categoryID,
		SiteID:      &siteUUID,
		LevelID:     nil,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.5, "weekly_rent": 73.5, "monthly_rent": 252.0, "deposit": 500.0}]`,
	}
	db.Create(&instrument1)

	instrument2 := models.Instrument{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		CategoryID:  &categoryID,
		SiteID:      &siteUUID,
		LevelID:     nil,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 15.0, "weekly_rent": 105.0, "monthly_rent": 360.0, "deposit": 750.0}]`,
	}
	db.Create(&instrument1)
	db.Create(&instrument2)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/instruments", handler.ListInstruments)

	req := httptest.NewRequest("GET", "/api/user/instruments?page=1&pageSize=10&sort=price", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List     []map[string]interface{} `json:"list"`
			Total    int64                    `json:"total"`
			Page     int                      `json:"page"`
			PageSize int                      `json:"pageSize"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, int64(2), response.Data.Total)
	assert.Equal(t, 2, len(response.Data.List))
}

func TestGetUserInstrumentDetail(t *testing.T) {
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
	instrumentID := uuid.New().String()
	instrument := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: "available",

		Pricing: `[{"daily_rent": 12.5, "weekly_rent": 87.5, "monthly_rent": 300.0, "deposit": 600.0}]`,
	}
	db.Create(&instrument)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/instruments/:id", handler.GetInstrument)

	req := httptest.NewRequest("GET", "/api/user/instruments/"+instrumentID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, instrumentID, response.Data["id"])
	assert.Equal(t, 12.5, response.Data["daily_rent"])
	assert.Equal(t, 600.0, response.Data["deposit"])
}

func TestCreateRentalOrder(t *testing.T) {
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

	// Create available instrument
	instrument := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: "available",

		Pricing: `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}
	db.Create(&instrument)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders", handler.CreateOrder)

	reqBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-15",
		"delivery_address": map[string]string{
			"street": "123 Music St",
			"city":   "Music City",
		},
		"notes": "Please deliver in the morning",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, "success", response.Message)
	assert.NotEmpty(t, response.Data["order_id"])
	assert.NotEmpty(t, response.Data["lease_id"])
	assert.Equal(t, 140.0, response.Data["amount"]) // 10 * 14 days
}

func TestListUserRentals(t *testing.T) {
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

	startDate := time.Now()
	endDate := startDate.AddDate(0, 1, 0)

	leaseSession := models.LeaseSession{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		InstrumentID: instrumentID,
		StartDate:    startDate,
		EndDate:      endDate,
		Status:       "active",
	}
	db.Create(&leaseSession)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/rentals", handler.ListRentals)

	req := httptest.NewRequest("GET", "/api/user/rentals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, 1, len(response.Data.List))
}

func TestReturnRental(t *testing.T) {
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
	leaseID := uuid.New().String()

	leaseSession := models.LeaseSession{
		ID:           leaseID,
		TenantID:     tenantID,
		UserID:       userID,
		InstrumentID: uuid.New().String(),
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(0, 1, 0),
		Status:       "active",
	}
	db.Create(&leaseSession)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/rentals/:id/return", handler.ReturnRental)

	reqBody := map[string]interface{}{
		"return_method":   "courier",
		"return_tracking": "SF654321",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/user/rentals/"+leaseID+"/return", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, "return_requested", response.Data["status"])
	assert.Equal(t, "courier", response.Data["return_method"])
}

func TestGetElectronicContract(t *testing.T) {
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
	contractID := uuid.New().String()

	contract := models.ElectronicContract{
		ID:             contractID,
		TenantID:       tenantID,
		UserID:         userID,
		OrderID:        uuid.New().String(),
		InstrumentID:   uuid.New().String(),
		ContractURL:    "https://contracts.example.com/" + contractID,
		ContractNumber: "CTR-" + contractID[:8],
		Status:         "signed",
		GeneratedAt:    time.Now(),
	}
	db.Create(&contract)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/contracts/:id", handler.GetContract)

	req := httptest.NewRequest("GET", "/api/user/contracts/"+contractID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, contractID, response.Data["id"])
	assert.Equal(t, "signed", response.Data["status"])
}
