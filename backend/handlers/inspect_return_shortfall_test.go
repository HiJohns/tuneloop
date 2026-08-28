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
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1799: 补缴订单重复接收防御。
// TestInspectReturn_BlocksDuplicateReceiveWhenShortfallPending:
// returning + pending payment_shortfall → InspectReturn 返回 40002，订单不完成。
func TestInspectReturn_BlocksDuplicateReceiveWhenShortfallPending(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-SHORTFALL-RECV-" + now.Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	start := "2026-08-01"
	end := "2026-08-30"
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusReturning,
		StartDate:    &start,
		EndDate:      &end,
		LeaseTerm:    30,
		Deposit:      models.FromYuan(500),
		CashPaid:     models.FromYuan(35),
	}
	require.NoError(t, db.Create(&order).Error)

	// pending payment_shortfall record — first receive already created it
	shortfallOrgID := orgID
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     &shortfallOrgID,
		UserID:    userID,
		OrderID:   &order.ID,
		OrderType: "payment_shortfall",
		Amount:    models.FromYuan(140),
		Type:      "payment",
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, staffID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": instrument.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "good",
		"notes":         "验收通过",
		"damage_amount": 0,
		"photos":        []string{"/uploads/media/inspect-test.jpg"},
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
	require.Contains(t, resp.Message, "订单待顾客补缴")

	// 订单不得被标记完成（保持 returning）
	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	require.Equal(t, models.OrderStatusReturning, after.Status)
}

// TestInspectReturn_NoShortfallCompletesOrder:
// 无 pending shortfall → 正常一次接收 completed（回归，不受防御影响）。
func TestInspectReturn_NoShortfallCompletesOrder(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-NOSHORTFALL-" + now.Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	start := "2026-08-01"
	end := "2026-08-30"
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusReturning,
		StartDate:    &start,
		EndDate:      &end,
		LeaseTerm:    30,
		Deposit:      models.FromYuan(500),
		CashPaid:     models.FromYuan(500),
	}
	require.NoError(t, db.Create(&order).Error)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, staffID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": instrument.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "good",
		"notes":         "验收通过",
		"damage_amount": 0,
		"photos":        []string{"/uploads/media/inspect-test.jpg"},
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	require.Equal(t, models.OrderStatusCompleted, after.Status)
}
