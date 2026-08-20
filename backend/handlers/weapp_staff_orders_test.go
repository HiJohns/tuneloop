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
	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// staffOrdersRouter builds a gin router for the weapp staff order endpoints
// (#1611): merchant order list, order detail, and staff refund.
func staffOrdersRouter(actor testutil.TestActor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	settlementHandler := NewUserSettlementHandler()
	router.GET("/api/merchant/orders", ListMerchantOrders)
	router.GET("/api/orders/:id", GetOrder)
	router.POST("/api/orders/:id/refund", settlementHandler.StaffRefundOrder)
	return router
}

// staffOrdersSeed creates a user + instrument + paid order for #1611 tests.
func staffOrdersSeed(t *testing.T, tenantID, orgID, userID string) (string, string) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "staffordersuser", Status: "active",
	}).Error)
	inst := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SO-" + time.Now().Format("150405"), BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus: "available",
	}
	require.NoError(t, db.Create(&inst).Error)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: inst.ID,
		Status:  models.OrderStatusPaid,
		Deposit: models.FromYuan(500), CashPaid: 3000,
	}
	require.NoError(t, db.Create(&order).Error)
	return order.ID, inst.ID
}

// TestStaffOrders_List verifies a staff token listing merchant orders sees
// the orders scoped to their org (#1611).
func TestStaffOrders_List(t *testing.T) {
	tenantID := "00000000-0000-4000-8000-0000000000b1"
	orgID := "00000000-0000-4000-8000-0000000000b2"
	userID := "00000000-0000-4000-8000-0000000000b3"
	orderID, _ := staffOrdersSeed(t, tenantID, orgID, userID)

	staffActor := testutil.MakeSiteMember(tenantID, orgID, uuid.New().String())
	router := staffOrdersRouter(staffActor)

	req := httptest.NewRequest("GET", "/api/merchant/orders?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1, "site staff must see their org orders")
	require.Equal(t, orderID, resp.Data.List[0].ID)
}

// TestStaffOrderDetail_RoleGate verifies order detail access control (#1611):
// staff with tenant scope can read the order, while a customer (USER role)
// cannot read another user's order.
func TestStaffOrderDetail_RoleGate(t *testing.T) {
	tenantID := "00000000-0000-4000-8000-0000000000c1"
	orgID := "00000000-0000-4000-8000-0000000000c2"
	customerID := "00000000-0000-4000-8000-0000000000c3"
	orderID, _ := staffOrdersSeed(t, tenantID, orgID, customerID)

	// Staff can read (tenant scope)
	staffActor := testutil.MakeSiteMember(tenantID, orgID, uuid.New().String())
	router := staffOrdersRouter(staffActor)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, httptest.NewRequest("GET", "/api/orders/"+orderID, nil))
	require.Equal(t, http.StatusOK, w1.Code)
	var staffResp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &staffResp))
	require.Equal(t, 20000, staffResp.Code)

	// Customer cannot read another user's order (40400 via user_id scope)
	otherCustomer := testutil.MakeCustomer(tenantID, uuid.New().String())
	router2 := staffOrdersRouter(otherCustomer)
	w2 := httptest.NewRecorder()
	router2.ServeHTTP(w2, httptest.NewRequest("GET", "/api/orders/"+orderID, nil))
	require.Equal(t, http.StatusNotFound, w2.Code)
	var custResp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &custResp))
	require.Equal(t, 40400, custResp.Code)
}

// TestStaffOrderActions verifies the deposit_refunding → staff refund →
// completed flow (#1611 regression of #1607).
func TestStaffOrderActions(t *testing.T) {
	tenantID := "00000000-0000-4000-8000-0000000000d1"
	orgID := "00000000-0000-4000-8000-0000000000d2"
	customerID := "00000000-0000-4000-8000-0000000000d3"

	orderID, _ := staffOrdersSeed(t, tenantID, orgID, customerID)
	db := database.GetDB()
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", orderID).
		Update("status", models.OrderStatusDepositRefunding).Error)

	staffActor := testutil.MakeSiteMember(tenantID, orgID, uuid.New().String())
	router := staffOrdersRouter(staffActor)

	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/refund", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	var updated models.Order
	require.NoError(t, db.First(&updated, "id = ?", orderID).Error)
	require.Equal(t, models.OrderStatusCompleted, updated.Status, "refund closes the order (L-04)")
}
