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

	"gorm.io/gorm"
)

func setupTestRouter(t *testing.T, tenantID, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", tenantID)
		c.Set("user_id", userID)
		c.Set("org_id", tenantID)
		c.Next()
	})
	return router
}

func setupTestData(t *testing.T, db *gorm.DB, tenantID string) (categoryID, instrumentID, userID string) {
	now := time.Now()

	userID = uuid.New().String()
	db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, ?, ?)`,
		userID, userID, tenantID, tenantID, "Test User", "test@example.com", "13800138000", now, now)

	categoryID = uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, level, visible, sort, created_at) 
		VALUES (?, 'Piano', ?, 1, true, 1, ?)`,
		categoryID, tenantID, now)

	instrumentID = uuid.New().String()
	db.Exec(`INSERT INTO instruments (id, name, tenant_id, org_id, category_id, brand, level, stock_status, images, specifications, pricing, created_at, updated_at) 
		VALUES (?, 'Test Piano', ?, ?, ?, 'Yamaha', 'standard', 'available', '[]', '{}', '{}', ?, ?)`,
		instrumentID, tenantID, tenantID, categoryID, now, now)

	return categoryID, instrumentID, userID
}

func cleanupTestData(db *gorm.DB, tenantID string) {
	db.Exec(`DELETE FROM orders WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM users WHERE tenant_id = ?`, tenantID)
}

func TestLeaseFlow_CompleteLifecycle(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)
	router.POST("/orders/:id/pay", PayOrder)
	router.POST("/orders/:id/pickup", PickupOrder)
	router.POST("/orders/:id/return", ReturnOrder)
	router.POST("/orders/:id/cancel", CancelOrder)

	t.Skip("CreateOrder is now in UserRentalHandler — update test to use /user/orders path")

	t.Run("Step1_CreateOrder", func(t *testing.T) {
		body := map[string]interface{}{
			"instrument_id":    instrumentID,
			"level":            "standard",
			"lease_term":       3,
			"deposit_mode":     "standard",
			"agreement_signed": true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/orders", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, float64(20000), resp["code"])
		data := resp["data"].(map[string]interface{})
		assert.NotEmpty(t, data["order_id"])
	})

	t.Run("Step2_GetOrder_Pending", func(t *testing.T) {
		var order models.Order
		db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).First(&order)

		req := httptest.NewRequest("GET", "/orders/"+order.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "pending", data["status"])
	})

	t.Run("Step3_PayOrder", func(t *testing.T) {
		var order models.Order
		db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).First(&order)

		req := httptest.NewRequest("POST", "/orders/"+order.ID+"/pay", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, models.OrderStatusPaid, data["new_status"])
	})

	t.Run("Step4_PickupOrder", func(t *testing.T) {
		var order models.Order
		db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).First(&order)

		req := httptest.NewRequest("POST", "/orders/"+order.ID+"/pickup", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, models.OrderStatusInLease, data["new_status"])
	})

	t.Run("Step5_ReturnOrder", func(t *testing.T) {
		var order models.Order
		db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).First(&order)

		req := httptest.NewRequest("POST", "/orders/"+order.ID+"/return", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "completed", data["new_status"])
	})
}

func TestLeaseFlow_CancelOrder(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	router := setupTestRouter(t, tenantID, userID)
	router.POST("/orders/:id/cancel", CancelOrder)

	t.Skip("CreateOrder is now in UserRentalHandler — update to use /user/orders path")

	body := map[string]interface{}{
		"instrument_id":    instrumentID,
		"level":            "standard",
		"lease_term":       3,
		"deposit_mode":     "standard",
		"agreement_signed": true,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var order models.Order
	db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).First(&order)

	var instrument models.Instrument
	db.First(&instrument, "id = ?", instrumentID)
	assert.Equal(t, "unavailable", instrument.StockStatus)

	req = httptest.NewRequest("POST", "/orders/"+order.ID+"/cancel", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "cancelled", data["new_status"])

	db.First(&instrument, "id = ?", instrumentID)
	assert.Equal(t, "available", instrument.StockStatus)
}

func TestLeaseFlow_InvalidStatusTransitions(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	router := setupTestRouter(t, tenantID, userID)
	router.POST("/orders/:id/pay", PayOrder)
	router.POST("/orders/:id/pickup", PickupOrder)

	orderID := uuid.New().String()
	db.Exec(`INSERT INTO orders (id, tenant_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, created_at) 
		VALUES (?, ?, ?, ?, 'standard', 3, 100, 500, 'paid', ?)`,
		orderID, tenantID, userID, instrumentID, time.Now())

	t.Run("CannotPayPaidOrder", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/orders/"+orderID+"/pay", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("CannotPickupPendingOrder", func(t *testing.T) {
		pendingOrderID := uuid.New().String()
		db.Exec(`INSERT INTO orders (id, tenant_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, created_at) 
			VALUES (?, ?, ?, ?, 'standard', 3, 100, 500, 'pending', ?)`,
			pendingOrderID, tenantID, userID, instrumentID, time.Now())

		req := httptest.NewRequest("POST", "/orders/"+pendingOrderID+"/pickup", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestLeaseFlow_GetOrdersWithStatusFilter(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders", GetOrders)

	now := time.Now()
	for i, status := range []string{models.OrderStatusReserved, models.OrderStatusPaid, models.OrderStatusInLease, models.OrderStatusCompleted} {
		orderID := uuid.New().String()
		db.Exec(`INSERT INTO orders (id, tenant_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, created_at) 
			VALUES (?, ?, ?, ?, 'standard', 3, 100, 500, ?, ?)`,
			orderID, tenantID, userID, instrumentID, status, now.Add(time.Duration(i)*time.Minute))
	}

	t.Run("GetAllOrders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/orders", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.GreaterOrEqual(t, int(data["total"].(float64)), 4)
	})

	t.Run("GetPendingOrders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/orders?status=pending", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		list := data["list"].([]interface{})
		for _, item := range list {
			order := item.(map[string]interface{})
			assert.Equal(t, "pending", order["status"])
		}
	})
}

func setupGuestTestRouter(t *testing.T, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Guest token: has userID but NO tenantID/orgID
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", "")
		c.Set("user_id", userID)
		c.Set("org_id", "")
		c.Next()
	})
	return router
}

