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

// #1880: customer-side v3 operations require ownership; the detail endpoint is
// visibility-scoped and desensitizes reporter PII for controlled requests.

type fixture1880 struct {
	tenantID, orgID string
	siteA           string
	ownerSub        string // reporter (customer)
	otherSub        string // another customer
	techASub        string // site A technician
	unrelatedSub    string // customer with no ties
}

func setup1880Fixture(t *testing.T) (*gin.Engine, *gorm.DB, fixture1880) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1880a1b2c3d4")
	siteA := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgID, Name: "网点A"}).Error)

	mkUser := func(role, siteID string) string {
		sub := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: sub, IAMSub: sub, TenantID: tenantID, OrgID: orgID,
			Username: "u-" + sub[:8], Name: "用户", Status: "active", Phone: "13800000000",
		}).Error)
		if siteID != "" {
			require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteID, UserID: sub, Role: role, Status: "active"}).Error)
		}
		return sub
	}

	f := fixture1880{
		tenantID: tenantID, orgID: orgID, siteA: siteA,
		ownerSub:     mkUser("", ""),
		otherSub:     mkUser("", ""),
		techASub:     mkUser("repair_technician", siteA),
		unrelatedSub: mkUser("", ""),
	}

	h := NewRepairRequestHandler()
	router := gin.New()
	router.POST("/repair-requests/:id/quotes/:qid/accept", AcceptQuote)
	router.PUT("/repair-requests/:id/tracking", h.UpdateTracking)
	router.POST("/repair-requests/:id/pay", h.PayRepairRequest)
	router.POST("/repair-requests/:id/confirm-receipt", h.ConfirmReceipt)
	router.POST("/repair-requests/:id/requote-reject", h.RejectRequote)
	router.POST("/repair-requests/:id/records", h.AddRecord)
	router.GET("/repair-requests/:id", h.Get)
	router.POST("/repair-requests", h.Create)
	return router, db, f
}

func mk1880Request(t *testing.T, db *gorm.DB, f fixture1880, status string, controlled bool) *models.RepairRequest {
	t.Helper()
	transit := f.siteA
	controlledSite := f.siteA
	req := &models.RepairRequest{
		ID: uuid.New().String(), TenantID: f.tenantID, SiteID: f.siteA, UserID: f.ownerSub,
		UserInstrumentID: uuid.New().String(), Status: status, MerchantType: "full",
		Description: "琴颈开裂",
	}
	if controlled {
		req.MerchantType = models.MerchantTypeControlled
		req.TransitSiteID = &transit
		req.ControlledSiteID = &controlledSite
	}
	require.NoError(t, db.Create(req).Error)
	return req
}

func do1880(t *testing.T, router *gin.Engine, method, path string, actor *testutil.TestActor, body interface{}) (int, map[string]interface{}) {
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

func code1880(resp map[string]interface{}) int {
	if v, ok := resp["code"].(float64); ok {
		return int(v)
	}
	return 0
}

// A: non-owners are rejected on every customer write endpoint.
func Test1880_NonOwner_Forbidden(t *testing.T) {
	router, db, f := setup1880Fixture(t)

	// accept quote (owner check happens before quote lookup)
	r1 := mk1880Request(t, db, f, "pending_assessment", false)
	other := testutil.MakeCustomer(f.tenantID, f.otherSub)
	httpStatus, resp := do1880(t, router, http.MethodPost, "/repair-requests/"+r1.ID+"/quotes/q1/accept", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, code1880(resp))

	// tracking
	r2 := mk1880Request(t, db, f, "pending_ship", false)
	httpStatus2, resp2 := do1880(t, router, http.MethodPut, "/repair-requests/"+r2.ID+"/tracking", &other, map[string]string{"tracking_number": "SF1"})
	require.Equal(t, http.StatusForbidden, httpStatus2)
	assert.Equal(t, 40300, code1880(resp2))

	// pay
	r3 := mk1880Request(t, db, f, "pending_payment", false)
	httpStatus3, resp3 := do1880(t, router, http.MethodPost, "/repair-requests/"+r3.ID+"/pay", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus3)
	assert.Equal(t, 40300, code1880(resp3))

	// confirm receipt
	r4 := mk1880Request(t, db, f, "returned", false)
	httpStatus4, resp4 := do1880(t, router, http.MethodPost, "/repair-requests/"+r4.ID+"/confirm-receipt", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus4)
	assert.Equal(t, 40300, code1880(resp4))

	// requote-reject
	r5 := mk1880Request(t, db, f, "repairing", false)
	httpStatus5, resp5 := do1880(t, router, http.MethodPost, "/repair-requests/"+r5.ID+"/requote-reject", &other, nil)
	require.Equal(t, http.StatusForbidden, httpStatus5)
	assert.Equal(t, 40300, code1880(resp5))
}

// B: the owner can perform the customer actions.
func Test1880_Owner_Allowed(t *testing.T) {
	router, db, f := setup1880Fixture(t)
	owner := testutil.MakeCustomer(f.tenantID, f.ownerSub)

	// accept quote: create a pending quote first
	r1 := mk1880Request(t, db, f, "pending_assessment", false)
	q := models.RepairQuote{
		ID: uuid.New().String(), RepairRequestID: r1.ID, SiteID: f.siteA,
		WorkerID: f.techASub, QuoteNo: "Q1", MaterialFee: models.FromYuan(100),
		Status: models.RepairQuotePending,
	}
	require.NoError(t, db.Create(&q).Error)
	httpStatus, resp := do1880(t, router, http.MethodPost, "/repair-requests/"+r1.ID+"/quotes/"+q.ID+"/accept", &owner, nil)
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, code1880(resp))

	// tracking
	r2 := mk1880Request(t, db, f, "pending_ship", false)
	httpStatus2, resp2 := do1880(t, router, http.MethodPut, "/repair-requests/"+r2.ID+"/tracking", &owner, map[string]string{"tracking_number": "SF1"})
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, code1880(resp2))

	// confirm receipt
	r3 := mk1880Request(t, db, f, "returned", false)
	httpStatus3, resp3 := do1880(t, router, http.MethodPost, "/repair-requests/"+r3.ID+"/confirm-receipt", &owner, nil)
	require.Equal(t, http.StatusOK, httpStatus3)
	require.Equal(t, 20000, code1880(resp3))

	// add record (reporter side)
	r4 := mk1880Request(t, db, f, "pending_assessment", false)
	httpStatus4, resp4 := do1880(t, router, http.MethodPost, "/repair-requests/"+r4.ID+"/records", &owner, map[string]string{"comment": "补充照片"})
	require.Equal(t, http.StatusOK, httpStatus4)
	require.Equal(t, 20000, code1880(resp4))
}

