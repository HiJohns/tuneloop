package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1797 tests: SortInstrument — 子分类内排序（上移/下移交换 sort_order）。

// setupSortGroup 创建同 category 的 3 个乐器（sort_order 1/2/3）+ 1 个异分类乐器。
func setupSortGroup(t *testing.T) (string, string, []string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID
	categoryID := uuid.New().String()

	var ids []string
	orders := []int{1, 2, 3}
	for i, so := range orders {
		inst := models.Instrument{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
			CategoryID: &categoryID,
			SN:         "SN-SORT-" + string(rune('A'+i)),
			SortOrder:  so,
		}
		require.NoError(t, db.Create(&inst).Error)
		ids = append(ids, inst.ID)
	}
	// 异分类乐器（不应参与同组排序）。
	otherCat := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		CategoryID: &otherCat, SN: "SN-SORT-OTHER", SortOrder: 1,
	}).Error)

	return tenantID, categoryID, ids
}

func sortRouter(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		if tenantID != "" {
			// #2288: 固定商户管理员范围，避免共享池场景下受前序用例上下文影响。
			ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
			ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		}
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", tenantID)
		c.Next()
	})
	router.PUT("/instruments/:id/sort", SortInstrument)
	return router
}

// TestSortInstrument_Up_Exchange (#1797): 上移 — 与上一相邻记录交换 sort_order。
func TestSortInstrument_Up_Exchange(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	db := database.GetDB()
	router := sortRouter(t, tenantID)

	// 上移第 2 个（sort_order=2 → 与 sort_order=1 交换）。
	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[1]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code      int `json:"code"`
		SortOrder int `json:"sort_order"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 1, resp.SortOrder, "moved instrument now has neighbor's sort_order")

	var first, second models.Instrument
	require.NoError(t, db.Where("id = ?", ids[0]).First(&first).Error)
	require.NoError(t, db.Where("id = ?", ids[1]).First(&second).Error)
	require.Equal(t, 2, first.SortOrder, "neighbor took the moved instrument's old order")
	require.Equal(t, 1, second.SortOrder)
}

// TestSortInstrument_Down_Exchange (#1797): 下移 — 与下一相邻记录交换。
func TestSortInstrument_Down_Exchange(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	db := database.GetDB()
	router := sortRouter(t, tenantID)

	body, _ := json.Marshal(map[string]interface{}{"direction": "down"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[0]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var second models.Instrument
	require.NoError(t, db.Where("id = ?", ids[1]).First(&second).Error)
	require.Equal(t, 1, second.SortOrder, "neighbor took the moved instrument's old order")
}

// TestSortInstrument_FirstUp_Boundary (#1797/#2241): 首位上移 → 40002 + 英文机器可读 message。
func TestSortInstrument_FirstUp_Boundary(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	router := sortRouter(t, tenantID)

	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[0]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "already first")
}

// TestSortInstrument_LastDown_Boundary (#1797/#2241): 末位下移 → 40002 + 英文机器可读 message。
func TestSortInstrument_LastDown_Boundary(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	router := sortRouter(t, tenantID)

	body, _ := json.Marshal(map[string]interface{}{"direction": "down"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[2]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "already last")
}

// TestSortInstrument_TenantIsolation (#1797): 其他租户的乐器 → 404（租户隔离）。
func TestSortInstrument_TenantIsolation(t *testing.T) {
	_, _, ids := setupSortGroup(t)
	// 另一个租户操作同一个 id → 40400（查不到该租户下的乐器）。
	otherTenant := uuid.New().String()
	router := sortRouter(t, otherTenant)

	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[0]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// TestSortInstrument_ZeroGroup_Renumbered (#2241): 大量 sort_order=0 的组，
// 移动后整组重排为 1..N（消除 0→1/2 值域跳变），且不产生跨组位移。
func TestSortInstrument_ZeroGroup_Renumbered(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID
	categoryID := uuid.New().String()
	otherCat := uuid.New().String()

	var ids []string
	for _, sn := range []string{"SN-ZZ-1", "SN-ZZ-2", "SN-ZZ-3"} {
		inst := models.Instrument{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
			CategoryID: &categoryID, SN: sn, SortOrder: 0,
		}
		require.NoError(t, db.Create(&inst).Error)
		ids = append(ids, inst.ID)
	}
	// 异分类 0 值行：不得被本组重排影响。
	other := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		CategoryID: &otherCat, SN: "SN-ZZ-OTHER", SortOrder: 0,
	}
	require.NoError(t, db.Create(&other).Error)

	router := sortRouter(t, tenantID)
	// 可见顺序（sort_order ASC, created_at DESC, id ASC）= ids[2], ids[1], ids[0]；
	// 上移中间项 → 与 ids[2] 换位。
	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[1]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	orders := map[string]int{}
	var rows []models.Instrument
	require.NoError(t, db.Where("category_id = ?", categoryID).Find(&rows).Error)
	for _, r := range rows {
		orders[r.ID] = r.SortOrder
	}
	require.Equal(t, 1, orders[ids[1]], "moved row should be first")
	require.Equal(t, 2, orders[ids[2]], "neighbor moved down to 2")
	require.Equal(t, 3, orders[ids[0]], "untouched row renumbered to 3 (no 0 left)")
	for _, so := range orders {
		require.NotZero(t, so, "no zero sort_order remains in group")
	}

	var otherAfter models.Instrument
	require.NoError(t, db.Where("id = ?", other.ID).First(&otherAfter).Error)
	require.Equal(t, 0, otherAfter.SortOrder, "other category must not be renumbered")
}

// TestSortInstrument_TopLevelAdmin_NoTenant (#2241): 顶层管理员（tid 空）在可见范围内
// 可排序，不再 40400。
func TestSortInstrument_TopLevelAdmin_NoTenant(t *testing.T) {
	tenantID, categoryID, ids := setupSortGroup(t)
	_ = categoryID
	router := sortRouter(t, "")

	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[1]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	db := database.GetDB()
	var moved models.Instrument
	require.NoError(t, db.Where("id = ? AND tenant_id = ?", ids[1], tenantID).First(&moved).Error)
	require.Equal(t, 1, moved.SortOrder)
}

// TestGetInstruments_DefaultGroupedOrder (#2241): 默认视图按分类分组
// （category_id, sort_order, created_at DESC, id ASC），组内排序不再被其他分类的
// 0 值记录打断。
func TestGetInstruments_DefaultGroupedOrder(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID
	catA, catB := uuid.New().String(), uuid.New().String()
	if catA > catB {
		catA, catB = catB, catA
	}

	mk := func(sn string, cat string, sort int) models.Instrument {
		inst := models.Instrument{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
			CategoryID: &cat, SN: sn, SortOrder: sort, StockStatus: "available",
		}
		require.NoError(t, db.Create(&inst).Error)
		return inst
	}
	a1 := mk("SN-G-A1", catA, 5)
	a2 := mk("SN-G-A2", catA, 1)
	b1 := mk("SN-G-B1", catB, 1)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", tenantID)
		c.Next()
	})
	router.GET("/instruments", GetInstruments)

	req := httptest.NewRequest("GET", "/instruments?page=1&pageSize=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	var got []string
	for _, it := range resp.Data.List {
		got = append(got, it.ID)
	}
	require.Equal(t, []string{a2.ID, a1.ID, b1.ID}, got,
		"default order must group by category (A: sort 1,5 then B: sort 1), not global sort_order")
}