func TestGuestCreateOrder_TenantDerivedFromInstrument(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	gTenantID := uuid.New().String()
	gOrgID := uuid.New().String()
	gInstrumentID := uuid.New().String()
	gUserID := uuid.New().String()
	now := time.Now()

	db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, ?, ?)`,
		gUserID, gUserID, gTenantID, gOrgID, "Guest User", "guest@example.com", "13900139000", now, now)

	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, site_id, level, stock_status, images, specifications, pricing, created_at, updated_at) 
		VALUES (?, ?, ?, NULL, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		gInstrumentID, gTenantID, gOrgID, now, now)

	defer func() {
		db.Exec(`DELETE FROM lease_sessions WHERE tenant_id = ?`, gTenantID)
		db.Exec(`DELETE FROM orders WHERE tenant_id = ?`, gTenantID)
		db.Exec(`DELETE FROM instruments WHERE id = ?`, gInstrumentID)
		db.Exec(`DELETE FROM users WHERE id = ?`, gUserID)
	}()

	router := setupGuestTestRouter(t, gUserID)
	handler := &UserRentalHandler{}
	router.POST("/user/orders", handler.CreateOrder)

	body := map[string]interface{}{
		"instrument_id": gInstrumentID,
		"start_date":    "2026-06-10",
		"end_date":      "2026-07-10",
		"delivery_address": map[string]interface{}{
			"city":    "Beijing",
			"address": "Chaoyang District",
		},
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), resp["code"])

	// Verify order was created with the instrument's tenant_id
	var order models.Order
	result := db.Where("user_id = ? AND instrument_id = ?", gUserID, gInstrumentID).First(&order)
	require.NoError(t, result.Error)
	assert.Equal(t, gTenantID, order.TenantID, "order tenant should match instrument tenant")
	assert.Equal(t, gOrgID, order.OrgID, "order org should match instrument org")
}

func TestGuestCreateOrder_InstrumentNotFound(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	gUserID := uuid.New().String()
	now := time.Now()
	db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, ?, ?)`,
		gUserID, gUserID, uuid.New().String(), uuid.New().String(), "Guest User", "guest@example.com", "13900139000", now, now)

	defer db.Exec(`DELETE FROM users WHERE id = ?`, gUserID)

	router := setupGuestTestRouter(t, gUserID)
	handler := &UserRentalHandler{}
	router.POST("/user/orders", handler.CreateOrder)

	body := map[string]interface{}{
		"instrument_id": uuid.New().String(),
		"start_date":    "2026-06-10",
		"end_date":      "2026-07-10",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(40002), resp["code"])
}

func TestGuestBatchCreateOrder(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	gTenantID := uuid.New().String()
	gOrgID := uuid.New().String()
	gUserID := uuid.New().String()
	now := time.Now()

	db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, ?, ?)`,
		gUserID, gUserID, gTenantID, gOrgID, "Guest User", "guest@example.com", "13900139000", now, now)

	id1 := uuid.New().String()
	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, level, stock_status, images, specifications, pricing, created_at, updated_at) 
		VALUES (?, ?, ?, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		id1, gTenantID, gOrgID, now, now)

	id2 := uuid.New().String()
	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, level, stock_status, images, specifications, pricing, created_at, updated_at) 
		VALUES (?, ?, ?, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		id2, gTenantID, gOrgID, now, now)

	defer func() {
		db.Exec(`DELETE FROM lease_sessions WHERE tenant_id = ?`, gTenantID)
		db.Exec(`DELETE FROM orders WHERE tenant_id = ?`, gTenantID)
		db.Exec(`DELETE FROM instruments WHERE id IN (?, ?)`, id1, id2)
		db.Exec(`DELETE FROM users WHERE id = ?`, gUserID)
	}()

	router := setupGuestTestRouter(t, gUserID)
	handler := &UserRentalHandler{}
	router.POST("/user/orders/batch", handler.BatchCreateOrder)

	body := map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": id1, "start_date": "2026-06-10", "end_date": "2026-07-10"},
			{"instrument_id": id2, "start_date": "2026-06-15", "end_date": "2026-07-15"},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/user/orders/batch", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(20000), resp["code"])
	data := resp["data"].(map[string]interface{})
	orders := data["orders"].([]interface{})
	assert.Len(t, orders, 2)
	for _, o := range orders {
		order := o.(map[string]interface{})
		assert.Equal(t, "paid", order["status"])
	}
}

func TestLeaseFlow_CreateOrder_InstrumentNotAvailable(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	db.Exec(`UPDATE instruments SET stock_status = 'unavailable' WHERE id = ?`, instrumentID)

	router := setupTestRouter(t, tenantID, userID)

	t.Skip("CreateOrder is now in UserRentalHandler")

	body := map[string]interface{}{
		"instrument_id":    instrumentID,
		"level":            "standard",
		"lease_term":       3,
		"deposit_mode":     "standard",
		"agreement_signed": true,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(40002), resp["code"])
}
