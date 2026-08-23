package handlers

import (
	"bytes"
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
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// seedCalcOrder creates an active user, a rented instrument and an order in
// the given status — the common fixture for the #1738 settlement-audit
// tests. Returns the order and the DB for follow-up setup/assertions.
func seedCalcOrder(t *testing.T, status string) (models.Order, *gorm.DB) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SettlementCalculation{}))

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "calc-" + status[:6], Status: "active",
	}).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CALC-" + status[:6], StockStatus: "rented",
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
	return order, db
}

// calcRouter builds a gin router with a customer actor injected, exposing
// the two real settlement endpoints under test (#1738).
func calcRouter(tenantID, actorUserID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := testutil.MakeCustomer(tenantID, actorUserID).InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewUserSettlementHandler()
	router.GET("/api/user/settlements/:id/calculate", h.CalculateSettlement)
	router.POST("/api/user/settlements/:id", h.ConfirmSettlement)
	return router
}

// TestSettlementCalculation_Preview verifies #1738 P2 through the real GET
// calculate handler (HTTP): the preview computation is persisted with
// trigger=preview and the response bytes are IDENTICAL to the stored result
// bytes — "what the client sees is what the audit trail stores".
func TestSettlementCalculation_Preview(t *testing.T) {
	order, _ := seedCalcOrder(t, models.OrderStatusReturned)
	router := calcRouter(order.TenantID, order.UserID)

	req := httptest.NewRequest("GET", "/api/user/settlements/"+order.ID+"/calculate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "calculate preview: %s", w.Body.String())

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	var row models.SettlementCalculation
	require.NoError(t, database.GetDB().
		Where("order_id = ? AND trigger = ?", order.ID, "preview").First(&row).Error)
	require.Equal(t, "preview", row.Trigger)
	require.NotEmpty(t, row.ActualDays)

	// Acceptance item 3: handler response bytes == persisted result bytes.
	require.True(t, bytes.Equal(resp.Data, []byte(*row.Result)),
		"response data must be byte-identical to the stored settlement_calculations.result")

	var snapshot map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*row.InputSnapshot), &snapshot))
	require.Equal(t, order.ID, snapshot["order_id"])
	require.Equal(t, string(order.Status), snapshot["status"])
	require.NotNil(t, snapshot["start_date"])
	require.NotNil(t, snapshot["cash_paid_cents"])
}

// TestSettlementCalculation_Confirm verifies #1738 P2 through the real POST
// confirm handler (HTTP): confirming persists the calculation with
// trigger=confirm.
func TestSettlementCalculation_Confirm(t *testing.T) {
	order, _ := seedCalcOrder(t, models.OrderStatusDepositRefunding)
	router := calcRouter(order.TenantID, order.UserID)

	req := httptest.NewRequest("POST", "/api/user/settlements/"+order.ID, bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "confirm settlement: %s", w.Body.String())

	var row models.SettlementCalculation
	require.NoError(t, database.GetDB().
		Where("order_id = ? AND trigger = ?", order.ID, "confirm").First(&row).Error)
	require.Equal(t, "confirm", row.Trigger)
	require.NotEmpty(t, *row.Result)

	var closed models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&closed).Error)
	require.Equal(t, models.OrderStatusCompleted, closed.Status)
}

// TestSettlementCalculation_AppendOnlyPreview verifies #1738 P2 audit-trail
// semantics through the real handler (HTTP): the table is append-only, so
// repeated previews intentionally create one row per call rather than
// updating in place.
func TestSettlementCalculation_AppendOnlyPreview(t *testing.T) {
	order, _ := seedCalcOrder(t, models.OrderStatusReturned)
	router := calcRouter(order.TenantID, order.UserID)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/user/settlements/"+order.ID+"/calculate", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "calculate preview #%d: %s", i+1, w.Body.String())
	}

	var count int64
	require.NoError(t, database.GetDB().Model(&models.SettlementCalculation{}).
		Where("order_id = ? AND trigger = ?", order.ID, "preview").Count(&count).Error)
	require.Equal(t, int64(2), count,
		"append-only audit trail keeps one row per preview call")
}
