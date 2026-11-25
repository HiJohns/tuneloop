package handlers

// #2509 / #2512: guest order creation must resolve orders.org_id from sites.org_id
// (never persist sites.id). Fixtures keep sites.id != sites.org_id (#2508/#2510).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

type guest2512Fixture struct {
	db       *gorm.DB
	tenantID string
	orgA     string
	orgB     string
	siteA    string
	siteB    string
	userID   string
	router   *gin.Engine
}

func newGuest2512Fixture(t *testing.T) *guest2512Fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	f := &guest2512Fixture{
		db:       db,
		tenantID: uuid.New().String(),
		orgA:     uuid.New().String(),
		orgB:     uuid.New().String(),
		siteA:    uuid.New().String(),
		siteB:    uuid.New().String(),
		userID:   uuid.New().String(),
	}
	require.NotEqual(t, f.siteA, f.orgA, "fixture: sites.id must differ from sites.org_id (#2508)")
	require.NotEqual(t, f.siteB, f.orgB, "fixture: sites.id must differ from sites.org_id (#2508)")
	require.NoError(t, db.Create(&models.Site{ID: f.siteA, TenantID: f.tenantID, OrgID: f.orgA, Name: "Site A"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.siteB, TenantID: f.tenantID, OrgID: f.orgB, Name: "Site B"}).Error)
	testutil.SeedVerifiedCustomer(t, db, f.tenantID, f.orgA, f.userID)

	handler := NewUserRentalHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		// Guest context: user id present, tenant/org empty (production JWT has no tid/oid).
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, f.userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "USER")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders", handler.CreateOrder)
	router.POST("/api/user/orders/batch", handler.BatchCreateOrder)
	f.router = router
	return f
}

