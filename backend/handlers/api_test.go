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
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
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

// TestGetInstrument_MediaDisplayStorageKey verifies GET /instruments/:id
// returns storage_key in media.display items, enabling the frontend to call
// DELETE /instruments/:id/media/key/:storage_key (#1646).
func TestGetInstrument_MediaDisplayStorageKey(t *testing.T) {
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

	instID := uuid.New().String()
	db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, category_id, level, stock_status, images, specifications, pricing, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		instID, tenantID, tenantID, categoryID, time.Now(), time.Now())

	mediaID := uuid.New().String()
	batchID := uuid.New().String()
	storageKey := "display/main-photo.jpg"
	db.Exec(`INSERT INTO instrument_media (id, instrument_id, tenant_id, org_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, 'upload', 'main.jpg', 'image', ?, true, 0, ?)`,
		mediaID, instID, tenantID, tenantID, batchID, storageKey, time.Now())

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/instruments/:id", GetInstrumentByID)

	req := httptest.NewRequest("GET", "/instruments/"+instID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Media struct {
				Display []map[string]interface{} `json:"display"`
			} `json:"media"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.Media.Display, 1, "display media must include the inserted image")

	item := resp.Data.Media.Display[0]
	sk, hasKey := item["storage_key"]
	require.True(t, hasKey, "media.display item must carry storage_key (#1646)")
	require.Equal(t, storageKey, sk)
}

// TestGetPublicCategories_IgnoresHomeMenuConfig verifies #1645: the public
// category list is driven ONLY by category.visible/sort — a stale
// home_menu_config (visible_ids without sub-categories) must not override
// hide/sort or filter out sub-categories.
func TestGetPublicCategories_IgnoresHomeMenuConfig(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, _, _ = setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	// Two top-level categories (sort 1/2) + one sub-category (sort 1)
	catA := uuid.New().String()
	catB := uuid.New().String()
	subCat := uuid.New().String()
	for _, c := range []struct {
		id       string
		name     string
		parentID string
		visible  bool
		sort     int
	}{
		{catA, "分类A", "", true, 1},
		{catB, "分类B", "", true, 2},
		{subCat, "子分类A1", catA, true, 1},
	} {
		res := db.Exec(`INSERT INTO categories (id, tenant_id, name, level, visible, sort, created_at)
			VALUES (?, ?, ?, 1, ?, ?, NOW())`,
			c.id, tenantID, c.name, c.visible, c.sort)
		require.NoError(t, res.Error)
	}

	// Stale home_menu_config: only catA visible, no sub-categories
	db.Exec(`INSERT INTO system_settings (id, tenant_id, setting_key, setting_value, updated_at)
		VALUES (gen_random_uuid(), ?, 'home_menu_config', ?, NOW())`,
		tenantID, `{"visible_ids":["`+catA+`"],"sort_order":{"`+catA+`":9}}`)

	router := setupTestRouter(t, tenantID, uuid.New().String())
	router.GET("/public/categories", GetPublicCategories)

	req := httptest.NewRequest("GET", "/public/categories?tenant="+tenantID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				ParentID string `json:"parent_id"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// home_menu_config must be ignored: all categories (incl. sub-category)
	// returned — no visible_ids filtering, no sort_order override.
	require.Len(t, resp.Data.List, 4, "home_menu_config must not filter categories (Piano+3)")
	ids := []string{resp.Data.List[0].ID, resp.Data.List[1].ID, resp.Data.List[2].ID, resp.Data.List[3].ID}
	require.Contains(t, ids, catA)
	require.Contains(t, ids, catB)
	require.Contains(t, ids, subCat, "sub-category must not be filtered out")
	// sort ASC applies: 分类B (sort=2) must be last, after the sort=1 group.
	require.Equal(t, catB, resp.Data.List[3].ID, "sort ASC: 分类B (sort=2) last")
	// Sub-category (sort=1) must be within the leading sort=1 group.
	require.Contains(t, ids[:3], subCat, "sub-category (sort=1) within leading group")
}
