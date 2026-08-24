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

// TestSettlementFlow covers the full customer rental closed loop via real
// HTTP handlers (Issue #1563, TC #1546): create order → pay → ship →
// confirm delivery → customer return → staff return-inspect (good) →
// settlement + refund. Verifies the settlement math end-to-end instead of
// calling computeSettlement directly (which the unit tests already cover).
func TestSettlementFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	orgID := tenantID
	instrumentID := uuid.New().String()

	// Instrument: daily 100, deposit 500. Lease 30 days → rent 3000.
	// overdue_daily_fee: 0 keeps the overdue fee at 0 for a return on the
	// end date, so the settlement math stays clean.
	baseRate := 100.0
	require.NoError(t, db.Create(&models.Instrument{
		ID:            instrumentID,
		TenantID:      tenantID,
		OrgID:         &orgID,
		StockStatus:   models.StockStatusAvailable,
		BaseDailyRate: models.ToCentsPtr(&baseRate),
		Pricing:       `{"daily_rent":100.0,"monthly_rent":3000.0,"deposit":500.0,"overdue_daily_fee":0}`,
	}).Error)

	// Default pricing template so CalculatePricing has a config to read:
	// tier 1 = 30 days at 100/day (0% discount), deposit_ratio 0 (use
	// instrument Pricing JSON deposit instead).
	require.NoError(t, db.Create(&models.PricingTemplate{
		ID:              uuid.New().String(),
		Code:            "default",
		Name:            "Default",
		IsSystemDefault: true,
		IsActive:        true,
		ConfigSchema:    `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20}],"deposit_ratio":0}`,
	}).Error)

	// Customer actor with a user row (prepaid points for points tracking).
	require.NoError(t, db.Create(&models.User{
		ID:            userID,
		TenantID:      tenantID,
		OrgID:         orgID,
		Username:      "settlement_flow_user",
		PrepaidPoints: models.Cents(1000),
		Status:        "active",
	}).Error)

	customer := testutil.MakeCustomer("", userID) // guest: no tid, derived from instrument
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

	startDate := "2026-07-01"
	endDate := "2026-07-30" // 30 days inclusive

	// Delivery 30 days ago: UpdateShipping recalculates the lease window from
	// delivered_at (#lease starts at delivery), so delivered 30 days ago with
	// rent_days=30 puts the lease end at today. The return then happens today
	// (on-time, no overdue), making the settlement math deterministic.
	now := time.Now()
	deliveredAt := now.AddDate(0, 0, -30)
	// scan at end_date 00:00:00 → not "after" end date → no overdue fee.
	scanTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// ------------------------------------------------------------------
	// Step 1: Customer creates the order (real handler, real pricing).
	// ------------------------------------------------------------------
	createBody := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    startDate,
		"end_date":      endDate,
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
	require.NotEmpty(t, createResp.Data.OrderID, "order_id must be returned")
	orderID := createResp.Data.OrderID

	// Order is reserved; deposit computed from instrument pricing.
	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	require.Equal(t, models.OrderStatusReserved, order.Status)
	assert.Equal(t, models.Cents(50000), order.Deposit, "deposit from instrument pricing")
	// Upfront payment = rent (30 × 100) + deposit (500) = 3500.
	assert.Equal(t, models.Cents(350000), order.CashPaid, "rent 3000 + deposit 500 (no points, no discount)")

	// ------------------------------------------------------------------
	// Step 2: Customer pays.
	// ------------------------------------------------------------------
	req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/pay", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "pay order: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusPaid)

	// Payment record: the real prepay flow creates this via /api/pay/prepay;
	// this test mirrors the booked outcome so executeRefund finds the record.
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
		Method:     strPtr("jsapi"),
		OutTradeNo: &outTradeNo,
	}).Error)

	// ------------------------------------------------------------------
	// Step 3: Staff ships.
	// ------------------------------------------------------------------
	shipBody := map[string]interface{}{
		"tracking_number": "SF-" + uuid.New().String()[:8],
		"company":         "顺丰快递",
		"shipped_at":      time.Now().UTC(),
	}
	jsonBody, _ = json.Marshal(shipBody)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "ship order: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusShipped)

	// ------------------------------------------------------------------
	// Step 4: Staff confirms delivery.
	// ------------------------------------------------------------------
	deliverBody := map[string]interface{}{"delivered_at": deliveredAt, "photos": []string{"/uploads/media/deliver-test.jpg"}}
	jsonBody, _ = json.Marshal(deliverBody)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "confirm delivery: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusInLease)

	// ------------------------------------------------------------------
	// Step 5: Customer returns.
	// ------------------------------------------------------------------
	returnBody := map[string]interface{}{
		"courier_company": "顺丰快递",
		"tracking_number": "SF-" + uuid.New().String()[:8],
	}
	jsonBody, _ = json.Marshal(returnBody)
	req = httptest.NewRequest("POST", "/api/orders/"+orderID+"/return", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "customer return: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusReturning)

	// ------------------------------------------------------------------
	// Step 6: Staff return-inspect (good condition) → settlement + refund.
	// ------------------------------------------------------------------
	inspectBody := map[string]interface{}{
		"instrument_sn": "SN-" + uuid.New().String()[:8],
		"scan_time":     scanTime, // on-time return: end_date 00:00 → no overdue
		"condition":     "good",
		"notes":         "完好归还",
		"photos":        []string{},
	}
	jsonBody, _ = json.Marshal(inspectBody)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "return inspect: %s", w.Body.String())
	testutil.AssertState(t, orderID, models.OrderStatusCompleted)

	// ------------------------------------------------------------------
	// Step 7: Verify settlement math end-to-end.
	// ------------------------------------------------------------------
	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&settlement).Error, "settlement must be created")

	// Lease window: delivered now-30d, lease end = now-30d + 30 = today.
	// Returned today (on-time, no overdue). Actual days = 31 (inclusive).
	// Rent payable = tier 1 (30 days × 100) = 3000; the 31st day has no
	// segment → not charged.
	require.Equal(t, 31, settlement.ActualRentDays)
	require.Equal(t, models.Cents(300000), settlement.ActualRentAmount)
	// No overdue fee (returned on end date), no damage.
	require.Equal(t, models.Cents(0), settlement.OverdueChargesTotal)
	// Cash refund = totalRentPaid (3000) + remainingDeposit (500) - rentPayable (3000) = 500.
	require.Equal(t, models.Cents(50000), settlement.CashRefundable, "deposit refund only for full-term on-time return")

	// Refund record created in the real flow and submitted to WeChat Pay:
	// the refund record stays "pending" until the REFUND.SUCCESS callback
	// confirms; the settlement refund_status is "refunding" (async).
	var refundRecord models.OrderRefundRecord
	require.NoError(t, db.Where("tenant_id = ?", tenantID).Order("created_at desc").First(&refundRecord).Error)
	assert.Equal(t, "pending", refundRecord.Status)
	assert.Equal(t, models.Cents(50000), refundRecord.Amount)
	require.NotEmpty(t, refundRecord.RefundID, "refund submitted to WeChat Pay (stub)")
	var settlementReload models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&settlementReload).Error)
	require.Equal(t, "refunding", settlementReload.RefundStatus)

	// Order marked deposit refunded.
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	require.True(t, order.DepositRefunded, "deposit_refunded must be true after refund")

	// Instrument returned to available.
	var instrument models.Instrument
	require.NoError(t, db.Where("id = ?", instrumentID).First(&instrument).Error)
	assert.Equal(t, models.StockStatusAvailable, instrument.StockStatus)

	// ------------------------------------------------------------------
	// Step 8: Idempotency — a second inspect on a completed order fails.
	// ------------------------------------------------------------------
	jsonBody, _ = json.Marshal(inspectBody)
	req = httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, "second inspect on completed order must fail")

	// No double settlement / double refund.
	var count int64
	require.NoError(t, db.Model(&models.Settlement{}).Where("order_id = ?", orderID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "settlement created exactly once")
	require.NoError(t, db.Model(&models.OrderRefundRecord{}).Where("tenant_id = ?", tenantID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "refund record created exactly once")
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
