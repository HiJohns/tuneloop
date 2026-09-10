package handlers

import (
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

// #1868: GET /instruments/:id must expose the repair workflow fields
// (repair_status / repair_worker_id / repair_worker_name) — RepairWorkflow.jsx
// drives every action panel (start / records / takeover / accept) from these
// values. Regression: detail response previously omitted them, leaving the
// repair page without any action buttons.

func setup1868Fixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	router := gin.New()
	router.GET("/instruments/:id", GetInstrumentByID)
	return router, db
}

func do1868Request(t *testing.T, router *gin.Engine, actor testutil.TestActor, instrumentID string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/instruments/"+instrumentID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Code, resp.Data
}

// A: repair_pending without worker → status exposed, worker fields null.
func TestInstrumentDetail_RepairFields_PendingNoWorker(t *testing.T) {
	router, db := setup1868Fixture(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1868a1b2c3d4")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1868-PENDING", StockStatus: "maintenance",
		RepairStatus: "repair_pending",
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, data := do1868Request(t, router, actor, instID)
	require.Equal(t, 20000, code)

	assert.Equal(t, "repair_pending", data["repair_status"])
	assert.Nil(t, data["repair_worker_id"])
	assert.Nil(t, data["repair_worker_name"])
}

// B: repair_in_progress with assigned worker → all three fields populated,
// worker name resolved from users.name.
func TestInstrumentDetail_RepairFields_InProgressWithWorker(t *testing.T) {
	router, db := setup1868Fixture(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("1868b2c3d4e5")
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "worker-1868", Name: "李四", Status: "active",
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1868-INPROGRESS", StockStatus: "maintenance",
		RepairStatus:   "repair_in_progress",
		RepairWorkerID: &userID,
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, data := do1868Request(t, router, actor, instID)
	require.Equal(t, 20000, code)

	assert.Equal(t, "repair_in_progress", data["repair_status"])
	require.NotNil(t, data["repair_worker_id"])
	assert.Equal(t, userID, data["repair_worker_id"].(string))
	require.NotNil(t, data["repair_worker_name"])
	assert.Equal(t, "李四", data["repair_worker_name"].(string))
}

// C: repair_completed → status exposed (acceptance panel driven by it).
func TestInstrumentDetail_RepairFields_Completed(t *testing.T) {
	router, db := setup1868Fixture(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1868c3d4e5f6")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1868-COMPLETED", StockStatus: "available",
		RepairStatus: "repair_completed",
	}).Error)

	actor := testutil.MakeCustomer(tenantID, uuid.New().String())
	code, data := do1868Request(t, router, actor, instID)
	require.Equal(t, 20000, code)

	assert.Equal(t, "repair_completed", data["repair_status"])
}

// D: cross-tenant lookup → 404 (existing tenant isolation regression).
func TestInstrumentDetail_RepairFields_CrossTenant404(t *testing.T) {
	router, db := setup1868Fixture(t)

	tenantA, _, _ := testfixtures.NewTenantIDs("1868d4e5f6a1")
	tenantB, orgB, _ := testfixtures.NewTenantIDs("1868e5f6a1b2")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantB, OrgID: &orgB,
		SN: "1868-OTHER-TENANT", StockStatus: "maintenance",
		RepairStatus: "repair_pending",
	}).Error)

	actor := testutil.MakeCustomer(tenantA, uuid.New().String())
	req := httptest.NewRequest(http.MethodGet, "/instruments/"+instID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40400, resp.Code)
}