// instrument2512 seeds an available instrument. siteID/currentSiteID are optional
// (sites.id values); org_id stays NULL so the resolver has to fall back.
func (f *guest2512Fixture) instrument2512(t *testing.T, siteID, currentSiteID *string) string {
	t.Helper()
	inst := models.Instrument{
		ID:          uuid.New().String(),
		TenantID:    f.tenantID,
		StockStatus: "available",
		Pricing:     `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}
	if siteID != nil {
		id := uuid.MustParse(*siteID)
		inst.SiteID = &id
	}
	if currentSiteID != nil {
		id := uuid.MustParse(*currentSiteID)
		inst.CurrentSiteID = &id
	}
	require.NoError(t, f.db.Create(&inst).Error)
	return inst.ID
}

type order2512Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data struct {
		OrderID string `json:"order_id"`
		Orders  []struct {
			OrderID string `json:"order_id"`
		} `json:"orders"`
	} `json:"data"`
}

func postOrder2512(t *testing.T, router *gin.Engine, path string, body interface{}) (int, order2512Response) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp order2512Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "response body: %s", w.Body.String())
	return w.Code, resp
}

func (f *guest2512Fixture) orderCount2512(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, f.db.Model(&models.Order{}).Where("user_id = ?", f.userID).Count(&count).Error)
	return count
}

func int2512(v bool) int {
	if v {
		return 1
	}
	return 0
}

// R1: org_id NULL + site_id=aID → persisted orders.org_id must be aOrg (red: old code wrote aID).
func TestCreateOrder2512_GuestOrgFromSite(t *testing.T) {
	f := newGuest2512Fixture(t)
	siteA := f.siteA
	instrumentID := f.instrument2512(t, &siteA, nil)

	status, resp := postOrder2512(t, f.router, "/api/user/orders", map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-03",
		"rent_days":     3,
	})
	require.Equal(t, http.StatusCreated, status, "status=%d code=%d message=%s", status, resp.Code, resp.Message)
	require.Equal(t, 20000, resp.Code)
	require.NotEmpty(t, resp.Data.OrderID)

	var order models.Order
	require.NoError(t, f.db.Where("id = ?", resp.Data.OrderID).First(&order).Error)

	oldFallback := f.siteA // pre-fix: effectiveOrgID = instrument.SiteID.String()
	t.Logf("ORDER_ORG=%s", order.OrgID)
	t.Logf("SITE_ORG=%s", f.orgA)
	t.Logf("SITE_ID_IS=%s", oldFallback)
	t.Logf("OLD_FALLBACK_IS_SITE_ID=%d", int2512(oldFallback != f.orgA))
	t.Logf("RED_CONFIRMED=%d", int2512(oldFallback != order.OrgID))
	t.Logf("ORDER_ORG_MATCHES_SITE_ORG=%d", int2512(order.OrgID == f.orgA))

	require.Equal(t, f.orgA, order.OrgID, "orders.org_id must be sites.org_id, not sites.id")
	require.NotEqual(t, f.siteA, order.OrgID)
	t.Logf("R1_OK")
}

// R2: site_id NULL + current_site_id=bID → orders.org_id = bOrg.
func TestCreateOrder2512_GuestOrgFromCurrentSite(t *testing.T) {
	f := newGuest2512Fixture(t)
	siteB := f.siteB
	instrumentID := f.instrument2512(t, nil, &siteB)

	status, resp := postOrder2512(t, f.router, "/api/user/orders", map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-03",
		"rent_days":     3,
	})
	require.Equal(t, http.StatusCreated, status, "status=%d code=%d message=%s", status, resp.Code, resp.Message)

	var order models.Order
	require.NoError(t, f.db.Where("id = ?", resp.Data.OrderID).First(&order).Error)

	t.Logf("ORDER_ORG=%s", order.OrgID)
	t.Logf("SITE_ORG=%s", f.orgB)
	t.Logf("RESOLVED_FROM_CURRENT_SITE=%d", int2512(order.OrgID == f.orgB))
	t.Logf("ORDER_ORG_MATCHES_CURRENT_SITE_ORG=%d", int2512(order.OrgID == f.orgB))
	require.Equal(t, f.orgB, order.OrgID)
	require.NotEqual(t, f.siteB, order.OrgID)
	t.Logf("R2_OK")
}

// R3: no org_id / site_id / current_site_id → 400/40002 and no order rows.
func TestCreateOrder2512_UnresolvableRejected(t *testing.T) {
	f := newGuest2512Fixture(t)
	instrumentID := f.instrument2512(t, nil, nil)

	status, resp := postOrder2512(t, f.router, "/api/user/orders", map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2024-01-01",
		"end_date":      "2024-01-03",
		"rent_days":     3,
	})
	after := f.orderCount2512(t)

	t.Logf("STATUS=%d", status)
	t.Logf("CODE=%d", resp.Code)
	t.Logf("ORDERS_AFTER=%d", after)

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, 40002, resp.Code)
	require.Equal(t, int64(0), after, "rejected order must not persist rows")
	t.Logf("R3_OK")
}

// R4: batch derives the same org for every order and rejects (all-or-nothing)
// when any instrument cannot be resolved.
func TestBatchCreateOrder2512_GuestOrgFromSiteAndReject(t *testing.T) {
	f := newGuest2512Fixture(t)
	siteA := f.siteA
	first := f.instrument2512(t, &siteA, nil)
	second := f.instrument2512(t, &siteA, nil)

	status, resp := postOrder2512(t, f.router, "/api/user/orders/batch", map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": first, "start_date": "2024-01-01", "rent_days": 3},
			{"instrument_id": second, "start_date": "2024-01-01", "rent_days": 3},
		},
	})
	require.Equal(t, http.StatusCreated, status, "status=%d code=%d message=%s", status, resp.Code, resp.Message)
	require.Len(t, resp.Data.Orders, 2)

	orgs := map[string]bool{}
	for _, item := range resp.Data.Orders {
		var order models.Order
		require.NoError(t, f.db.Where("id = ?", item.OrderID).First(&order).Error)
		orgs[order.OrgID] = true
		require.Equal(t, f.orgA, order.OrgID)
		require.NotEqual(t, f.siteA, order.OrgID)
	}
	t.Logf("BATCH_ORG=%s", f.orgA)
	t.Logf("SITE_ORG=%s", f.orgA)
	t.Logf("BATCH_ORGS_UNIQUE=%d", int2512(len(orgs) == 1))
	t.Logf("BATCH_ORG_MATCHES_SITE_ORG=%d", int2512(len(orgs) == 1))

	// Second batch: one resolvable + one unresolvable → reject before any write.
	before := f.orderCount2512(t)
	ok := f.instrument2512(t, &siteA, nil)
	broken := f.instrument2512(t, nil, nil)
	rejectStatus, rejectResp := postOrder2512(t, f.router, "/api/user/orders/batch", map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": ok, "start_date": "2024-01-01", "rent_days": 3},
			{"instrument_id": broken, "start_date": "2024-01-01", "rent_days": 3},
		},
	})
	after := f.orderCount2512(t)

	t.Logf("REJECT_STATUS=%d", rejectStatus)
	t.Logf("REJECT_CODE=%d", rejectResp.Code)
	t.Logf("BATCH_ORDERS_AFTER=%d", after-before)

	require.Equal(t, http.StatusBadRequest, rejectStatus)
	require.Equal(t, 40002, rejectResp.Code)
	require.Equal(t, before, after, "rejected batch must not persist partial orders")
	t.Logf("R4_OK")
}
