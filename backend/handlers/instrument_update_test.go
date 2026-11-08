package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestUpdateInstrument pins the partial-update contract (docs/spec/api.md §5.5.2,
// "UpdateInstrumentRequest, partial update"): only supplied fields change, absent
// fields keep their stored values. It runs on the shared real test DB instead of
// sqlmock so newly added handler queries (categories / instrument_levels) do not
// have to be mirrored as brittle SQL expectations (#2264 batch 4).
//
// The legacy "missing name/level/category_id must return 400" sub-cases were
// removed: the endpoint has no name field at all, and partial update explicitly
// allows omitting level/category_id.
func TestUpdateInstrument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()

	levelA := models.InstrumentLevel{ID: uuid.New(), Caption: "入门级", Code: "beginner"}
	levelB := models.InstrumentLevel{ID: uuid.New(), Caption: "专业级", Code: "professional"}
	require.NoError(t, db.Create(&levelA).Error)
	require.NoError(t, db.Create(&levelB).Error)

	catA := models.Category{ID: uuid.New().String(), TenantID: tenantID, Name: "钢琴"}
	catB := models.Category{ID: uuid.New().String(), TenantID: tenantID, Name: "吉他"}
	require.NoError(t, db.Create(&catA).Error)
	require.NoError(t, db.Create(&catB).Error)

	deposit := models.FromYuan(500)
	seedInstrument := func(t *testing.T) models.Instrument {
		t.Helper()
		inst := models.Instrument{
			ID:             uuid.New().String(),
			TenantID:       tenantID,
			OrgID:          &orgID,
			CategoryID:     &catA.ID,
			CategoryName:   catA.Name,
			LevelID:        &levelA.ID,
			LevelName:      levelA.Caption,
			SN:             "SN-" + uuid.New().String()[:8],
			Description:    "original description",
			Images:         "[]",
			Specifications: "{}",
			Pricing:        "{}",
			Deposit:        &deposit,
			StockStatus:    models.StockStatusAvailable,
		}
		require.NoError(t, db.Create(&inst).Error)
		return inst
	}

	put := func(t *testing.T, instrumentID string, body map[string]interface{}) *httptest.ResponseRecorder {
		t.Helper()
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: instrumentID}}
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/instruments/"+instrumentID, bytes.NewReader(bodyBytes))
		ctx.Request.Header.Set("Content-Type", "application/json")
		tenantCtx := context.WithValue(context.Background(), middleware.ContextKeyTenantID, tenantID)
		ctx.Request = ctx.Request.WithContext(tenantCtx)
		UpdateInstrument(ctx)
		return w
	}

	reload := func(t *testing.T, instrumentID string) models.Instrument {
		t.Helper()
		var got models.Instrument
		require.NoError(t, db.Where("id = ?", instrumentID).First(&got).Error)
		return got
	}

	t.Run("instrument not found", func(t *testing.T) {
		w := put(t, uuid.New().String(), map[string]interface{}{"description": "x"})
		assert.Equal(t, http.StatusNotFound, w.Code, "body: %s", w.Body.String())
		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, float64(40401), response["code"])
		assert.Contains(t, response["message"], "instrument not found")
	})

	t.Run("only description updates and leaves other fields untouched", func(t *testing.T) {
		inst := seedInstrument(t)
		w := put(t, inst.ID, map[string]interface{}{"description": "updated description"})
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		got := reload(t, inst.ID)
		assert.Equal(t, "updated description", got.Description)
		assert.Equal(t, inst.CategoryID, got.CategoryID)
		assert.Equal(t, inst.CategoryName, got.CategoryName)
		assert.Equal(t, inst.LevelID, got.LevelID)
		assert.Equal(t, inst.LevelName, got.LevelName)
		assert.Equal(t, inst.Deposit, got.Deposit)
		assert.Equal(t, inst.StockStatus, got.StockStatus)
	})

	t.Run("only level_id updates level and leaves other fields untouched", func(t *testing.T) {
		inst := seedInstrument(t)
		w := put(t, inst.ID, map[string]interface{}{"level_id": levelB.ID.String()})
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		got := reload(t, inst.ID)
		require.NotNil(t, got.LevelID)
		assert.Equal(t, levelB.ID, *got.LevelID)
		assert.Equal(t, levelB.Caption, got.LevelName)
		assert.Equal(t, inst.Description, got.Description)
		assert.Equal(t, inst.CategoryID, got.CategoryID)
		assert.Equal(t, inst.Deposit, got.Deposit)
	})

	t.Run("only category_id updates category and leaves other fields untouched", func(t *testing.T) {
		inst := seedInstrument(t)
		w := put(t, inst.ID, map[string]interface{}{"category_id": catB.ID})
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		got := reload(t, inst.ID)
		require.NotNil(t, got.CategoryID)
		assert.Equal(t, catB.ID, *got.CategoryID)
		assert.Equal(t, catB.Name, got.CategoryName)
		assert.Equal(t, inst.Description, got.Description)
		assert.Equal(t, inst.LevelID, got.LevelID)
		assert.Equal(t, inst.Deposit, got.Deposit)
	})

	t.Run("invalid category_id format returns 400", func(t *testing.T) {
		inst := seedInstrument(t)
		w := put(t, inst.ID, map[string]interface{}{"category_id": "not-a-uuid"})
		assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, float64(40002), response["code"])
		assert.Contains(t, response["message"], "invalid category_id format")
	})
}

