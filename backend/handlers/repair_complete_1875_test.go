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

// #1875: CompleteRepair must succeed when a photo record exists.
// Regression: the photo check compared the JSONB photos column to '' which
// raises a Postgres error (invalid input syntax for type json); the ignored
// Count error left recordCount=0 so every completion returned 40003 even
// with valid photo records.

func setup1875Fixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	router := gin.New()
	h := &RepairHandler{}
	router.POST("/repair/:id/complete", h.CompleteRepair)
	return router, db
}

func do1875Complete(t *testing.T, router *gin.Engine, actor testutil.TestActor, instrumentID string) (int, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/repair/"+instrumentID+"/complete", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))

	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Code
}

func new1875Instrument(t *testing.T, db *gorm.DB, seed, status string, workerID string) (string, string, string) {
	t.Helper()
	tenantID, orgID, _ := testfixtures.NewTenantIDs(seed)
	instID := uuid.New().String()
	inst := models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: seed, StockStatus: "maintenance", RepairStatus: status,
	}
	if workerID != "" {
		inst.RepairWorkerID = &workerID
	}
	require.NoError(t, db.Create(&inst).Error)
	return tenantID, orgID, instID
}

// A: happy path — a record with a real photo URL exists → 20000 + status flips.
func TestCompleteRepair_WithPhotoRecord_Succeeds(t *testing.T) {
	router, db := setup1875Fixture(t)

	workerID := uuid.New().String()
	tenantID, _, instID := new1875Instrument(t, db, "1875a1b2c3d4", "repair_in_progress", workerID)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: workerID,
		Comment: "更换琴弦完成", Photos: `["/uploads/media/1789099361937697600_6058a2c0.webp"]`,
		CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, workerID)
	httpStatus, bizCode := do1875Complete(t, router, actor, instID)
	require.Equal(t, http.StatusOK, httpStatus)
	require.Equal(t, 20000, bizCode)

	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", instID).Error)
	assert.Equal(t, "repair_completed", inst.RepairStatus)
}

// B: no records at all → 40003.
func TestCompleteRepair_NoRecord_Rejected(t *testing.T) {
	router, db := setup1875Fixture(t)

	workerID := uuid.New().String()
	tenantID, _, instID := new1875Instrument(t, db, "1875b2c3d4e5", "repair_in_progress", workerID)

	actor := testutil.MakeCustomer(tenantID, workerID)
	httpStatus, bizCode := do1875Complete(t, router, actor, instID)
	require.Equal(t, http.StatusBadRequest, httpStatus)
	assert.Equal(t, 40003, bizCode)
}

// C: photos = [] (empty array) → 40003.
func TestCompleteRepair_EmptyPhotos_Rejected(t *testing.T) {
	router, db := setup1875Fixture(t)

	workerID := uuid.New().String()
	tenantID, _, instID := new1875Instrument(t, db, "1875c3d4e5f6", "repair_in_progress", workerID)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: workerID,
		Comment: "只有评论", Photos: `[]`, CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, workerID)
	httpStatus, bizCode := do1875Complete(t, router, actor, instID)
	require.Equal(t, http.StatusBadRequest, httpStatus)
	assert.Equal(t, 40003, bizCode)
}

// D: not the assigned worker → 40300.
func TestCompleteRepair_NotAssignedWorker_Forbidden(t *testing.T) {
	router, db := setup1875Fixture(t)

	workerID := uuid.New().String()
	otherID := uuid.New().String()
	tenantID, _, instID := new1875Instrument(t, db, "1875d4e5f6a7", "repair_in_progress", workerID)
	require.NoError(t, db.Create(&models.RepairRecord{
		ID: uuid.New().String(), InstrumentID: instID, WorkerID: otherID,
		Comment: "越权尝试", Photos: `["/uploads/media/y.webp"]`, CreatedAt: time.Now(),
	}).Error)

	actor := testutil.MakeCustomer(tenantID, otherID)
	httpStatus, bizCode := do1875Complete(t, router, actor, instID)
	require.Equal(t, http.StatusForbidden, httpStatus)
	assert.Equal(t, 40300, bizCode)
}

// E: instrument not in repair_in_progress → 40002.
func TestCompleteRepair_NotInProgress_Rejected(t *testing.T) {
	router, db := setup1875Fixture(t)

	workerID := uuid.New().String()
	tenantID, _, instID := new1875Instrument(t, db, "1875e5f6a7b8", "repair_pending", workerID)

	actor := testutil.MakeCustomer(tenantID, workerID)
	httpStatus, bizCode := do1875Complete(t, router, actor, instID)
	require.Equal(t, http.StatusBadRequest, httpStatus)
	assert.Equal(t, 40002, bizCode)
}
