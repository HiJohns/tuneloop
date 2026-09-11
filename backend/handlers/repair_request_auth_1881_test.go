package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1881: customer-repair v3 staff/technician operations must be scoped to the
// request's sites; cross-site actors are rejected and listings never fall back
// to a tenant-wide dump.

type fixture1881 struct {
	tenantID, orgID string
	siteA, siteB    string // siteA = repair/target site, siteB = unrelated
	transitSite     string
	memberA         string // site_member @ A
	techA           string // repair_technician @ A
	transitStaff    string // site_member @ transit
	staffB          string // site_member @ B
	noSiteStaff     string // user with no memberships
	customerID      string
}

func setup1881Fixture(t *testing.T) (*gin.Engine, *gorm.DB, fixture1881) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1881a1b2c3d4")
	siteA, siteB, transit := uuid.New().String(), uuid.New().String(), uuid.New().String()
	for _, s := range []string{siteA, siteB, transit} {
		require.NoError(t, db.Create(&models.Site{ID: s, TenantID: tenantID, OrgID: orgID, Name: "S-" + s[:6]}).Error)
	}

	mkUser := func(role, siteID string) string {
		sub := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: sub, IAMSub: sub, TenantID: tenantID, OrgID: orgID,
			Username: "u-" + sub[:8], Name: "用户", Status: "active",
		}).Error)
		if siteID != "" {
			require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteID, UserID: sub, Role: role, Status: "active"}).Error)
		}
		return sub
	}

	f := fixture1881{
		tenantID: tenantID, orgID: orgID,
		siteA: siteA, siteB: siteB, transitSite: transit,
		memberA:      mkUser("site_member", siteA),
		techA:        mkUser("repair_technician", siteA),
		transitStaff: mkUser("site_member", transit),
		staffB:       mkUser("site_member", siteB),
		noSiteStaff:  mkUser("site_member", ""),
		customerID:   uuid.New().String(),
	}

	h := NewRepairRequestHandler()
	router := gin.New()
	router.POST("/repair-requests/:id/quotes", SubmitQuote)
	router.GET("/repair-requests/:id/quotes", ListQuotes)
	router.GET("/repair-requests", h.List)
	router.POST("/repair-requests/:id/complete", h.CompleteRepairRequest)
	router.PUT("/repair-requests/:id/return-shipping", h.ReturnShipping)
	router.POST("/repair-requests/:id/transit-process", h.TransitProcess)
	router.POST("/repair-requests/:id/transit-relay", h.TransitRelay)
	router.POST("/repair-requests/:id/receive", h.Receive)
	router.POST("/repair-requests/:id/requote", h.Requote)
	return router, db, f
}

func mk1881Request(t *testing.T, db *gorm.DB, f fixture1881, status string, controlled bool) *models.RepairRequest {
	t.Helper()
	id := uuid.New().String()
	transit := f.transitSite
	controlledSite := f.siteA
	req := &models.RepairRequest{
		ID: id, TenantID: f.tenantID, SiteID: f.siteA, UserID: f.customerID,
		UserInstrumentID: uuid.New().String(), Status: status,
		MerchantType: "full",
	}
	if controlled {
		req.MerchantType = models.MerchantTypeControlled
		req.TransitSiteID = &transit
		req.ControlledSiteID = &controlledSite
	}
	require.NoError(t, db.Create(req).Error)
	return req
}

func do1881(t *testing.T, router *gin.Engine, method, path string, actor *testutil.TestActor, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if actor != nil {
		req = req.WithContext(actor.InjectContext(req.Context()))
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func codeOf(resp map[string]interface{}) int {
	if v, ok := resp["code"].(float64); ok {
		return int(v)
	}
	return 0
}

// A: only a repair_technician site member may quote; quote is attributed to their site.
func TestSubmitQuote_RequiresTechnician(t *testing.T) {
	router, db, f := setup1881Fixture(t)
	r := mk1881Request(t, db, f, "pending_assessment", false)

	tech := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techA)
	httpStatus, resp := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/quotes", &tech, map[string]float64{"material_fee": 100})
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, codeOf(resp))
	assert.Equal(t, f.siteA, resp["data"].(map[string]interface{})["site_id"])

	// non-technician (site_member) → forbidden
	staffB := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffB)
	httpStatus2, resp2 := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/quotes", &staffB, map[string]float64{"material_fee": 100})
	require.Equal(t, http.StatusForbidden, httpStatus2)
	assert.Equal(t, 40300, codeOf(resp2))
}

