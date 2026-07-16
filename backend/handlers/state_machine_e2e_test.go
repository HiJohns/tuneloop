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
	"gorm.io/gorm"
)

func setupE2ETestEnv(t *testing.T) (*gin.Engine, string, string, string, string) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil, "", "", "", ""
	}
	database.SetDB(db)

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

	instrumentID := uuid.New().String()
	db.Create(&models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		StockStatus: models.StockStatusAvailable,
		Pricing:     `[{"monthly_rent": 100.0, "deposit": 500.0}]`,
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

	return router, tenantID, userID, orgID, instrumentID
}

func createTestOrder(t *testing.T, db *gorm.DB, tenantID, orgID, userID, instrumentID string) string {
	orderID := uuid.New().String()
	db.Create(&models.Order{
		ID:        orderID,
		TenantID:  tenantID,
		OrgID:     orgID,
		UserID:    userID,
		InstrumentID: instrumentID,
		Status:    models.OrderStatusReserved,
		StartDate: strPtr("2026-06-01"),
		EndDate:   strPtr("2026-07-01"),
	})

	db.Create(&models.LeaseSession{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        stringPtr(orgID),
		OrderID:      orderID,
		UserID:       userID,
		InstrumentID: instrumentID,
		Status:       models.LeaseStatusActive,
	})
	return orderID
}

func TestScenarioA_StandardClosedLoop(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()
	orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)

	t.Run("A3_Pay", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusPaid)
	})

	t.Run("A4_Ship", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"tracking_number": "SF12345678",
			"company":         "顺丰快递",
			"shipped_at":      time.Now().UTC(),
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusShipped)
		assert.True(t, testutil.AssertStateHistoryContains(t, orderID, models.OrderStatusPaid, models.OrderStatusShipped))
	})

	t.Run("A5_ConfirmDelivery", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"delivered_at": time.Now().UTC(),
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusInLease)
	})

	t.Run("A6_Return", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"courier_company": "顺丰快递",
			"tracking_number": "SF87654321",
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusReturning)
	})

	t.Run("A7_InspectGood", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"instrument_sn": "SN-12345",
			"scan_time":     time.Now().UTC(),
			"condition":     "good",
			"notes":         "完好归还",
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusCompleted)
		assert.True(t, testutil.AssertStateHistoryContains(t, orderID, models.OrderStatusReturning, models.OrderStatusCompleted))

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, models.OrderStatusCompleted, data["status"])
	})
}

func TestScenarioC_CancelBoundary(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()

	t.Run("C1_CancelReserved", func(t *testing.T) {
		orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)
		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/cancel", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusCancelled)
	})

	t.Run("C2_CancelPaid", func(t *testing.T) {
		orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)
		payReq := httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, payReq)
		require.Equal(t, http.StatusOK, w.Code)

		cancelReq := httptest.NewRequest("POST", "/api/orders/"+orderID+"/cancel", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, cancelReq)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusCancelled)
	})

	t.Run("C3_CannotCancelShipped", func(t *testing.T) {
		orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)
		step(t, router, orderID, "pay")
		stepShip(t, router, orderID)

		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/cancel", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("C4_CannotShipFromAvailable", func(t *testing.T) {
		orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)
		reqBody := map[string]interface{}{
			"tracking_number": "SF12345678",
			"company":         "顺丰快递",
			"shipped_at":      time.Now().UTC(),
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestScenarioA_DamageVariant(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()
	orderID := createTestOrder(t, db, tenantID, orgID, userID, instrumentID)

	step(t, router, orderID, "pay")
	stepShip(t, router, orderID)
	stepDeliver(t, router, orderID)
	stepReturn(t, router, orderID)

	t.Run("A7_InspectDamaged", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"instrument_sn": "SN-12345",
			"scan_time":     time.Now().UTC(),
			"condition":     "damaged",
			"notes":         "琴颈断裂",
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		testutil.AssertState(t, orderID, models.OrderStatusCompleted)
	})
}

func step(t *testing.T, router *gin.Engine, orderID, action string) {
	var req *http.Request
	switch action {
	case "pay":
		req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
	case "return":
		body := map[string]interface{}{
			"courier_company": "顺丰快递",
			"tracking_number": "SF87654321",
		}
		jsonBody, _ := json.Marshal(body)
		req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func stepShip(t *testing.T, router *gin.Engine, orderID string) {
	reqBody := map[string]interface{}{
		"tracking_number": "SF12345678",
		"company":         "顺丰快递",
		"shipped_at":      time.Now().UTC(),
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func stepDeliver(t *testing.T, router *gin.Engine, orderID string) {
	reqBody := map[string]interface{}{
		"delivered_at": time.Now().UTC(),
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func stepReturn(t *testing.T, router *gin.Engine, orderID string) {
	reqBody := map[string]interface{}{
		"courier_company": "顺丰快递",
		"tracking_number": "SF87654321",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
