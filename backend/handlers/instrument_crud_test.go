package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestInstrumentCRUD covers §1.1 instrument management end-to-end:
// create (admin) → list → get → update → status change. Uses the
// site_admin actor (full cus_perm) against the real handlers.
func TestInstrumentCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("00000000a1b2")

	// Fixtures: category + instrument level (required by CreateInstrument).
	categoryID := uuid.New().String()
	require.NoError(t, db.Create(&models.Category{
		ID:       categoryID,
		TenantID: tenantID,
		Name:     "钢琴",
		Visible:  true,
	}).Error)

	levelID := uuid.New().String()
	require.NoError(t, db.Create(&models.InstrumentLevel{
		ID:        uuid.MustParse(levelID),
		Caption:   "入门",
		Code:      "entry",
		SortOrder: 1,
	}).Error)

	// Site fixture (instrument belongs to a site).
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID:       siteID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "海淀店",
	}).Error)

	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	router.POST("/api/instruments", CreateInstrument)
	router.GET("/api/instruments", GetInstruments)
	router.GET("/api/instruments/:id", GetInstrumentByID)
	router.PUT("/api/instruments/:id", UpdateInstrument)

	baseRate := 100.0
	createBody := map[string]interface{}{
		"level_id":        levelID,
		"category_id":     categoryID,
		"sn":              "INS-TEST-001",
		"site_id":         siteID,
		"base_daily_rate": baseRate,
		"status":          "available",
		"pricing":         map[string]interface{}{"daily_rent": 100.0, "deposit": 500.0},
	}
	jsonBody, _ := json.Marshal(createBody)

	// Step 1: Create instrument.
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "create instrument: %s", w.Body.String())

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	require.Equal(t, 20100, createResp.Code)
	require.NotEmpty(t, createResp.Data.ID, "instrument id returned")
	instrumentID := createResp.Data.ID

	// Step 2: List instruments → created one present.
	req = httptest.NewRequest("GET", "/api/instruments", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Equal(t, 20000, listResp.Code)
	found := false
	for _, item := range listResp.Data.List {
		if item["id"] == instrumentID {
			found = true
			break
		}
	}
	require.True(t, found, "created instrument appears in list")

	// Step 3: Get by ID → fields correct.
	req = httptest.NewRequest("GET", "/api/instruments/"+instrumentID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var getResp struct {
		Code int `json:"code"`
		Data struct {
			SN     string `json:"sn"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	require.Equal(t, 20000, getResp.Code)
	require.Equal(t, "INS-TEST-001", getResp.Data.SN)
	require.Equal(t, "available", getResp.Data.Status)

	// Step 4: Update instrument (status → rented).
	updateBody, _ := json.Marshal(map[string]interface{}{"status": "rented"})
	req = httptest.NewRequest("PUT", "/api/instruments/"+instrumentID, bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "update instrument: %s", w.Body.String())

	var updated models.Instrument
	require.NoError(t, db.Where("id = ?", instrumentID).First(&updated).Error)
	require.Equal(t, "rented", updated.StockStatus, "status updated to rented")

	// Step 5: Invalid create (missing level_id) → 400.
	badBody, _ := json.Marshal(map[string]interface{}{
		"category_id": categoryID,
		"sn":          "INS-TEST-002",
	})
	req = httptest.NewRequest("POST", "/api/instruments", bytes.NewBuffer(badBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, "missing level_id → 400")
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
var _ = time.Now
