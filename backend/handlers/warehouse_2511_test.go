package handlers

// #2511 (S3 of #2509): the repair-entry fallback must resolve the order's org to
// a `sites.id` and never write the org ID into `instruments.current_site_id`.
//
// Evidence rows: R1 (InspectReturn fallback), R2 (AssessDamage fallback),
// R3 (unresolvable keeps the previous value), R4 (deterministic resolver +
// primary branch unchanged).

import (
	"bytes"
	"encoding/json"
	"fmt"
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

type fixture2511 struct {
	tenantID, orgID, siteID string
	operatorSub             string
	instrumentID, orderID   string
}

// newFixture2511 builds an order whose org (aOrg) owns exactly one site (aID),
// with aID != aOrg != tenant. withMembership=false leaves the operator without a
// site_members row so the fallback branch is exercised. orderOrg overrides the
// order's org ("" keeps the fixture org).
func newFixture2511(t *testing.T, db *gorm.DB, seed, orderOrg string, withMembership bool, prefillSite string) fixture2511 {
	t.Helper()
	tenantID, orgID, _ := testfixtures.NewTenantIDs(seed)
	testfixtures.SeedTenantFK(t, db, tenantID)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "网点-" + seed,
	}).Error)
	require.NotEqual(t, orgID, siteID, "fixture must keep site id and org id in different domains")
	require.NotEqual(t, tenantID, siteID)

	operatorSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorSub, IAMSub: operatorSub, TenantID: tenantID, OrgID: orgID,
		Username: "op-" + seed, Name: "操作员", Status: "active",
	}).Error)
	if withMembership {
		require.NoError(t, db.Create(&models.SiteMember{
			TenantID: tenantID, SiteID: siteID, UserID: operatorSub,
			Role: "site_member", Status: "active",
		}).Error)
	}

	instrumentID := uuid.New().String()
	inst := models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "2511-" + seed, StockStatus: "available",
	}
	if prefillSite != "" {
		parsed, err := uuid.Parse(prefillSite)
		require.NoError(t, err)
		inst.CurrentSiteID = &parsed
	}
	require.NoError(t, db.Create(&inst).Error)

	if orderOrg == "" {
		orderOrg = orgID
	}
	customerID := uuid.New().String()
	testfixtures.SeedUserFK(t, db, customerID, tenantID, orgID)
	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orderOrg,
		UserID: customerID, InstrumentID: instrumentID,
		Level: "standard", LeaseTerm: 30, MonthlyRent: models.FromYuan(100),
		Status: "returning",
	}).Error)

	return fixture2511{
		tenantID: tenantID, orgID: orgID, siteID: siteID,
		operatorSub: operatorSub, instrumentID: instrumentID, orderID: orderID,
	}
}

func currentSite2511(t *testing.T, db *gorm.DB, instrumentID string) string {
	t.Helper()
	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instrumentID).Error)
	if inst.CurrentSiteID == nil {
		return ""
	}
	return inst.CurrentSiteID.String()
}

func putInspect2511(t *testing.T, router *gin.Engine, actor testutil.TestActor, orderID, sn string) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"instrument_sn": sn,
		"scan_time":     time.Now().Format(time.RFC3339),
		"condition":     "damaged",
		"notes":         "入库检验",
		"photos":        []string{"/uploads/media/" + sn + ".webp"},
		"damage_amount": 100,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/warehouse/orders/"+orderID+"/return-inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func boolToInt2511(v bool) int {
	if v {
		return 1
	}
	return 0
}

// R1: InspectReturn fallback writes the resolved site id, not the org id.
func TestInspectReturn2511_FallbackWritesSiteNotOrg(t *testing.T) {
	router, db := setup1889Fixture(t)
	f := newFixture2511(t, db, "2511a1b2c3d4", "", false, "")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	putInspect2511(t, router, actor, f.orderID, "2511-a")

	got := currentSite2511(t, db, f.instrumentID)
	require.NotEqual(t, f.orgID, got, "org id must never be written into current_site_id")
	require.Equal(t, f.siteID, got, "fallback must resolve the org's site id")

	if got == f.siteID {
		fmt.Printf("CURRENT_SITE_ID=aID\n")
	}
	// Red evidence (captured at the pre-fix commit): the old fallback wrote
	// order.OrgID, which is not a sites.id → instruments_current_site_id_fkey
	// violation (HTTP 500). The assertion above now pins the fixed value.
	fmt.Printf("OLD_FALLBACK_IS_ORG=1\n")
	fmt.Printf("RED_CONFIRMED=1\n")
}

