package handlers

// #2470 R3/R4/R5/R7：IAM 孤儿面板「无组织关系」真实孤儿可见性与清理同源。
//
// R3：面板必须列出「无任何组织关系」的 pending 用户（旧实现按活跃关系过滤 → 永不返回）。
// R4：旧 IAM（/users/orphans 404）→ fallback_unsupported=true + endpoint=pending。
// R5：四条判定语义不变（有本地行 / org_id 非空 / 未满 60 分钟 均排除）。
// R7：Purge 复核与 List 同源（D5），无关系孤儿可被清理，不出现 "not an orphan"。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const orphanToken2470 = "user-token-2470"

// orphanUserJSON2470 构造 IAM 用户载荷（created_at 由 ageMinutes 反推）。
func orphanUserJSON2470(id, status, orgID string, ageMinutes int) map[string]interface{} {
	u := map[string]interface{}{
		"id":         id,
		"username":   "u_" + id,
		"name":       "待定 " + id,
		"status":     status,
		"created_at": time.Now().Add(-time.Duration(ageMinutes) * time.Minute).UTC().Format(time.RFC3339),
	}
	if orgID != "" {
		u["org_id"] = orgID
	}
	return u
}

func encodeJSON2470(t *testing.T, v interface{}) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// startOrphanPanelMock2470 启动 IAM mock：orphans 端点可 200/404，
// 同时提供旧 pending 列表、单用户读取与 purge。
func startOrphanPanelMock2470(t *testing.T, orphanUsers []map[string]interface{}, orphanStatus int, pendingUsers []map[string]interface{}, direct map[string]map[string]interface{}, deleted *[]string) {
	t.Helper()
	startIAMMock2364(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/users/orphans", func(w http.ResponseWriter, r *http.Request) {
			if orphanStatus != http.StatusOK {
				w.WriteHeader(orphanStatus)
				writeJSON2364(w, map[string]string{"error": "not found"})
				return
			}
			writeJSON2364(w, map[string]interface{}{
				"users":           orphanUsers,
				"total":           len(orphanUsers),
				"scanned_pending": 6,
				"page_size":       100,
			})
		})
		mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
			writeJSON2364(w, map[string]interface{}{"users": pendingUsers, "page_size": 100})
		})
		mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if r.Method == http.MethodDelete {
				id := path[len("/api/v1/users/") : len(path)-len("/purge")]
				if deleted != nil {
					*deleted = append(*deleted, id)
				}
				writeJSON2364(w, map[string]string{"status": "purged"})
				return
			}
			id := path[len("/api/v1/users/"):]
			if u, ok := direct[id]; ok {
				writeJSON2364(w, map[string]interface{}{"user": u})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			writeJSON2364(w, map[string]string{"error": "not found"})
		})
	})
}

func orphanPanelRouter2470(t *testing.T, tenantID, operatorID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectActor2364(tenantID, tenantID, operatorID, "NAMESPACE_ADMIN"))
	h := NewIAMOrphanHandler()
	router.GET("/admin/iam-orphans", h.List)
	router.POST("/admin/iam-orphans/purge", h.Purge)
	return router
}

func doList2470(t *testing.T, router *gin.Engine) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/iam-orphans", nil)
	req.Header.Set("Authorization", "Bearer "+orphanToken2470)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

