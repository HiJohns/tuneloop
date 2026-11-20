package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2427：人员管理筛选参数契约
//   R3 非法 site_id（"null"/"abc"）→ 40002，不得把非法值带进 SQL 后静默返回 0 行；
//   R4 省略 site_id → 全量（total>0）；合法 site_id 仍按网点过滤。

type personnel2427Response struct {
	Code int `json:"code"`
	Data struct {
		List  []map[string]interface{} `json:"list"`
		Total int                      `json:"total"`
	} `json:"data"`
}

func personnel2427Get(t *testing.T, r *gin.Engine, query string) (int, personnel2427Response) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/personnel?"+query, nil))
	var resp personnel2427Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestPersonnelFilter2427_InvalidSiteID_40002(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	for _, bad := range []string{"null", "abc", "undefined", "123"} {
		code, resp := personnel2427Get(t, r, "actor=merchant&site_id="+bad)
		require.Equal(t, http.StatusBadRequest, code, "site_id=%s 必须 400（而不是静默 0 行）", bad)
		assert.Equal(t, 40002, resp.Code, "site_id=%s 必须返回 40002", bad)
		assert.Empty(t, resp.Data.List, "非法参数不得返回列表数据（避免被当成空列表）")
		assert.Equal(t, 0, resp.Data.Total)
	}
}

func TestPersonnelFilter2427_EmptySiteReturnsAll(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	code, resp := personnel2427Get(t, r, "actor=merchant")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 20000, resp.Code)
	assert.Greater(t, resp.Data.Total, 0, "省略 site_id 必须返回全量（非 0 条）")
	require.NotEmpty(t, resp.Data.List)

	names := map[string]bool{}
	for _, row := range resp.Data.List {
		if n, ok := row["name"].(string); ok {
			names[n] = true
		}
	}
	assert.True(t, names["网点A成员"], "全量结果应含网点成员")
	assert.True(t, names["直属员工"], "全量结果应含商户直属员工")
}

func TestPersonnelFilter2427_ValidSiteFilterStillWorks(t *testing.T) {
	r, f := setupPersonnel2065(t)
	code, resp := personnel2427Get(t, r, "actor=merchant&site_id="+f.siteA)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 20000, resp.Code)
	require.NotEmpty(t, resp.Data.List, "合法 site_id 必须仍能过滤出该网点成员")
	for _, row := range resp.Data.List {
		assert.Equal(t, "网点A", row["site_name"], "合法 site_id 必须只返回该网点成员")
	}
}