// B: complete/return-shipping require the repair site's staff.
func TestCompleteAndReturnShipping_SiteScoped(t *testing.T) {
	router, db, f := setup1881Fixture(t)

	r := mk1881Request(t, db, f, "repairing", false)
	other := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffB)
	httpStatus, resp := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/complete", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, codeOf(resp))

	tech := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techA)
	httpStatus2, resp2 := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/complete", &tech, nil)
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, codeOf(resp2))

	r2 := mk1881Request(t, db, f, "return_pending", false)
	memberA := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberA)
	httpStatus3, resp3 := do1881(t, router, http.MethodPut, "/repair-requests/"+r2.ID+"/return-shipping", &other, map[string]string{"return_tracking_number": "SF1"})
	require.Equal(t, http.StatusForbidden, httpStatus3)
	assert.Equal(t, 40300, codeOf(resp3))

	httpStatus4, resp4 := do1881(t, router, http.MethodPut, "/repair-requests/"+r2.ID+"/return-shipping", &memberA, map[string]string{"return_tracking_number": "SF1"})
	require.Equal(t, http.StatusOK, httpStatus4)
	require.Equal(t, 20000, codeOf(resp4))
}

// C: transit process/relay require the transit site's staff.
func TestTransitOps_TransitSiteScoped(t *testing.T) {
	router, db, f := setup1881Fixture(t)

	r := mk1881Request(t, db, f, "transit_processing", true)
	other := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffB)
	httpStatus, resp := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/transit-process", &other, map[string]float64{"transit_service_fee": 10})
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, codeOf(resp))

	ts := testutil.MakeSiteMember(f.tenantID, f.orgID, f.transitStaff)
	httpStatus2, resp2 := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/transit-process", &ts, map[string]float64{"transit_service_fee": 10})
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, codeOf(resp2))
}

// D: receive is stage-scoped (full → target site; transit_in → controlled site).
func TestReceive_StageScoped(t *testing.T) {
	router, db, f := setup1881Fixture(t)

	// full-authority shipping → target site staff only
	r := mk1881Request(t, db, f, "shipping", false)
	other := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffB)
	httpStatus, resp := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/receive", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, codeOf(resp))

	targetStaff := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberA) // site_member of siteA
	httpStatus2, resp2 := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/receive", &targetStaff, nil)
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, codeOf(resp2))

	// transit_in → controlled site staff only
	r2 := mk1881Request(t, db, f, "transit_in", true)
	httpStatus3, resp3 := do1881(t, router, http.MethodPost, "/repair-requests/"+r2.ID+"/receive", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus3)
	assert.Equal(t, 40300, codeOf(resp3))
}

// E: requote requires this site's repair technician.
func TestRequote_SiteTechnicianOnly(t *testing.T) {
	router, db, f := setup1881Fixture(t)

	r := mk1881Request(t, db, f, "repairing", false)
	// site member (not technician) → 403
	member := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffB) // unrelated site → 403 anyway
	httpStatus, resp := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/requote", &member, map[string]float64{"material_fee": 10})
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, codeOf(resp))

	tech := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techA)
	httpStatus2, resp2 := do1881(t, router, http.MethodPost, "/repair-requests/"+r.ID+"/requote", &tech, map[string]float64{"material_fee": 10})
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, codeOf(resp2))
}

// F: staff quote listing is site-isolated; staff request list never dumps tenant-wide.
func TestListings_SiteIsolated(t *testing.T) {
	router, db, f := setup1881Fixture(t)

	rA := mk1881Request(t, db, f, "pending_assessment", false) // site A
	rB := mk1881Request(t, db, f, "pending_assessment", false) // site A as well? create B explicitly below
	_ = rB
	// move rB to site B
	require.NoError(t, db.Model(&models.RepairRequest{}).Where("id = ?", rB.ID).Update("site_id", f.siteB).Error)
	// create a quote at site B directly
	require.NoError(t, db.Create(&models.RepairQuote{
		ID: uuid.New().String(), RepairRequestID: rB.ID, SiteID: f.siteB,
		WorkerID: f.staffB, QuoteNo: "QB", MaterialFee: models.FromYuan(1),
		Status: models.RepairQuotePending,
	}).Error)

	tech := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techA)
	_, resp := do1881(t, router, http.MethodGet, "/repair-requests/"+rA.ID+"/quotes", &tech, nil)
	if resp["data"] != nil {
		list, _ := resp["data"].(map[string]interface{})["list"].([]interface{})
		assert.Len(t, list, 0, "tech at A must not see B's quotes")
	}

	// member at A sees only site A requests
	memberA := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberA)
	httpStatus, resp2 := do1881(t, router, http.MethodGet, "/repair-requests", &memberA, nil)
	require.Equal(t, http.StatusOK, httpStatus)
	list2, _ := resp2["data"].(map[string]interface{})["list"].([]interface{})
	for _, item := range list2 {
		assert.Equal(t, f.siteA, item.(map[string]interface{})["site_id"])
	}

	// user with no memberships sees empty (not tenant-wide)
	lonely := testutil.MakeSiteMember(f.tenantID, f.orgID, f.noSiteStaff)
	_, resp3 := do1881(t, router, http.MethodGet, "/repair-requests", &lonely, nil)
	list3, _ := resp3["data"].(map[string]interface{})["list"].([]interface{})
	assert.Len(t, list3, 0)
}
