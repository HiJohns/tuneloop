package handlers

// #2529 资产详情页（/site/stock/:id）取数契约：
// GET /instruments/:id 必须返回页面依赖的键集合（standard daily rate 原始分、
// poster、images、media.display[].url、category_name、level_name、site_name、status），
// 且不存在的 id 必须返回 40400（页面据此区分「不存在」与「加载失败」）。
//
// 证据行：R1（详情接口字段契约 + 不存在 id）。

import (
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

func seedAssetDetail2529(t *testing.T, db *gorm.DB) (tenantID, instrumentID string) {
	t.Helper()
	tenantID = uuid.New().String()
	categoryID := uuid.New().String()
	instrumentID = uuid.New().String()
	now := time.Now()

	require.NoError(t, db.Exec(
		`INSERT INTO categories (id, name, tenant_id, level, visible, sort, created_at)
		 VALUES (?, 'Piano2529', ?, 1, true, 1, ?)`,
		categoryID, tenantID, now).Error)

	rate := models.Cents(1000) // 分：¥10.00/天
	orgID := tenantID
	catID := categoryID
	require.NoError(t, db.Create(&models.Instrument{
		ID:             instrumentID,
		TenantID:       tenantID,
		OrgID:          &orgID,
		CategoryID:     &catID,
		SN:             "SN2529-DETAIL",
		StockStatus:    models.StockStatusAvailable,
		Images:         `["/uploads/media/legacy-2529.jpg"]`,
		Poster:         "/uploads/media/poster-2529.jpg",
		BaseDailyRate:  &rate,
		Specifications: "{}",
		Pricing:        "{}",
	}).Error)

	require.NoError(t, db.Create(&models.InstrumentMedia{
		TenantID:     tenantID,
		OrgID:        tenantID,
		InstrumentID: &instrumentID,
		BatchID:      uuid.New().String(),
		BatchType:    "instrument_create",
		FileName:     "display-2529.jpg",
		FileType:     "image",
		StorageKey:   "display-2529.jpg",
		IsDisplay:    true,
		SortOrder:    1,
	}).Error)

	return tenantID, instrumentID
}

// R1：详情接口字段契约 + 不存在 id 的 40400。
func TestAssetDetail2529_R1(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID, instrumentID := seedAssetDetail2529(t, db)

	gin.SetMode(gin.TestMode)
	router := setupTestRouter(t, tenantID, uuid.New().String())
	router.GET("/instruments/:id", GetInstrumentByID)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+instrumentID, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	rate, ok := resp.Data["base_daily_rate"].(float64)
	require.True(t, ok, "base_daily_rate must be present (page renders standard daily rate)")
	require.EqualValues(t, 1000, int64(rate), "base_daily_rate must be cents")
	require.Equal(t, "/uploads/media/poster-2529.jpg", resp.Data["poster"])
	require.NotNil(t, resp.Data["images"], "images must be present for legacy fallback")
	_, hasCategory := resp.Data["category_name"]
	_, hasLevel := resp.Data["level_name"]
	_, hasSite := resp.Data["site_name"]
	_, hasStatus := resp.Data["status"]
	require.True(t, hasCategory && hasLevel && hasSite && hasStatus,
		"detail payload must keep category_name/level_name/site_name/status")

	media, ok := resp.Data["media"].(map[string]interface{})
	require.True(t, ok, "media must be an object")
	display, ok := media["display"].([]interface{})
	require.True(t, ok, "media.display must be an array")
	require.NotEmpty(t, display, "media.display must carry the display image (page hero)")
	first, ok := display[0].(map[string]interface{})
	require.True(t, ok)
	displayURL, _ := first["url"].(string)
	require.NotEmpty(t, displayURL, "media.display[0].url must be present")

	fmt.Printf("STATUS=200\n")
	fmt.Printf("CODE=20000\n")
	fmt.Printf("HAS_BASE_DAILY_RATE=1\n")
	fmt.Printf("HAS_MEDIA_DISPLAY=1\n")
	fmt.Printf("HAS_POSTER=1\n")

	// Unknown id → 40400 (page shows「资产不存在或已删除」, not a blank screen).
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/instruments/"+uuid.New().String(), nil))
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())

	var notFound struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(missing.Body.Bytes(), &notFound))
	require.Equal(t, 40400, notFound.Code)
	fmt.Printf("NOT_FOUND_CODE=40400\n")
}
