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
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestLeaseEarlyReturn covers §2.7 early-return refund end-to-end:
// full lease flow with return BEFORE the lease end → rebate on unused
// days. Delivery 30d ago, return at day 28 → 2 unused days rebated.
func TestLeaseEarlyReturn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("00000000e1f2")
	instrumentID := uuid.New().String()

	// Instrument: daily 100, deposit 500. Lease 30 days → rent 3000.
	baseRate := 100.0
	require.NoError(t, db.Create(&models.Instrument{
		ID:            instrumentID,
		TenantID:      tenantID,
		OrgID:         &orgID,
		StockStatus:   models.StockStatusAvailable,
		BaseDailyRate: &baseRate,
		Pricing:       `{"daily_rent":100.0,"monthly_rent":3000.0,"deposit":500.0,"overdue_daily_fee":0}`,
	}).Error)

	require.NoError(t, db.Create(&models.PricingTemplate{
		ID:              uuid.New().String(),
		Code:            "default",
		Name:            "Default",
		IsSystemDefault: true,
		IsActive:        true,
		ConfigSchema:    `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20}],"deposit_ratio":0}`,
	}).Error)

	// Customer (guest) + staff actors.
	customer := testutil.MakeCustomer("", userID)
	staff := testutil.MakeSiteMember(tenantID, orgID, userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		if c.Request.URL.Path == "/api/user/orders" {
			ctx = customer.InjectContext(ctx)
		} else {
			ctx = staff.InjectContext(ctx)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	userRentalHandler := NewUserRentalHandler()
	warehouseHandler := NewWarehouseHandler()
	router.POST("/api/user/orders", userRentalHandler.CreateOrder)
	router.POST("/api/orders/:id/pay", PayOrder)
	router.PUT("/api/warehouse/orders/:id/shipping", warehouseHandler.UpdateShipping)
	router.PUT("/api/warehouse/orders/:id/delivery", warehouseHandler.ConfirmDelivery)
	router.POST("/api/orders/:id/return", ReturnOrder)
	router.PUT("/api/warehouse/orders/:id/return-inspect", warehouseHandler.InspectReturn)

	// Delivery 27 days ago: lease runs now-27 .. now+3 (30 days). Return
	// happens today → 28 actual days (CalculateDays is inclusive of both
	// endpoints), 2 unused days rebated.
	now := time.Now()
	deliveredAt := now.AddDate(0, 0, -27)
	scanTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Step 1: Create order (30-day lease).
	createBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2026-07-01",
		"end_date":      "2026-07-30",
		"rent_days":     30,
	}
	jsonBody, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "create order: %s", w.Body.String())

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	require.Equal(t, 20000, createResp.Code)
	orderID := createResp.Data.OrderID

	// Step 2: Pay + record payment (mock books directly).
	req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusPaid)

	outTradeNo := "mock" + uuid.New().String()[:20]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrgID:      &orgID,
		UserID:     userID,
		OrderID:    &orderID,
		OrderType:  "rent",
		Amount:     3500,
		Type:       "payment",
		Status:     "paid",
		Method:     strPtr("mock"),
		OutTradeNo: &outTradeNo,
	}).Error)

	// Step 3: Ship.
	shipBody, _ := json.Marshal(map[string]interface{}{
		"tracking_number": "SF-" + uuid.New().String()[:8],
		"company":         "顺丰快递",
		"shipped_at":      time.Now().UTC(),
	})
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(shipBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "ship: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusShipped)

	// Step 4: Deliver (30d ago → lease end today).
	deliverBody, _ := json.Marshal(map[string]interface{}{"delivered_at": deliveredAt})
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(deliverBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "deliver: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusInLease)

	// Step 5: Return.
	returnBody, _ := json.Marshal(map[string]interface{}{
		"courier_company": "顺丰快递",
		"tracking_number": "SF-" + uuid.New().String()[:8],
	})
	req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(returnBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusReturning)

	// Step 6: Inspect good (early return, no overdue).
	inspectBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": "SN-" + uuid.New().String()[:8],
		"scan_time":     scanTime,
		"condition":     "good",
		"photos":        []string{},
	})
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(inspectBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	testutil.AssertState(t, orderID, models.OrderStatusCompleted)

	// Step 7: Settlement — actual days = delivered..scan (28 days),
	// rent payable = 28×100 = 2800, refund = 3000+500-2800 = 700.
	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&settlement).Error)
	require.Equal(t, 28, settlement.ActualRentDays)
	require.Equal(t, 2800.0, settlement.ActualRentAmount)
	require.Equal(t, 0.0, settlement.OverdueChargesTotal)
	require.Equal(t, 700.0, settlement.CashRefundable, "3000 rent + 500 deposit - 2800 payable")

	// Step 8: Refund record booked (mock).
	var refundRecord models.OrderRefundRecord
	require.NoError(t, db.Where("tenant_id = ?", tenantID).Order("created_at desc").First(&refundRecord).Error)
	assert.Equal(t, "refunded", refundRecord.Status)
	assert.Equal(t, 700.0, refundRecord.Amount)

	// Step 9: Instrument returned to available.
	var instrument models.Instrument
	require.NoError(t, db.Where("id = ?", instrumentID).First(&instrument).Error)
	assert.Equal(t, models.StockStatusAvailable, instrument.StockStatus)
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
