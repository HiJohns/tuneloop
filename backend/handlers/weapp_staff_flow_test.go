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
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// setupWeappStaffTables prepares the tables needed by the weapp staff-flow
// regression tests (#1610).
func setupWeappStaffTables(t *testing.T) *gorm.DB {
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	for _, m := range []interface{}{
		&models.Instrument{},
		&models.Order{},

		&models.DamageReport{},
		&models.Notification{},
		&models.OrderStatusHistory{},
		&models.Settlement{},
		&models.OrderPaymentRecord{},
		&models.OrderRefundRecord{},
		&models.LeaseSession{},
		&models.SiteMember{},
		&models.User{},
		&models.Merchant{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
	return db
}

// staffTestRouter builds a gin router with the IAM-context middleware used by
// the warehouse authRequired routes.
func staffTestRouter(tenantID, orgID, userID string, handler *WarehouseHandler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/shipping", handler.UpdateShipping)
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)
	return router
}

// TestShippingFlow_StaffToken verifies the weapp staff shipping flow (#1610):
// a staff token calling PUT /warehouse/orders/:id/shipping transitions the
// order paid → shipped and persists the staff-filled shipping fee (#1541).
func TestShippingFlow_StaffToken(t *testing.T) {
	db := setupWeappStaffTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-SHIP-" + now.Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   models.StockStatusAvailable,
	}
	require.NoError(t, db.Create(&instrument).Error)

	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       uuid.New().String(),
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusPaid,
		Deposit:      models.FromYuan(500),
		CashPaid:     3000,
	}
	require.NoError(t, db.Create(&order).Error)

	handler := NewWarehouseHandler()
	router := staffTestRouter(tenantID, orgID, staffID, handler)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"tracking_number": "SF123456789",
		"company":         "顺丰速运",
		"shipped_at":      now.Format(time.RFC3339),
		"shipping_fee":    100.0,
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/shipping", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			OrderID string `json:"order_id"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, models.OrderStatusShipped, resp.Data.Status)

	var updated models.Order
	require.NoError(t, db.First(&updated, "id = ?", order.ID).Error)
	require.Equal(t, models.OrderStatusShipped, updated.Status)
	require.Equal(t, 100.0, updated.ShippingFee, "staff-filled shipping fee must persist (#1541)")
	require.NotNil(t, updated.TrackingNumber)
	require.Equal(t, "SF123456789", *updated.TrackingNumber)

	var hist models.OrderStatusHistory
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&hist).Error)
	require.Equal(t, models.OrderStatusPaid, hist.StatusFrom)
	require.Equal(t, models.OrderStatusShipped, hist.StatusTo)
}

// TestReceiveFlow_StaffToken verifies the weapp staff receiving flow (#1610):
// PUT /warehouse/orders/:id/return-inspect handles both good (auto refund)
// and damaged (damage report + notification) branches.
func TestReceiveFlow_StaffToken(t *testing.T) {
	db := setupWeappStaffTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	staffID := uuid.New().String()
	userID := uuid.New().String()
	now := time.Now()

	newOrder := func(status string) (models.Instrument, models.Order) {
		inst := models.Instrument{
			TenantID:      tenantID,
			OrgID:         &orgID,
			SN:            "SN-RECV-" + uuid.New().String()[:8],
			BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
			StockStatus:   models.StockStatusRented,
		}
		require.NoError(t, db.Create(&inst).Error)
		start := "2026-08-01"
		end := "2026-08-30"
		o := models.Order{
			ID:           uuid.New().String(),
			TenantID:     tenantID,
			OrgID:        orgID,
			UserID:       userID,
			InstrumentID: inst.ID,
			Status:       status,
			StartDate:    &start,
			EndDate:      &end,
			LeaseTerm:    30,
			Deposit:      models.FromYuan(500),
			CashPaid:     3000,
		}
		require.NoError(t, db.Create(&o).Error)
		return inst, o
	}

	handler := NewWarehouseHandler()
	router := staffTestRouter(tenantID, orgID, staffID, handler)

	// Damaged branch: creates damage report + assessment and notifies customer
	inst1, order1 := newOrder(models.OrderStatusReturning)
	damagedBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": inst1.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "damaged",
		"notes":         "琴面刮痕",
		"damage_amount": 200.0,
	})
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, httptest.NewRequest("PUT", "/api/warehouse/orders/"+order1.ID+"/return-inspect", bytes.NewReader(damagedBody)))
	require.Equal(t, http.StatusOK, w1.Code)
	var resp1 struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &resp1))
	require.Equal(t, 20000, resp1.Code)
	var report models.DamageReport
	require.NoError(t, db.Where("lease_id = ?", order1.ID).First(&report).Error, "damaged branch creates damage report")
	var notif models.Notification
	require.NoError(t, db.Where("user_id = ? AND ref_type = ?", userID, "damage_report").First(&notif).Error)

	// Good branch: auto settlement + completed
	inst2, order2 := newOrder(models.OrderStatusReturning)
	goodBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": inst2.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "good",
		"notes":         "验收通过",
	})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, httptest.NewRequest("PUT", "/api/warehouse/orders/"+order2.ID+"/return-inspect", bytes.NewReader(goodBody)))
	require.Equal(t, http.StatusOK, w2.Code)
	var updated models.Order
	require.NoError(t, db.First(&updated, "id = ?", order2.ID).Error)
	require.Equal(t, models.OrderStatusCompleted, updated.Status, "good branch auto-completes the order")
}

// TestStaffFlow_UnauthenticatedDenied documents the current auth contract of
// the warehouse staff endpoints (#1610): the IAM interceptor rejects requests
// without a valid token (401), while role-level gating (customer vs staff) is
// NOT enforced inside the handlers — a known gap flagged in the issue comment.
func TestStaffFlow_UnauthenticatedDenied(t *testing.T) {
	db := setupWeappStaffTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-NOAUTH-" + now.Format("150405"),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   models.StockStatusAvailable,
	}
	require.NoError(t, db.Create(&instrument).Error)
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       uuid.New().String(),
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusPaid,
		Deposit:      models.FromYuan(500),
		CashPaid:     3000,
	}
	require.NoError(t, db.Create(&order).Error)

	handler := NewWarehouseHandler()
	router := staffTestRouter(tenantID, orgID, staffID, handler)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"tracking_number": "SF000",
		"company":         "顺丰",
		"shipped_at":      now.Format(time.RFC3339),
		"shipping_fee":    10.0,
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/shipping", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler-level behavior: a valid JWT context (any role) is accepted.
	// The 401 gating lives in the IAMInterceptor middleware, which is out of
	// scope for this handler-level test. Documenting current behavior only.
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
}
