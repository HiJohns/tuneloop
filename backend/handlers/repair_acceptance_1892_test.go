package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1892: site staff need a list of repair_completed instruments at their own
// sites. Before this endpoint the acceptance handoff (technician completes →
// staff accepts) had no discoverable entry point: repair_completed instruments
// appeared in no list, only reachable by scanning the SN.

func new1892Router(t *testing.T) (*gin.Engine, *gorm.DB, fixture1882) {
	t.Helper()
	router, db, f := setup1882Fixture(t)
	router.GET("/repair/acceptance", (&RepairHandler{}).ListAcceptanceRepairs)
	return router, db, f
}

func do1892GetAcceptance(t *testing.T, router *gin.Engine, actor testutil.TestActor) []map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/repair/acceptance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	return resp.Data.List
}

// Site isolation + status filter: only repair_completed instruments at the
// caller's sites are listed, enriched with the worker display name.
func TestListAcceptanceRepairs_SiteScopedAndEnriched(t *testing.T) {
	router, db, f := new1892Router(t)

	// visible: repair_completed at site A, assigned to the technician
	visible := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)
	// hidden: wrong status at site A
	mk1882Instrument(t, db, f, "repair_pending", f.siteA, f.techSub)
	// hidden: repair_completed at another site
	mk1882Instrument(t, db, f, "repair_completed", f.siteB, f.techSub)
	// hidden: repair_completed without a site (legacy current_site_id NULL)
	require.NoError(t, db.Create(&models.Instrument{
		ID: uuid.New().String(), TenantID: f.tenantID, OrgID: &f.orgID,
		SN: "1892-nosite", StockStatus: "maintenance", RepairStatus: "repair_completed",
	}).Error)

	// distinctive worker name to prove enrichment
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", f.techSub).Update("name", "李四师傅").Error)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	list := do1892GetAcceptance(t, router, actor)

	require.Len(t, list, 1)
	assert.Equal(t, visible, list[0]["id"])
	assert.Equal(t, "李四师傅", list[0]["repair_worker_name"])
}

// Operators without site memberships get an empty list (never tenant-wide).
func TestListAcceptanceRepairs_NoMembershipEmpty(t *testing.T) {
	router, db, f := new1892Router(t)
	mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	sub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: sub, IAMSub: sub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "u-nosite-" + sub[:8], Name: "无站点", Status: "active",
	}).Error)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, sub)
	assert.Empty(t, do1892GetAcceptance(t, router, actor))
}
