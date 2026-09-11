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

// #1882: lease-repair authorization — acceptance/rejection require staff of
// the instrument's site and forbid self-acceptance (R1); takeover/reassign are
// site-scoped (R4/R5); pending list is site-scoped.

type fixture1882 struct {
	tenantID, orgID string
	siteA, siteB    string
	memberSub       string // site_member @ A
	techSub         string // repair_technician @ A
	staffBSub       string // site_member @ B
	adminASub       string // site_admin @ A
}

func setup1882Fixture(t *testing.T) (*gin.Engine, *gorm.DB, fixture1882) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1882a1b2c3d4")
	siteA := uuid.New().String()
	siteB := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgID, Name: "A"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgID, Name: "B"}).Error)

	mkUser := func(role, siteID string) string {
		sub := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: sub, IAMSub: sub, TenantID: tenantID, OrgID: orgID,
			Username: "u-" + sub[:8], Name: "用户", Status: "active",
		}).Error)
		require.NoError(t, db.Create(&models.SiteMember{
			TenantID: tenantID, SiteID: siteID, UserID: sub, Role: role, Status: "active",
		}).Error)
		return sub
	}

	f := fixture1882{
		tenantID: tenantID, orgID: orgID,
		siteA: siteA, siteB: siteB,
		memberSub: mkUser("site_member", siteA),
		techSub:   mkUser("repair_technician", siteA),
		staffBSub: mkUser("site_member", siteB),
		adminASub: mkUser("site_admin", siteA),
	}

	h := &RepairHandler{}
	router := gin.New()
	router.POST("/repair/:id/accept", h.AcceptRepair)
	router.POST("/repair/:id/reject", h.RejectRepair)
	router.POST("/repair/:id/takeover", h.TakeoverRepair)
	router.POST("/repair/:id/reassign", h.ReassignRepair)
	router.GET("/repair/pending", h.ListPendingRepairs)
	return router, db, f
}

func mk1882Instrument(t *testing.T, db *gorm.DB, f fixture1882, status, siteID, workerSub string) string {
	t.Helper()
	id := uuid.New().String()
	siteUUID := uuid.MustParse(siteID)
	inst := models.Instrument{
		ID: id, TenantID: f.tenantID, OrgID: &f.orgID,
		SN: "1882-" + id[:8], StockStatus: "maintenance",
		RepairStatus: status, CurrentSiteID: &siteUUID,
	}
	if workerSub != "" {
		inst.RepairWorkerID = &workerSub
	}
	require.NoError(t, db.Create(&inst).Error)
	return id
}

func do1882Post(t *testing.T, router *gin.Engine, actor testutil.TestActor, path string, body interface{}) (int, int) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))

	var resp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp.Code
}

// A: site member accepts a repair completed at their site.
func TestAcceptRepair_SiteMember_OK(t *testing.T) {
	router, db, f := setup1882Fixture(t)
	instID := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/accept", nil)
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, bizCode)

	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instID).Error)
	assert.Empty(t, inst.RepairStatus)
	assert.Equal(t, "available", inst.StockStatus)
}

// B (R1): the assigned repair worker cannot accept their own repair.
func TestAcceptRepair_RepairWorkerSelfAccept_Forbidden(t *testing.T) {
	router, db, f := setup1882Fixture(t)
	instID := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techSub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/accept", nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, bizCode)
}

// C: staff of a different site cannot accept.
func TestAcceptRepair_OtherSite_Forbidden(t *testing.T) {
	router, db, f := setup1882Fixture(t)
	instID := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffBSub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/accept", nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, bizCode)
}

// D (R3): site staff reject → reason persisted as a repair record.
func TestRejectRepair_SiteMember_PersistsReason(t *testing.T) {
	router, db, f := setup1882Fixture(t)
	instID := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/reject", map[string]string{"comment": "漆面未处理"})
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, bizCode)

	var rec models.RepairRecord
	require.NoError(t, db.Where("instrument_id = ?", instID).First(&rec).Error)
	assert.Contains(t, rec.Comment, "验收驳回：漆面未处理")
	assert.Equal(t, f.memberSub, rec.WorkerID)

	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instID).Error)
	assert.Equal(t, "repair_in_progress", inst.RepairStatus)
}

// E (R1): the repair worker cannot reject their own repair.
func TestRejectRepair_RepairWorker_Forbidden(t *testing.T) {
	router, db, f := setup1882Fixture(t)
	instID := mk1882Instrument(t, db, f, "repair_completed", f.siteA, f.techSub)

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.techSub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/reject", map[string]string{"comment": "x"})
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, bizCode)
}

// F (R5): site_admin can reassign to a same-site member; site_member cannot.
func TestReassignRepair_AdminOnly(t *testing.T) {
	router, db, f := setup1882Fixture(t)

	// admin → ok
	instID := mk1882Instrument(t, db, f, "repair_in_progress", f.siteA, f.techSub)
	actor := testutil.MakeSiteAdmin(f.tenantID, f.orgID, f.adminASub)
	httpStatus, bizCode := do1882Post(t, router, actor, "/repair/"+instID+"/reassign", map[string]string{"worker_id": f.memberSub})
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, bizCode)

	// member → forbidden
	instID2 := mk1882Instrument(t, db, f, "repair_in_progress", f.siteA, f.techSub)
	actor2 := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	httpStatus2, bizCode2 := do1882Post(t, router, actor2, "/repair/"+instID2+"/reassign", map[string]string{"worker_id": f.techSub})
	require.Equal(t, http.StatusForbidden, httpStatus2)
	assert.Equal(t, 40300, bizCode2)
}

// G (R4): takeover restricted to the instrument's site.
func TestTakeoverRepair_SiteScoped(t *testing.T) {
	router, db, f := setup1882Fixture(t)

	instID := mk1882Instrument(t, db, f, "repair_in_progress", f.siteA, f.techSub)
	actorB := testutil.MakeSiteMember(f.tenantID, f.orgID, f.staffBSub)
	httpStatus, bizCode := do1882Post(t, router, actorB, "/repair/"+instID+"/takeover", nil)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, bizCode)

	instID2 := mk1882Instrument(t, db, f, "repair_in_progress", f.siteA, f.techSub)
	actorA := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	httpStatus2, bizCode2 := do1882Post(t, router, actorA, "/repair/"+instID2+"/takeover", nil)
	require.Equal(t, http.StatusOK, httpStatus2)
	require.Equal(t, 20000, bizCode2)
}

// H: pending list only returns instruments at the operator's sites.
func TestListPendingRepairs_SiteScoped(t *testing.T) {
	router, db, f := setup1882Fixture(t)

	instA := mk1882Instrument(t, db, f, "repair_pending", f.siteA, "")
	mk1882Instrument(t, db, f, "repair_pending", f.siteB, "")

	actor := testutil.MakeSiteMember(f.tenantID, f.orgID, f.memberSub)
	req := httptest.NewRequest(http.MethodGet, "/repair/pending", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.List, 1)
	assert.Equal(t, instA, resp.Data.List[0]["id"])
}
