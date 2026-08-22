package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1750: 首页乐器列表价格单位错乱——后端统一 daily_rate_cents（分）契约，
// 前端仅 /100 显示。测试锁定 resolveDailyRateCents 解析与接口透出。

func TestResolveDailyRateCents(t *testing.T) {
	t.Run("base_daily_rate 权威（分直用）", func(t *testing.T) {
		inst := models.Instrument{BaseDailyRate: models.ToCentsPtr(float64Ptr(100))}
		require.Equal(t, int64(10000), resolveDailyRateCents(inst), "100 元 = 10000 分")
	})

	t.Run("pricing.daily_rent 兜底（元→分）", func(t *testing.T) {
		inst := models.Instrument{Pricing: `{"daily_rent":100.0}`}
		require.Equal(t, int64(10000), resolveDailyRateCents(inst), "daily_rent 元语义 → FromYuan")
	})

	t.Run("pricing 缺失 → 0", func(t *testing.T) {
		inst := models.Instrument{}
		require.Equal(t, int64(0), resolveDailyRateCents(inst))
	})

	t.Run("base_daily_rate 优先于 pricing", func(t *testing.T) {
		inst := models.Instrument{
			BaseDailyRate: models.ToCentsPtr(float64Ptr(80)),
			Pricing:       `{"daily_rent":100.0}`,
		}
		require.Equal(t, int64(8000), resolveDailyRateCents(inst), "BaseDailyRate 列权威优先")
	})
}

func TestGetPublicInstruments_DailyRateCentsContract(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	baseRate := 100.0
	require.NoError(t, db.Create(&models.Instrument{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		SN:            "SN-PUB-1750",
		StockStatus:   models.StockStatusAvailable,
		BaseDailyRate: models.ToCentsPtr(&baseRate),
		Pricing:       `{"daily_rent":100.0}`,
	}).Error)

	router := gin.New()
	router.GET("/api/public/instruments", GetPublicInstruments)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/public/instruments?tenant="+tenantID, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				BaseDailyRate  *float64 `json:"base_daily_rate"`
				DailyRateCents int64    `json:"daily_rate_cents"`
				Pricing        string   `json:"pricing"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1)
	require.Equal(t, int64(10000), resp.Data.List[0].DailyRateCents,
		"daily_rate_cents must be CENTS (100 元 = 10000 分)")
}

func TestSearchInstruments_DailyRateCentsContract(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	baseRate := 50.0
	require.NoError(t, db.Create(&models.Instrument{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		SN:            "SN-SEARCH-1750",
		CategoryName:  "搜索测试",
		StockStatus:   models.StockStatusAvailable,
		BaseDailyRate: models.ToCentsPtr(&baseRate),
	}).Error)

	router := gin.New()
	router.GET("/api/public/instruments/search", SearchInstruments)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/public/instruments/search?q=搜索测试", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				DailyRateCents int64 `json:"daily_rate_cents"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1)
	require.Equal(t, int64(5000), resp.Data.List[0].DailyRateCents, "search 也返回分契约")
}

var _ = database.GetDB
