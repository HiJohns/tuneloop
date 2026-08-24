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
	"tuneloop-backend/services/wechatpay"

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
	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, category_id, level, stock_status, images, specifications, pricing, created_at, updated_at) 
		VALUES (?, ?, ?, ?, 'standard', 'available', '[]', '{}', '{}', ?, ?)`,
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
			"deposit_mode":     "ratio",
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
		"deposit_mode":     "ratio",
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

func TestCancelOrderByCustomer_StatusGuard(t *testing.T) {
	wechatpay.InitGlobal(wechatpay.LoadConfig())
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.OrderRefundRecord{}, &models.OrderStatusHistory{}, &models.OrderLog{}))

	tenantID := "00000000-0000-0000-0000-0000000000c1"
	orgID := "00000000-0000-0000-0000-0000000000c2"
	userID := "00000000-0000-0000-0000-0000000000c3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "CNL-" + time.Now().Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   "available",
	}
	require.NoError(t, db.Create(&instrument).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.POST("/orders/:id/cancel-by-user", CancelOrderByCustomer)

	createOrder := func(t *testing.T, status string) string {
		order := models.Order{
			TenantID:     tenantID,
			OrgID:        orgID,
			UserID:       userID,
			InstrumentID: instrument.ID,
			Level:        "standard",
			LeaseTerm:    1,
			Status:       status,
			Deposit:      models.FromYuan(0),
			CashPaid:     models.FromYuan(1),
		}
		require.NoError(t, db.Create(&order).Error)
		return order.ID
	}

	t.Run("paid_cancel_succeeds_with_refund_status", func(t *testing.T) {
		orderID := createOrder(t, models.OrderStatusPaid)

		req := httptest.NewRequest("POST", "/orders/"+orderID+"/cancel-by-user", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, float64(20000), resp["code"])
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "cancelled", data["new_status"])
		// cash_paid=100 → refund amount > 0, status from refund record (MockMode → refunded)
		assert.Equal(t, float64(100), data["refund_amount"])
		assert.Equal(t, "refunded", data["refund_status"])
	})

	t.Run("in_lease_cancel_rejected", func(t *testing.T) {
		orderID := createOrder(t, models.OrderStatusInLease)

		req := httptest.NewRequest("POST", "/orders/"+orderID+"/cancel-by-user", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, float64(40002), resp["code"])
	})

	t.Run("in_transit_cancel_rejected", func(t *testing.T) {
		orderID := createOrder(t, models.OrderStatusInTransit)

		req := httptest.NewRequest("POST", "/orders/"+orderID+"/cancel-by-user", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, float64(40002), resp["code"])
	})

	t.Run("reserved_cancel_succeeds_without_refund", func(t *testing.T) {
		orderID := createOrder(t, models.OrderStatusReserved)

		req := httptest.NewRequest("POST", "/orders/"+orderID+"/cancel-by-user", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, float64(20000), resp["code"])
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "cancelled", data["new_status"])
	})
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
		"deposit_mode":     "ratio",
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

// TestGetOrderLogs_DedupAndOperator verifies #1701: a status-history entry that
// duplicates an explicit order_log event (same transition) is skipped, and the
// order_log operator resolves from OperatorID instead of hardcoded "system".
func TestGetOrderLogs_DedupAndOperator(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, org_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'standard', 1, 0, 0, 'in_lease', now(), now())`,
		orderID, tenantID, tenantID, userID, instID).Error)

	// in_lease transition recorded BOTH in history and order_logs.
	db.Exec(`INSERT INTO order_status_history (id, tenant_id, order_id, status_from, status_to, changed_by, changed_at, created_at, updated_at)
		VALUES (?, ?, ?, 'shipped', 'in_lease', ?, now(), now(), now())`,
		uuid.New().String(), tenantID, orderID, userID)
	db.Exec(`INSERT INTO order_logs (id, order_id, event, operator_id, operator_name, created_at)
		VALUES (?, ?, '已签收，租赁开始', ?, '王五', now())`,
		uuid.New().String(), orderID, userID)

	// A non-duplicated transition (paid→pending_shipment) must still appear.
	db.Exec(`INSERT INTO order_status_history (id, tenant_id, order_id, status_from, status_to, changed_by, changed_at, created_at, updated_at)
		VALUES (?, ?, ?, 'paid', 'pending_shipment', ?, now(), now(), now())`,
		uuid.New().String(), tenantID, orderID, userID)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id/logs", GetOrderLogs)

	req := httptest.NewRequest("GET", "/orders/"+orderID+"/logs?pageSize=50", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Event    string `json:"event"`
				Operator string `json:"operator"`
			} `json:"logs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// "已签收，租赁开始" (order_log) present; bare history in_lease entry skipped.
	hasSigned := false
	hasBareInLease := false
	for _, l := range resp.Data.List {
		if l.Event == "已签收，租赁开始" {
			hasSigned = true
			if l.Operator == "system" {
				t.Errorf("order_log operator must resolve from OperatorID, not hardcoded 'system'")
			}
			if l.Operator == "" {
				t.Errorf("order_log operator must not be empty")
			}
		}
		if l.Event == "in_lease" {
			hasBareInLease = true
		}
	}
	if !hasSigned {
		t.Error("expected order_log 已签收，租赁开始 in timeline")
	}
	if hasBareInLease {
		t.Error("bare history in_lease entry must be deduped when order_log exists (#1701)")
	}
}

// TestGetOrder_DamagePanel verifies #1707: a pending_damage_response order
// detail returns the damage object (amount, description, photos, fee preview
// with refund = paid - damage - rent - shipping).
func TestGetOrder_DamagePanel(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, org_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, shipping_fee, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'standard', 1, 0, 1000, 'pending_damage_response', 50, now(), now())`,
		orderID, tenantID, tenantID, userID, instID).Error)

	damageAmt := 200.0
	reportID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO damage_reports (id, tenant_id, org_id, lease_id, instrument_id, user_id, damage_amount, damage_description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '乐器刮痕', 'pending', now(), now())`,
		reportID, tenantID, tenantID, orderID, instID, userID, damageAmt).Error)
	// Staff photos: instrument_media receiving batch (authoritative source, #1708).
	require.NoError(t, db.Exec(`INSERT INTO instrument_media (id, tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, 'receiving', 'a.jpg', 'image', '/uploads/media/a.jpg', false, 0, now())`,
		uuid.New().String(), tenantID, tenantID, instID, uuid.New().String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO instrument_media (id, tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, 'receiving', 'b.jpg', 'image', '/uploads/media/b.jpg', false, 1, now())`,
		uuid.New().String(), tenantID, tenantID, instID, uuid.New().String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO order_payment_records (id, tenant_id, user_id, order_id, order_type, out_trade_no, amount, type, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'rent', ?, 1500, 'payment', 'paid', now(), now())`,
		uuid.New().String(), tenantID, userID, orderID, "rent"+uuid.NewString()[:8]).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest("GET", "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
			Damage struct {
				ReportID       string   `json:"report_id"`
				DamageAmount   float64  `json:"damage_amount"`
				Description    string   `json:"description"`
				Status         string   `json:"status"`
				Photos         []string `json:"photos"`
				ActualRentDays int      `json:"actual_rent_days"`
				ShippingFee    float64  `json:"shipping_fee"`
				Deposit        float64  `json:"deposit"`
				PaidTotal      float64  `json:"paid_total"`
				Refund         float64  `json:"refund"`
			} `json:"damage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "pending_damage_response", resp.Data.Status)

	d := resp.Data.Damage
	require.Equal(t, reportID, d.ReportID, "damage.report_id must reference the damage report")
	require.Equal(t, 200.0, d.DamageAmount)
	require.Equal(t, "乐器刮痕", d.Description)
	require.Equal(t, "pending", d.Status)
	require.Len(t, d.Photos, 2, "damage.photos must come from instrument_media receiving batch (#1708)")
	require.Equal(t, 50.0, d.ShippingFee)
	require.Equal(t, 1000.0, d.Deposit)
	require.Equal(t, 1500.0, d.PaidTotal)
	// refund = paid(1500) - damage(200) - rent(0, no settlement/breakdown) - shipping(50) = 1250
	require.Equal(t, 1250.0, d.Refund, "refund = paid - damage - rent - shipping")
}

