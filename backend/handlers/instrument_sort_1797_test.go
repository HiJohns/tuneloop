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

// TestSortInstrument_FirstUp_Boundary (#1797): 首位上移 → 40002「已在最前」。
func TestSortInstrument_FirstUp_Boundary(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	router := sortRouter(t, tenantID)

	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[0]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "已在最前")
}

// TestSortInstrument_LastDown_Boundary (#1797): 末位下移 → 40002「已在最后」。
func TestSortInstrument_LastDown_Boundary(t *testing.T) {
	tenantID, _, ids := setupSortGroup(t)
	router := sortRouter(t, tenantID)

	body, _ := json.Marshal(map[string]interface{}{"direction": "down"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[2]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "已在最后")
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

// TestSortInstrument_ZeroZero_AssignAdjacent (#1797): 两个 sort_order 均为 0
// 的记录交换时设为相邻序号（保证可重复操作）。
func TestSortInstrument_ZeroZero_AssignAdjacent(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID
	categoryID := uuid.New().String()

	var ids []string
	for _, sn := range []string{"SN-ZZ-1", "SN-ZZ-2"} {
		inst := models.Instrument{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
			CategoryID: &categoryID, SN: sn, SortOrder: 0,
		}
		require.NoError(t, db.Create(&inst).Error)
		ids = append(ids, inst.ID)
	}
	router := sortRouter(t, tenantID)

	// 第 2 个上移（两者均为 0）→ 设为 2/1。
	body, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req := httptest.NewRequest("PUT", "/instruments/"+ids[1]+"/sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var first, second models.Instrument
	require.NoError(t, db.Where("id = ?", ids[0]).First(&first).Error)
	require.NoError(t, db.Where("id = ?", ids[1]).First(&second).Error)
	require.Equal(t, 2, first.SortOrder, "neighbor got moved instrument's old order (2)")
	require.Equal(t, 1, second.SortOrder, "moved instrument now at front (1)")
	// 再次上移（非 0/0 场景仍可交换）：ids[0]（sort_order=2）上移 → 与 ids[1]（1）交换。
	body2, _ := json.Marshal(map[string]interface{}{"direction": "up"})
	req2 := httptest.NewRequest("PUT", "/instruments/"+ids[0]+"/sort", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var first2 models.Instrument
	require.NoError(t, db.Where("id = ?", ids[0]).First(&first2).Error)
	require.Equal(t, 1, first2.SortOrder, "repeatable: ids[0] moved to front (1)")
}