// C: create validates user_instrument ownership and requires authentication.
func Test1880_Create_OwnershipAndAuth(t *testing.T) {
	router, db, f := setup1880Fixture(t)

	// foreign user_instrument → 40003
	foreignUI := models.UserInstrument{
		ID: uuid.New().String(), UserID: f.otherSub, SN: "OWN-1",
	}
	require.NoError(t, db.Create(&foreignUI).Error)

	owner := testutil.MakeCustomer(f.tenantID, f.ownerSub)
	httpStatus, resp := do1880(t, router, http.MethodPost, "/repair-requests", &owner, map[string]string{
		"user_instrument_id": foreignUI.ID, "site_id": f.siteA, "description": "x",
	})
	require.Equal(t, http.StatusBadRequest, httpStatus)
	assert.Equal(t, 40003, code1880(resp))

	// anonymous → 40100
	httpStatus2, resp2 := do1880(t, router, http.MethodPost, "/repair-requests", nil, map[string]string{
		"sn": "OWN-1", "site_id": f.siteA,
	})
	require.Equal(t, http.StatusUnauthorized, httpStatus2)
	assert.Equal(t, 40100, code1880(resp2))
}

// D: detail visibility and controlled-PII desensitization.
func Test1880_Get_VisibilityAndPII(t *testing.T) {
	router, db, f := setup1880Fixture(t)

	// owner (full) → 200 with reporter phone
	rFull := mk1880Request(t, db, f, "pending_assessment", false)
	owner := testutil.MakeCustomer(f.tenantID, f.ownerSub)
	httpStatus, resp := do1880(t, router, http.MethodGet, "/repair-requests/"+rFull.ID, &owner, nil)
	require.Equal(t, http.StatusOK, httpStatus)
	assert.Equal(t, f.ownerSub, resp["data"].(map[string]interface{})["user_id"])

	// controlled: site technician sees request but without reporter PII
	rCtrl := mk1880Request(t, db, f, "repairing", true)
	tech := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techASub)
	httpStatus2, resp2 := do1880(t, router, http.MethodGet, "/repair-requests/"+rCtrl.ID, &tech, nil)
	require.Equal(t, http.StatusOK, httpStatus2)
	data2 := resp2["data"].(map[string]interface{})
	assert.Equal(t, "", data2["reporter_phone"])
	assert.Equal(t, "", data2["reporter_address"])
	assert.Equal(t, "", data2["reporter_postal_code"])
	assert.Equal(t, "", data2["reporter_name"])

	// unrelated customer → 404
	unrelated := testutil.MakeCustomer(f.tenantID, f.unrelatedSub)
	httpStatus3, resp3 := do1880(t, router, http.MethodGet, "/repair-requests/"+rFull.ID, &unrelated, nil)
	require.Equal(t, http.StatusNotFound, httpStatus3)
	assert.Equal(t, 40400, code1880(resp3))

	// owner of controlled request still sees PII
	ownerCtrl := testutil.MakeCustomer(f.tenantID, f.ownerSub)
	httpStatus4, resp4 := do1880(t, router, http.MethodGet, "/repair-requests/"+rCtrl.ID, &ownerCtrl, nil)
	require.Equal(t, http.StatusOK, httpStatus4)
	assert.NotEqual(t, "", resp4["data"].(map[string]interface{})["reporter_phone"])
}