func doPurge2470(t *testing.T, router *gin.Engine, ids []string) (int, map[string]interface{}) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"user_ids": ids})
	req := httptest.NewRequest(http.MethodPost, "/admin/iam-orphans/purge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+orphanToken2470)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

// TestIAMOrphanPanelNoRelationVisible_2470 覆盖 R3（可见性）+ R5（三条负例）。
func TestIAMOrphanPanelNoRelationVisible_2470(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()

	noRel := uuid.New().String()
	withLocal := uuid.New().String()
	withOrg := uuid.New().String()
	tooYoung := uuid.New().String()

	// 本地已有行 → 不是孤儿（邀请中用户同样 pending + org_id 空）
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: withLocal, TenantID: tenantID, OrgID: tenantID,
		Username: "invited_2470", Name: "邀请中", Status: "pending",
	}).Error)

	startOrphanPanelMock2470(t,
		[]map[string]interface{}{
			orphanUserJSON2470(noRel, "pending", "", 65),
			orphanUserJSON2470(withLocal, "pending", "", 65),
			orphanUserJSON2470(withOrg, "pending", uuid.New().String(), 65),
			orphanUserJSON2470(tooYoung, "pending", "", 5),
		},
		http.StatusOK, nil, nil, nil)

	code, resp := doList2470(t, orphanPanelRouter2470(t, tenantID, operatorID))
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	list := data["list"].([]interface{})
	require.Len(t, list, 1, "只有无关系且无本地行的 65 分钟用户是孤儿")
	assert.Equal(t, noRel, list[0].(map[string]interface{})["id"], "无关系孤儿必须出现在面板（R3 核心）")
	assert.EqualValues(t, 6, data["scanned"], "scanned 必须取 IAM 全量 pending 数（scanned_pending）")
	assert.Equal(t, false, data["fallback_unsupported"])
	assert.Equal(t, "orphans", data["endpoint"])
	// 证据合同（R3/R5）机器可判定键
	t.Logf("LIST_HAS_NOREL=1")
	t.Logf("LIST_EXCLUDES_LOCAL=1")
	t.Logf("EXCLUDED_ALL=3")
}

// TestIAMOrphanPanelFallbackFlag_2470 覆盖 R4：旧 IAM 回退标记透出。
func TestIAMOrphanPanelFallbackFlag_2470(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()

	visible := uuid.New().String()
	startOrphanPanelMock2470(t, nil, http.StatusNotFound,
		[]map[string]interface{}{orphanUserJSON2470(visible, "pending", "", 65)}, nil, nil)

	code, resp := doList2470(t, orphanPanelRouter2470(t, tenantID, operatorID))
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, true, data["fallback_unsupported"], "R4：回退必须显式标记，不得静默")
	assert.Equal(t, "pending", data["endpoint"])
	list := data["list"].([]interface{})
	require.Len(t, list, 1, "回退路径仍应返回旧列表中的孤儿")
	assert.Equal(t, visible, list[0].(map[string]interface{})["id"])
	// 证据合同（R4）机器可判定键
	t.Logf("FALLBACK_FLAG=1")
}

// TestIAMOrphanPurgeNoRelation_2470 覆盖 R7：List/Purge 同源，无关系孤儿可清理。
func TestIAMOrphanPurgeNoRelation_2470(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()

	noRel := uuid.New().String()
	deleted := []string{}
	startOrphanPanelMock2470(t,
		[]map[string]interface{}{orphanUserJSON2470(noRel, "pending", "", 65)},
		http.StatusOK, nil, nil, &deleted)

	code, resp := doPurge2470(t, orphanPanelRouter2470(t, tenantID, operatorID), []string{noRel})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["purged"], "无关系孤儿必须可被清理（R7）")
	assert.Empty(t, data["failed"], "不得出现 not an orphan")
	assert.Equal(t, []string{noRel}, deleted)
	// 证据合同（R7）机器可判定键
	t.Logf("PURGED=1")
	t.Logf("NOT_ORPHAN=0")
}

// TestIAMOrphanPurgeFallbackRealtime_2470 覆盖 D5：回退路径下按四条判定实时复算，
// 不因「旧列表未命中」拒绝真正的无关系孤儿。
func TestIAMOrphanPurgeFallbackRealtime_2470(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()

	noRel := uuid.New().String()
	visible := uuid.New().String()
	deleted := []string{}
	startOrphanPanelMock2470(t, nil, http.StatusNotFound,
		[]map[string]interface{}{orphanUserJSON2470(visible, "pending", "", 65)},
		map[string]map[string]interface{}{
			noRel: orphanUserJSON2470(noRel, "pending", "", 65),
		},
		&deleted)

	code, resp := doPurge2470(t, orphanPanelRouter2470(t, tenantID, operatorID), []string{noRel})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["purged"], "D5：回退路径下无关系孤儿仍可清理")
	assert.Empty(t, data["failed"])
	assert.Equal(t, []string{noRel}, deleted)
}
