package handlers

// #2577 库存监控页筛选：本文件是**只读回归测试**，不改任何生产代码。
//
// 目的：证明页面即将发送的参数组合（`sn` / `category_id` / `stock_status` /
// `exclude_status`，由 `frontend-pc/src/utils/instrumentStockFilters.js` 生成）
// 在真实 Postgres 数据上确实成立，避免「前端拼的参数后端不认」这类静默失配。
//
// fixture（Accepted Plan #2577 R2）：
//
//	分类 P（父） → C（子）；X（其他分类，无子级）
//	A: sn=CVZ-104, available, C
//	B: sn=CVZ-105, available, C
//	E: sn=CVZ-106, available, X
//	D: sn=CVZ-107, lost,      X
//
// 断言：`data.total == len(data.list)`（服务端分页语义与筛选同时成立）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type instrumentStockFilters2577Fixture struct {
	tenantID string
	userID   string
	parentID string
	childID  string
	otherID  string
}

func seedStockFilters2577(t *testing.T, db *gorm.DB) instrumentStockFilters2577Fixture {
	t.Helper()
	f := instrumentStockFilters2577Fixture{
		tenantID: uuid.New().String(),
		userID:   uuid.New().String(),
		parentID: uuid.New().String(),
		childID:  uuid.New().String(),
		otherID:  uuid.New().String(),
	}
	mkCategory := func(id, name string, parent *string) {
		require.NoError(t, db.Create(&models.Category{
			ID: id, TenantID: f.tenantID, Name: name, ParentID: parent, Level: 1, Visible: true,
		}).Error)
	}
	mkCategory(f.parentID, "Piano2577", nil)
	mkCategory(f.childID, "GrandPiano2577", &f.parentID)
	mkCategory(f.otherID, "Violin2577", nil)

	mkInstrument := func(sn, status, categoryID string) {
		cat := categoryID
		inst := models.Instrument{
			ID:             uuid.New().String(),
			TenantID:       f.tenantID,
			OrgID:          &f.tenantID,
			CategoryID:     &cat,
			SN:             sn,
			StockStatus:    status,
			Images:         "[]",
			Specifications: "{}",
			Pricing:        "{}",
		}
		require.NoError(t, db.Create(&inst).Error, "create instrument %s", sn)
	}
	mkInstrument("CVZ-104", models.StockStatusAvailable, f.childID)
	mkInstrument("CVZ-105", models.StockStatusAvailable, f.childID)
	mkInstrument("CVZ-106", models.StockStatusAvailable, f.otherID)
	mkInstrument("CVZ-107", models.StockStatusLost, f.otherID)

	return f
}

// listStock2577 executes GET /instruments with the given query and returns the
// SNs plus the reported total.
func listStock2577(t *testing.T, f instrumentStockFilters2577Fixture, query string) ([]string, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := setupTestRouter(t, f.tenantID, f.userID)
	router.GET("/instruments", GetInstruments)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments?"+query, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				SN string `json:"sn"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	sns := make([]string, 0, len(resp.Data.List))
	for _, item := range resp.Data.List {
		sns = append(sns, item.SN)
	}
	return sns, int(resp.Data.Total)
}

func TestInstrumentStockFilters2577(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedStockFilters2577(t, db)

	// 1) 识别码模糊搜索（页面 Input.Search → `sn`）
	snHits, snTotal := listStock2577(t, f, "page=1&pageSize=100&sn=CVZ-104")
	require.Equal(t, []string{"CVZ-104"}, snHits)

	// 2) 父分类筛选必须展开后代分类（页面分类下拉 → `category_id`）
	categoryHits, categoryTotal := listStock2577(t, f, "page=1&pageSize=100&category_id="+f.parentID)
	require.ElementsMatch(t, []string{"CVZ-104", "CVZ-105"}, categoryHits)

	// 3) 状态筛选（页面状态下拉 → `stock_status`）
	statusHits, statusTotal := listStock2577(t, f, "page=1&pageSize=100&stock_status=available")
	require.ElementsMatch(t, []string{"CVZ-104", "CVZ-105", "CVZ-106"}, statusHits)

	// 4) 非管理级可见性（页面 → `exclude_status=lost,archived,sold`）
	excludeHits, excludeTotal := listStock2577(t, f, "page=1&pageSize=100&exclude_status=lost,archived,sold")
	require.ElementsMatch(t, []string{"CVZ-104", "CVZ-105", "CVZ-106"}, excludeHits)
	require.NotContains(t, excludeHits, "CVZ-107", "lost instrument must be excluded for non-managers")

	// 5) 组合筛选（页面同时带状态 + 排除 + 分类）
	combinedHits, combinedTotal := listStock2577(t, f,
		"page=1&pageSize=100&category_id="+f.childID+"&stock_status=available&exclude_status=lost,archived,sold")
	require.ElementsMatch(t, []string{"CVZ-104", "CVZ-105"}, combinedHits)

	fmt.Printf("SN_HITS=%d\n", len(snHits))
	fmt.Printf("CATEGORY_HITS=%d\n", len(categoryHits))
	fmt.Printf("STATUS_HITS=%d\n", len(statusHits))
	fmt.Printf("EXCLUDE_HITS=%d\n", len(excludeHits))
	fmt.Printf("COMBINED_HITS=%d\n", len(combinedHits))

	totalMatch := 1
	for _, pair := range []struct {
		hits  int
		total int
	}{
		{len(snHits), snTotal},
		{len(categoryHits), categoryTotal},
		{len(statusHits), statusTotal},
		{len(excludeHits), excludeTotal},
		{len(combinedHits), combinedTotal},
	} {
		if pair.hits != pair.total {
			totalMatch = 0
			t.Errorf("total != len(list): hits=%d total=%d", pair.hits, pair.total)
		}
	}
	fmt.Printf("TOTAL_MATCH=%d\n", totalMatch)
}
