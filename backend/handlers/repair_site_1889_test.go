package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1889: instruments entering the repair flow must record current_site_id so
// the acceptance gate can verify the operator's site. Regression: only the
// inventory-transfer flow ever wrote the column, so repaired instruments had
// no site and AcceptRepair returned 40003 "instrument has no site".

func setup1889Fixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	wh := &WarehouseHandler{}
	router := gin.New()
	router.PUT("/warehouse/orders/:id/return-inspect", wh.InspectReturn)
	router.PUT("/warehouse/orders/:id/damage", wh.AssessDamage)
	router.POST("/orders/:id/assessment", NewAssessmentHandler(db).SubmitAssessment)
	return router, db
}

type fixture1889 struct {
	tenantID, orgID, siteID string
	operatorSub             string
	customerID              string
	instrumentID            string
	orderID                 string
}

func newFixture1889(t *testing.T, db *gorm.DB, seed string, orderStatus string) fixture1889 {
	t.Helper()
	tenantID, orgID, _ := testfixtures.NewTenantIDs(seed)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "测试网点-" + seed,
	}).Error)

	operatorSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorSub, IAMSub: operatorSub, TenantID: tenantID, OrgID: orgID,
		Username: "op-" + seed, Name: "操作员", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteID, UserID: operatorSub,
		Role: "site_member", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "1889-" + seed, StockStatus: "available",
	}).Error)

	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID,
		UserID: uuid.New().String(), InstrumentID: instrumentID,
		Level: "standard", LeaseTerm: 30, MonthlyRent: models.FromYuan(100),
		Status: orderStatus,
	}).Error)

	return fixture1889{
		tenantID: tenantID, orgID: orgID, siteID: siteID,
		operatorSub: operatorSub, customerID: uuid.New().String(),
		instrumentID: instrumentID, orderID: orderID,
	}
}

func do1889Put(t *testing.T, router *gin.Engine, actor testutil.TestActor, path string, body interface{}) int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	return w.Code
}

func assertCurrentSite(t *testing.T, db *gorm.DB, instrumentID, siteID string) {
	t.Helper()
	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instrumentID).Error)
	require.NotNil(t, inst.CurrentSiteID, "current_site_id must be set when entering repair")
	assert.Equal(t, siteID, inst.CurrentSiteID.String())
	assert.Equal(t, "repair_pending", inst.RepairStatus)
}

// A: helper resolves the operator's site and returns nil for unknown users.
func TestResolveOperatorSiteID(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := newFixture1889(t, db, "1889f1a2b3c4", "returning")

	site := resolveOperatorSiteID(db, f.operatorSub)
	require.NotNil(t, site)
	assert.Equal(t, f.siteID, *site)

	assert.Nil(t, resolveOperatorSiteID(db, uuid.New().String()), "unknown user → nil")

	// user without site membership → nil
	lonelySub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: lonelySub, IAMSub: lonelySub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "lonely", Name: "无站点", Status: "active",
	}).Error)
	assert.Nil(t, resolveOperatorSiteID(db, lonelySub), "no membership → nil")
}

// B: InspectReturn damaged sets current_site_id to the operator's site.
func TestInspectReturn_Damaged_SetsCurrentSiteID(t *testing.T) {
	router, db := setup1889Fixture(t)
	f := newFixture1889(t, db, "1889a1b2c3d4", "returning")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	status := do1889Put(t, router, actor, "/warehouse/orders/"+f.orderID+"/return-inspect", map[string]interface{}{
		"instrument_sn": "1889-a",
		"scan_time":     time.Now().Format(time.RFC3339),
		"condition":     "damaged",
		"notes":         "琴盒损坏",
		"photos":        []string{"/uploads/media/1889-a.webp"},
		"damage_amount": 100,
	})
	require.Equal(t, http.StatusOK, status)
	assertCurrentSite(t, db, f.instrumentID, f.siteID)
}

// C: AssessDamage sets current_site_id to the operator's site.
func TestAssessDamage_SetsCurrentSiteID(t *testing.T) {
	router, db := setup1889Fixture(t)
	f := newFixture1889(t, db, "1889b2c3d4e5", "returning")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	status := do1889Put(t, router, actor, "/warehouse/orders/"+f.orderID+"/damage", map[string]interface{}{
		"damage_description": "琴颈开裂",
		"damage_amount":      200,
		"notes":              "需维修",
	})
	require.Equal(t, http.StatusOK, status)
	assertCurrentSite(t, db, f.instrumentID, f.siteID)
}

// D: SubmitAssessment (hasDamage) sets current_site_id to the operator's site.
func TestSubmitAssessment_Damaged_SetsCurrentSiteID(t *testing.T) {
	router, db := setup1889Fixture(t)
	f := newFixture1889(t, db, "1889c3d4e5f6", "returning")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	raw, err := json.Marshal(map[string]interface{}{
		"hasDamage":  true,
		"signature":  "signed",
		"assessedAt": time.Now().Format(time.RFC3339),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/orders/"+f.orderID+"/assessment", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	assertCurrentSite(t, db, f.instrumentID, f.siteID)
}
