package handlers

import (
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

// #1873: GET /repair/:id/records must resolve worker display names by IAM sub.
// Regression: repair_records.worker_id stores the IAM sub (JWT sub), but the
// previous name lookup matched users.id — the two diverge for most real users
// (38/42 on prerelease), leaving worker_name empty. repairWorkerName (#1868)
// had the same defect and is covered here too.

func setup1873Fixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	router := gin.New()
	h := &RepairHandler{}
	router.GET("/repair/:id/records", h.ListRecords)
	return router, db
}

func do1873RecordsRequest(t *testing.T, router *gin.Engine, actor testutil.TestActor, instrumentID string) (int, []map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/repair/"+instrumentID+"/records", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Records []map[string]interface{} `json:"records"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Code, resp.Data.Records
}

// A: worker_id = IAM sub ≠ users.id → name still resolved from users.name.
func TestRepairRecords_WorkerNameResolvedByIAMSub(t *testing.T) {
	router, db := setup1873Fixture(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1873a1b2c3d4")
	localID := uuid.New().String()
	iamSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: localID, IAMSub: iamSub, TenantID: tenantID, OrgID: orgID,
		Username: "tech-1873", Name: "王五", Status: "active",
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1873-RECORDS", StockStatus: "maintenance",
		RepairStatus: "repair_in_progress",
	}).Error)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: iamSub,
		Comment: "更换琴弦", Photos: "[]", CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, records := do1873RecordsRequest(t, router, actor, instID)
	require.Equal(t, 20000, code)
	require.Len(t, records, 1)

	assert.Equal(t, "王五", records[0]["worker_name"])
	// original fields preserved
	assert.Equal(t, iamSub, records[0]["worker_id"])
	assert.Equal(t, instID, records[0]["instrument_id"])
	assert.Equal(t, "更换琴弦", records[0]["comment"])
}

// B: name falls back to username when users.name is empty.
func TestRepairRecords_WorkerNameFallbackChain(t *testing.T) {
	router, db := setup1873Fixture(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1873b2c3d4e5")
	iamSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: iamSub, TenantID: tenantID, OrgID: orgID,
		Username: "worker01", Name: "", Status: "active",
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1873-FALLBACK", StockStatus: "maintenance",
	}).Error)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: iamSub,
		Comment: "", Photos: "[]", CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, records := do1873RecordsRequest(t, router, actor, instID)
	require.Equal(t, 20000, code)
	require.Len(t, records, 1)

	assert.Equal(t, "worker01", records[0]["worker_name"])
}

// C: unknown worker → worker_name empty, other fields intact.
func TestRepairRecords_WorkerNameUnknown(t *testing.T) {
	router, db := setup1873Fixture(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1873c3d4e5f6")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1873-UNKNOWN", StockStatus: "maintenance",
	}).Error)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: uuid.New().String(),
		Comment: "已接手", Photos: "[]", CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, records := do1873RecordsRequest(t, router, actor, instID)
	require.Equal(t, 20000, code)
	require.Len(t, records, 1)

	assert.Equal(t, "", records[0]["worker_name"])
}

// D: repairWorkerName (instrument detail, #1868) must also resolve by IAM sub.
func TestInstrumentDetail_RepairWorkerName_ByIAMSub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	router := gin.New()
	router.GET("/instruments/:id", GetInstrumentByID)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1873d4e5f6a7")
	localID := uuid.New().String()
	iamSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: localID, IAMSub: iamSub, TenantID: tenantID, OrgID: orgID,
		Username: "tech-detail", Name: "赵六", Status: "active",
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1873-DETAIL", StockStatus: "maintenance",
		RepairStatus:   "repair_in_progress",
		RepairWorkerID: &iamSub,
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	req := httptest.NewRequest(http.MethodGet, "/instruments/"+instID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	require.NotNil(t, resp.Data["repair_worker_name"])
	assert.Equal(t, "赵六", resp.Data["repair_worker_name"].(string))
}