func TestUpdateInstrumentPricingField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer sqlDB.Close()

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	assert.NoError(t, err)

	testutil.SetDB(t, db)

	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
		"model", "description", "images", "video", "specifications", "pricing",
		"stock_status", "created_at", "updated_at",
	}).AddRow(
		"inst-100", "tenant-001", "org-001", "550e8400-e29b-41d4-a716-446655440000", "Flute", "Yamaha", "advanced",
		"YFL-221", "Professional flute", "[]", "", `[]`, "{}",
		"available", time.Now(), time.Now(),
	)
	mock.ExpectQuery(`SELECT \* FROM "instruments"`).
		WithArgs("inst-100", "tenant-001", 1).
		WillReturnRows(rows)

	catRows := sqlmock.NewRows([]string{"name"}).AddRow("Flute")
	mock.ExpectQuery(`SELECT "name" FROM "categories"`).
		WithArgs("550e8400-e29b-41d4-a716-446655440000", 1).
		WillReturnRows(catRows)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "instruments"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	requestBody := map[string]interface{}{
		"name":        "Flute Updated",
		"brand":       "Yamaha",
		"level":       "advanced",
		"model":       "YFL-221",
		"category_id": "550e8400-e29b-41d4-a716-446655440000",
		"description": "Professional flute",
		"specifications": []map[string]interface{}{
			{
				"name":         "silver-plated",
				"daily_rent":   150.0,
				"weekly_rent":  800.0,
				"monthly_rent": 3500.0,
				"deposit":      10000.0,
				"stock":        2,
			},
		},
		"pricing": map[string]interface{}{
			"discount_days": 7,
			"discount_rate": 0.9,
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Params = gin.Params{{Key: "id", Value: "inst-100"}}
	ctx.Request = httptest.NewRequest("PUT", "/api/instruments/inst-100", bytes.NewReader(bodyBytes))
	ctx.Request.Header.Set("Content-Type", "application/json")

	tenantCtx := context.WithValue(context.Background(), middleware.ContextKeyTenantID, "tenant-001")
	ctx.Request = ctx.Request.WithContext(tenantCtx)

	UpdateInstrument(ctx)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200, got %d. Body: %s", w.Code, w.Body.String())

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(20000), response["code"])

	assert.NoError(t, mock.ExpectationsWereMet())
}