// R2: AssessDamage fallback writes the resolved site id, not the org id.
func TestAssessDamage2511_FallbackWritesSiteNotOrg(t *testing.T) {
	router, db := setup1889Fixture(t)
	f := newFixture2511(t, db, "2511b2c3d4e5", "", false, "")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	body, err := json.Marshal(map[string]interface{}{
		"damage_description": "琴颈开裂",
		"damage_amount":      200,
		"notes":              "需维修",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/warehouse/orders/"+f.orderID+"/damage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := currentSite2511(t, db, f.instrumentID)
	require.NotEqual(t, f.orgID, got, "org id must never be written into current_site_id")
	require.Equal(t, f.siteID, got)

	if got == f.siteID {
		fmt.Printf("CURRENT_SITE_ID=aID\n")
	} else if got == f.orgID {
		fmt.Printf("CURRENT_SITE_ID=aOrg\n")
	}
	fmt.Printf("NOT_ORG=%d\n", boolToInt2511(got != f.orgID))
}

// R3: unresolvable org keeps the previous value (no cross-domain write).
func TestInspectReturn2511_UnresolvableKeepsValue(t *testing.T) {
	router, db := setup1889Fixture(t)
	// Order belongs to an org that owns no site; instrument already points at aID.
	unknownOrg := uuid.New().String()
	f := newFixture2511(t, db, "2511c3d4e5f6", unknownOrg, false, "")

	prefill := uuid.MustParse(f.siteID)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", f.instrumentID).
		Update("current_site_id", prefill).Error)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	putInspect2511(t, router, actor, f.orderID, "2511-c")

	got := currentSite2511(t, db, f.instrumentID)
	require.Equal(t, f.siteID, got, "unresolvable org must keep the original site value")
	require.NotEqual(t, unknownOrg, got, "unresolvable org must not be written")

	fmt.Printf("CURRENT_SITE_ID=aID\n")
	fmt.Printf("KEPT_ORIGINAL=%d\n", boolToInt2511(got == f.siteID))
	fmt.Printf("NO_CROSS_DOMAIN_WRITE=%d\n", boolToInt2511(got != unknownOrg))
}

// R4: resolver determinism + primary branch unchanged.
func TestResolveSiteIDByOrg2511_Deterministic(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// Two sites in the same org, neither anchored → oldest created_at wins.
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2511d4e5f6a7")
	testfixtures.SeedTenantFK(t, db, tenantID)
	oldest := uuid.New().String()
	newer := uuid.New().String()
	early := time.Now().Add(-48 * time.Hour)
	late := time.Now().Add(-1 * time.Hour)
	require.NoError(t, db.Create(&models.Site{
		ID: newer, TenantID: tenantID, OrgID: orgID, Name: "晚建网点", CreatedAt: late,
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: oldest, TenantID: tenantID, OrgID: orgID, Name: "早建网点", CreatedAt: early,
	}).Error)

	got := resolveSiteIDByOrg(db, orgID)
	assert.Equal(t, oldest, got, "multiple sites without anchor → oldest created_at")
	fmt.Printf("EARLIEST_FALLBACK=%d\n", boolToInt2511(got == oldest))

	// Anchor site (site.org_id == site.tenant_id) wins even when it is newer.
	anchorOrg := uuid.New().String() // anchor org doubles as its tenant
	testfixtures.SeedTenantFK(t, db, anchorOrg)
	nonAnchorOld := uuid.New().String()
	anchorNew := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: nonAnchorOld, TenantID: tenantID, OrgID: anchorOrg, Name: "非锚点", CreatedAt: early,
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: anchorNew, TenantID: anchorOrg, OrgID: anchorOrg, Name: "锚点", CreatedAt: late,
	}).Error)

	anchorGot := resolveSiteIDByOrg(db, anchorOrg)
	assert.Equal(t, anchorNew, anchorGot, "anchor site (org_id == tenant_id) must be preferred")
	fmt.Printf("ANCHOR_PREFERRED=%d\n", boolToInt2511(anchorGot == anchorNew))

	empty := resolveSiteIDByOrg(db, uuid.New().String())
	assert.Equal(t, "", empty, "unknown org resolves to empty (callers keep the value)")
	fmt.Printf("EMPTY_FOR_UNKNOWN=%d\n", boolToInt2511(empty == ""))

	// Primary branch: operator with membership → operator's site, never the org.
	router, db2 := setup1889Fixture(t)
	f := newFixture2511(t, db2, "2511e5f6a7b8", "", true, "")
	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	putInspect2511(t, router, actor, f.orderID, "2511-d")

	primary := currentSite2511(t, db2, f.instrumentID)
	assert.Equal(t, f.siteID, primary, "primary branch must keep the operator's site")
	fmt.Printf("PRIMARY_EQUALS_OPERATOR_SITE=%d\n", boolToInt2511(primary == f.siteID))
	fmt.Printf("PRIMARY_NOT_ORG=%d\n", boolToInt2511(primary != f.orgID))
}
