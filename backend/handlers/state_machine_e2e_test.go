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

	_ = db.Migrator().DropTable(&models.Instrument{}, &models.Order{}, &models.LeaseSession{}, &models.OrderStatusHistory{}, &models.DamageAssessment{}, &models.Notification{}, &models.OrderPaymentRecord{}, &models.Settlement{}, &models.OrderRefundRecord{}, &models.PointsTransaction{}, &models.User{}, &models.DamageReport{})
	require.NoError(t, db.Migrator().CreateTable(&models.Instrument{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Order{}))
	require.NoError(t, db.Migrator().CreateTable(&models.LeaseSession{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderStatusHistory{}))
	require.NoError(t, db.Migrator().CreateTable(&models.DamageAssessment{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Notification{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderPaymentRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Settlement{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderRefundRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.PointsTransaction{}))
	require.NoError(t, db.Migrator().AutoMigrate(&models.User{}))
	require.NoError(t, db.Migrator().CreateTable(&models.DamageReport{}))
	// iam_sub is excluded from migration (-:migration tag); add manually
	if !db.Migrator().HasColumn(&models.User{}, "iam_sub") {
		require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN iam_sub varchar(255)`).Error)
	}

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

// TestInspectReturn_OverdueFee verifies #1493: late return inspection returns
// overdue_days/overdue_fee and charges them once at return.
func TestInspectReturn_OverdueFee(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()

	// Build an overdue order directly in returning status.
	orderID := uuid.New().String()
	oldEnd := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	pricing := `{"overdue_daily_fee":15,"daily_rent":10}`
	require.NoError(t, db.Create(&models.Order{
		ID:               orderID,
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrumentID,
		StartDate:        strPtr(start),
		EndDate:          strPtr(oldEnd),
		LeaseTerm:        20,
		Status:           models.OrderStatusReturning,
		Deposit:          500,
		PricingBreakdown: strPtr(`{"base_daily_rent":10}`),
	}).Error)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", instrumentID).Update("pricing", pricing).Error)

	// Scan at today → overdue days = endDate+1 .. today = 10 days
	reqBody := map[string]interface{}{
		"instrument_sn": "TEST-SN",
		"scan_time":     time.Now(),
		"condition":     "good",
		"notes":         "overdue return test",
		"photos":        []string{},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			OverdueDays      int     `json:"overdue_days"`
			OverdueDailyRate float64 `json:"overdue_daily_rate"`
			OverdueFee       float64 `json:"overdue_fee"`
		} `json:"data"`
	}
	t.Logf("RESP BODY: %s", w.Body.String())
	var dbOrder models.Order
	require.NoError(t, db.First(&dbOrder, "id = ?", orderID).Error)
	t.Logf("DB end_date=%v status=%v", *dbOrder.EndDate, dbOrder.Status)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 10, resp.Data.OverdueDays, "overdue days = endDate+1 to today = 10")
	require.Equal(t, 15.0, resp.Data.OverdueDailyRate, "rate from instrument pricing overdue_daily_fee")
	require.Equal(t, 150.0, resp.Data.OverdueFee, "fee = 15 × 10")
}

// TestInspectReturn_NoOverdue verifies on-time return has zero overdue fee.
func TestInspectReturn_NoOverdue(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()

	orderID := uuid.New().String()
	futureEnd := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	require.NoError(t, db.Create(&models.Order{
		ID:               orderID,
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrumentID,
		StartDate:        strPtr(start),
		EndDate:          strPtr(futureEnd),
		LeaseTerm:        15,
		Status:           models.OrderStatusReturning,
		Deposit:          500,
		PricingBreakdown: strPtr(`{"base_daily_rent":10}`),
	}).Error)

	reqBody := map[string]interface{}{
		"instrument_sn": "TEST-SN2",
		"scan_time":     time.Now(),
		"condition":     "good",
		"photos":        []string{},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			OverdueDays int     `json:"overdue_days"`
			OverdueFee  float64 `json:"overdue_fee"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Data.OverdueDays, "on-time return has no overdue days")
	require.Equal(t, 0.0, resp.Data.OverdueFee)
}

// TestInspectReturn_Good_ExecutesRefund verifies that a good-condition
// return inspection auto-executes the settlement refund (#1530):
// prepaid points refunded, deposit refunded, settlement record created.
func TestInspectReturn_Good_ExecutesRefund(t *testing.T) {
	router, tenantID, userID, orgID, instrumentID := setupE2ETestEnv(t)
	if router == nil {
		return
	}
	db := database.GetDB()

	// User row with prepaid points (required for points refund)
	require.NoError(t, db.Create(&models.User{
		ID:             userID,
		TenantID:       tenantID,
		OrgID:          orgID,
		Username:       "refund_test_user",
		PrepaidPoints:  1000,
		Status:         "active",
	}).Error)

	orderID := uuid.New().String()
	start := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	end := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	now := time.Now()
	require.NoError(t, db.Create(&models.Order{
		ID:                orderID,
		TenantID:          tenantID,
		OrgID:             orgID,
		UserID:            userID,
		InstrumentID:      instrumentID,
		StartDate:         strPtr(start),
		EndDate:           strPtr(end),
		LeaseTerm:         12,
		Status:            models.OrderStatusReturning,
		Deposit:           500,
		CashPaid:          300,
		PrepaidPointsUsed: 200,
		ReturnedAt:        &now,
		PricingBreakdown:  strPtr(`{"base_daily_rent":100,"final_daily_rent":100,"total_amount":1200,"tier_segments":[{"days":12,"rate":100,"tier":1,"discount":1,"subtotal":1200}]}`),
	}).Error)

	// Payment record for the cash portion
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     &orgID,
		UserID:    userID,
		OrderID:   &orderID,
		OrderType: "rent",
		Amount:    300,
		Type:      "payment",
		Status:    "paid",
		Method:    strPtr("mock"),
	}).Error)

	reqBody := map[string]interface{}{
		"instrument_sn": "REFUND-SN",
		"scan_time":     time.Now(),
		"condition":     "good",
		"photos":        []string{},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Settlement record created
	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&settlement).Error)
	require.GreaterOrEqual(t, settlement.ActualRentDays, 1, "actual days >= 1")
	require.Equal(t, float64(settlement.ActualRentDays)*100, settlement.ActualRentAmount, "rent = actual days × 100")
	require.Equal(t, 200.0, settlement.PrepaidRefunded, "prepaid points refunded first (order used 200)")
	require.True(t, settlement.CashRefundable > 0, "cash refund for remaining deposit + rent overpayment")

	// Order marked deposit refunded
	var order models.Order
	require.NoError(t, db.First(&order, "id = ?", orderID).Error)
	require.True(t, order.DepositRefunded, "deposit_refunded set after refund")

	// User prepaid points increased by the refunded amount
	var user models.User
	require.NoError(t, db.First(&user, "id = ?", userID).Error)
	require.Equal(t, 1200.0, user.PrepaidPoints, "1000 + 200 prepaid refunded")
	// Idempotent: second inspect attempt must not double-refund
	req2 := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusBadRequest, w2.Code, "order no longer in returning status")
}
