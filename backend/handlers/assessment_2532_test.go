package handlers

// #2532 (S3 of #2509, sibling of #2511): SubmitAssessment's fallback branch must
// resolve the order's org to a `sites.id` before writing
// `instruments.current_site_id`, and must not write anything when the org maps
// to no site.
//
// Evidence rows: R1 (fallback writes site id + red confirmation), R2
// (unresolvable org keeps the previous value), R3 (primary branch unchanged).
//
// The fixture is intentionally self-contained (does not reuse newFixture1889,
// whose `sites.org_id == order.OrgID` shape cannot tell the two domains apart).

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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fixture2532 struct {
	tenantID, orgID, siteID string
	operatorSub, customerID string
	instrumentID, orderID   string
}

// newFixture2532 builds tenant T with site A(id=aID, org_id=aOrg, aID != aOrg)
// and an order whose org is orderOrg ("" = aOrg). withMembership=false leaves the
// operator without a site_members row, which deterministically drives
// SubmitAssessment into the fallback branch (resolveOperatorSiteID → nil).
func newFixture2532(t *testing.T, db *gorm.DB, seed, orderOrg string, withMembership bool) fixture2532 {
	t.Helper()

	tenantID, orgID, _ := testfixtures.NewTenantIDs(seed)
	testfixtures.SeedTenantFK(t, db, tenantID)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "网点-" + seed,
	}).Error)
	require.NotEqual(t, orgID, siteID, "fixture must keep site id and org id in different domains")

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
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "2532-" + seed, StockStatus: "available",
	}).Error)

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

	return fixture2532{
		tenantID: tenantID, orgID: orgID, siteID: siteID,
		operatorSub: operatorSub, customerID: customerID,
		instrumentID: instrumentID, orderID: orderID,
	}
}

func setup2532Fixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	router := gin.New()
	router.POST("/orders/:id/assessment", NewAssessmentHandler(db).SubmitAssessment)
	return router, db
}

func postAssessment2532(t *testing.T, router *gin.Engine, actor testutil.TestActor, orderID string) (*httptest.ResponseRecorder, int) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"hasDamage":  true,
		"signature":  "signed",
		"assessedAt": time.Now().Format(time.RFC3339),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/orders/"+orderID+"/assessment", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	return w, w.Code
}

func currentSite2532(t *testing.T, db *gorm.DB, instrumentID string) string {
	t.Helper()
	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instrumentID).Error)
	if inst.CurrentSiteID == nil {
		return ""
	}
	return inst.CurrentSiteID.String()
}

// isSiteID2532 reports whether value is a real `sites.id` (i.e. not an org id).
func isSiteID2532(t *testing.T, db *gorm.DB, value string) bool {
	t.Helper()
	if value == "" {
		return false
	}
	var count int64
	require.NoError(t, db.Model(&models.Site{}).Where("id = ?", value).Count(&count).Error)
	return count > 0
}

func flag2532(v bool) int {
	if v {
		return 1
	}
	return 0
}

// R1: fallback resolves order.OrgID to the org's site id (red-green).
func TestSubmitAssessment2532_FallbackWritesSiteNotOrg(t *testing.T) {
	router, db := setup2532Fixture(t)
	f := newFixture2532(t, db, "2532a1b2c3d4", "", false)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	_, status := postAssessment2532(t, router, actor, f.orderID)

	got := currentSite2532(t, db, f.instrumentID)
	operatorSite := resolveOperatorSiteID(db, f.operatorSub)
	resolved := resolveSiteIDByOrg(db, f.orgID)
	// The pre-fix expression wrote order.OrgID, which is not in the sites.id set.
	oldValueIsOrg := !isSiteID2532(t, db, f.orgID)

	fmt.Printf("STATUS=%d\n", status)
	fmt.Printf("OPERATOR_SITE=%s\n", map[bool]string{true: "none", false: "present"}[operatorSite == nil])
	fmt.Printf("RESOLVED_IS_SITE=%d\n", flag2532(resolved == f.siteID))
	fmt.Printf("CURRENT_IS_SITE=%d\n", flag2532(isSiteID2532(t, db, got)))
	fmt.Printf("CURRENT_NOT_ORG=%d\n", flag2532(got != "" && got != f.orgID))
	fmt.Printf("OLD_VALUE_IS_ORG=%d\n", flag2532(oldValueIsOrg))
	fmt.Printf("RED_CONFIRMED=%d\n", flag2532(oldValueIsOrg && status == http.StatusOK && got == f.siteID))

	require.Equal(t, http.StatusOK, status, "fallback branch must still succeed")
	require.Nil(t, operatorSite, "fixture must drive the fallback branch")
	require.NotEqual(t, f.orgID, got, "org id must never be written into current_site_id")
	require.Equal(t, f.siteID, got, "fallback must write the resolved sites.id")
}

// R2: unresolvable org keeps the previous value (negative case).
func TestSubmitAssessment2532_UnresolvableKeepsValue(t *testing.T) {
	router, db := setup2532Fixture(t)
	unknownOrg := uuid.New().String() // owns no site
	f := newFixture2532(t, db, "2532b2c3d4e5", unknownOrg, false)

	prefill := uuid.MustParse(f.siteID)
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", f.instrumentID).
		Update("current_site_id", prefill).Error)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	_, status := postAssessment2532(t, router, actor, f.orderID)

	got := currentSite2532(t, db, f.instrumentID)
	resolved := resolveSiteIDByOrg(db, unknownOrg)

	fmt.Printf("STATUS=%d\n", status)
	fmt.Printf("KEPT_ORIGINAL=%d\n", flag2532(got == f.siteID))
	fmt.Printf("NO_CROSS_DOMAIN_WRITE=%d\n", flag2532(got != unknownOrg))
	fmt.Printf("RESOLVED_EMPTY=%d\n", flag2532(resolved == ""))
	fmt.Printf("OLD_WOULD_WRITE_ORG=%d\n", flag2532(unknownOrg != "" && !isSiteID2532(t, db, unknownOrg)))

	require.Equal(t, http.StatusOK, status)
	require.Empty(t, resolved, "unknown org must not resolve to a site")
	require.Equal(t, f.siteID, got, "unresolvable org must keep the original site value")
	require.NotEqual(t, unknownOrg, got, "unresolvable org must not be written")
}

// R3: primary branch (operator has a site membership) is unchanged (negative case).
func TestSubmitAssessment2532_PrimaryBranchUnchanged(t *testing.T) {
	router, db := setup2532Fixture(t)
	f := newFixture2532(t, db, "2532c3d4e5f6", "", true)

	operatorSite := resolveOperatorSiteID(db, f.operatorSub)
	require.NotNil(t, operatorSite, "fixture must drive the primary branch")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.operatorSub)
	_, status := postAssessment2532(t, router, actor, f.orderID)

	got := currentSite2532(t, db, f.instrumentID)

	fmt.Printf("STATUS=%d\n", status)
	fmt.Printf("PRIMARY_EQUALS_OPERATOR_SITE=%d\n", flag2532(got == *operatorSite))
	fmt.Printf("PRIMARY_NOT_ORG=%d\n", flag2532(got != "" && got != f.orgID))

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, *operatorSite, got, "primary branch must keep writing the operator's site")
	require.NotEqual(t, f.orgID, got)
}
