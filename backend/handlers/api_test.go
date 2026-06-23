package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
)

func TestGetInstruments_Thumbnail(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	categoryID, _, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	// Create a second instrument with a display image in InstrumentMedia
	instWithMedia := uuid.New().String()
	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, category_id, level, stock_status, images, specifications, pricing, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		instWithMedia, tenantID, tenantID, categoryID, time.Now(), time.Now())

	mediaID := uuid.New().String()
	batchID := uuid.New().String()
	res := db.Exec(`INSERT INTO instrument_media (id, instrument_id, tenant_id, org_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, 'upload', 'test.jpg', 'image', 'test/thumb.jpg', true, 0, ?)`,
		mediaID, instWithMedia, tenantID, tenantID, batchID, time.Now())
	require.NoError(t, res.Error)

	// Register GetInstruments handler on a test router
	router := setupTestRouter(t, tenantID, userID)
	router.GET("/instruments", GetInstruments)

	req := httptest.NewRequest("GET", "/instruments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
			Total int                     `json:"total"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 20000, resp.Code)
	assert.Equal(t, 2, resp.Data.Total)

	for _, inst := range resp.Data.List {
		id := inst["id"].(string)
		thumbnail, hasThumb := inst["thumbnail"]
		require.True(t, hasThumb, "instrument %s should have thumbnail field", id)
		if id == instWithMedia {
			thumbStr, ok := thumbnail.(string)
			require.True(t, ok, "thumbnail should be a string")
			assert.NotEmpty(t, thumbStr, "instrument with media should have non-empty thumbnail")
		} else {
			thumbStr, _ := thumbnail.(string)
			assert.Empty(t, thumbStr, "instrument without media should have empty thumbnail")
		}
	}
}