func TestGetOrder_DamagePanel_FromDamageReport(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, org_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, shipping_fee, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'standard', 1, 0, 1000, 'pending_damage_response', 50, now(), now())`,
		orderID, tenantID, tenantID, userID, instID).Error)

	// Damage data lives in damage_reports (post-migration).
	require.NoError(t, db.Exec(`INSERT INTO damage_reports (id, tenant_id, org_id, lease_id, instrument_id, user_id, condition, damage_description, damage_amount, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'damaged', '弦断了', 100, 'pending', now(), now())`,
		uuid.New().String(), tenantID, tenantID, orderID, instID, userID).Error)
	// Staff photos live in instrument_media (receiving batch) — must fall back.
	require.NoError(t, db.Exec(`INSERT INTO instrument_media (id, tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, 'receiving', 'r1.jpg', 'image', '/uploads/media/staff_photo.webp', false, 0, now())`,
		uuid.New().String(), tenantID, tenantID, instID, uuid.New().String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO order_payment_records (id, tenant_id, user_id, order_id, order_type, out_trade_no, amount, type, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'rent', ?, 1500, 'payment', 'paid', now(), now())`,
		uuid.New().String(), tenantID, userID, orderID, "rent"+uuid.NewString()[:8]).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest("GET", "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Damage struct {
				DamageAmount float64  `json:"damage_amount"`
				Description  string   `json:"description"`
				Status       string   `json:"status"`
				Photos       []string `json:"photos"`
				Refund       float64  `json:"refund"`
			} `json:"damage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	d := resp.Data.Damage
	require.Equal(t, 100.0, d.DamageAmount, "damage_amount from damage_reports")
	require.Equal(t, "弦断了", d.Description)
	require.Equal(t, "pending", d.Status)
	require.Len(t, d.Photos, 1)
	require.Equal(t, "/uploads/media/staff_photo.webp", d.Photos[0], "photos from instrument_media receiving batch")
	require.Equal(t, 1350.0, d.Refund, "refund = 1500 - 100 - 0(rent) - 50")
}
