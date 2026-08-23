package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// seedRestoreOrder creates an active user, an instrument in the given stock
// status and an order in the given status — the common fixture for the
// #1767 restore-path tests. Returns the DB for follow-up setup/assertions.
func seedRestoreOrder(t *testing.T, status string, stockStatus string) (models.Order, models.Instrument, string, *gorm.DB) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SettlementCalculation{}))

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "restore-" + status[:6], Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-RESTORE-" + stockStatus, StockStatus: stockStatus,
	}
	require.NoError(t, db.Create(&instrument).Error)

	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: instrument.ID,
		StartDate: strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm: 30, Status: status,
		ReturnedAt: &returnedAt,
		Deposit:    models.FromYuan(500), CashPaid: 300000,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	return order, instrument, userID, db
}

// requireInstrumentRestored asserts the instrument row is back to available.
func requireInstrumentRestored(t *testing.T, instrumentID string) {
	var updated models.Instrument
	require.NoError(t, database.GetDB().Where("id = ?", instrumentID).First(&updated).Error)
	require.Equal(t, models.StockStatusAvailable, updated.StockStatus,
		"#1767 restore path must set instrument back to available")
}

// TestConfirmSettlement_RestoresInstrument verifies #1767 through the real
// ConfirmSettlement handler (HTTP): confirming settlement on a
// deposit_refunding order closes it and restores the rented instrument.
func TestConfirmSettlement_RestoresInstrument(t *testing.T) {
	order, instrument, userID, _ := seedRestoreOrder(t, models.OrderStatusDepositRefunding, "rented")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := testutil.MakeCustomer(order.TenantID, userID).InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/settlements/:id", NewUserSettlementHandler().ConfirmSettlement)

	req := httptest.NewRequest("POST", "/api/user/settlements/"+order.ID, bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "confirm settlement: %s", w.Body.String())

	requireInstrumentRestored(t, instrument.ID)

	var completed models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&completed).Error)
	require.Equal(t, models.OrderStatusCompleted, completed.Status, "settlement confirm closes the order")
}

// TestStaffRefundOrder_RestoresInstrument verifies #1767 through the real
// StaffRefundOrder handler (HTTP): staff refund on a deposit_refunding
// order closes it and restores the rented instrument.
func TestStaffRefundOrder_RestoresInstrument(t *testing.T) {
	order, instrument, _, _ := seedRestoreOrder(t, models.OrderStatusDepositRefunding, "rented")

	staffID := uuid.New().String()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := testutil.MakeSiteMember(order.TenantID, order.OrgID, staffID).InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/orders/:id/refund", NewUserSettlementHandler().StaffRefundOrder)

	req := httptest.NewRequest("POST", "/api/orders/"+order.ID+"/refund", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "staff refund: %s", w.Body.String())

	requireInstrumentRestored(t, instrument.ID)

	var completed models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&completed).Error)
	require.Equal(t, models.OrderStatusCompleted, completed.Status, "refund closes the order")
}

// TestPaymentShortfall_RestoresInstrument verifies #1767 through the real
// applySideEffects path used by the WeChat callback: a payment_shortfall
// payment completes the order and restores the maintenance instrument.
func TestPaymentShortfall_RestoresInstrument(t *testing.T) {
	order, instrument, _, db := seedRestoreOrder(t, models.OrderStatusReturning, "maintenance")

	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: order.ID,
		RefundStatus: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	record := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: order.TenantID,
		UserID: order.UserID, OrderID: &order.ID,
		OrderType: "payment_shortfall", Amount: models.FromYuan(100),
		Type: "payment", Status: "paid",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, applySideEffects(db, &record, time.Now()))

	requireInstrumentRestored(t, instrument.ID)

	var completed models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&completed).Error)
	require.Equal(t, models.OrderStatusCompleted, completed.Status, "shortfall payment closes the order")
}
