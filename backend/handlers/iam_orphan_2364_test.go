package handlers

// #2369（#2364 C）：IAM 孤儿账号诊断/清理。
// R1 回滚失败 → 响应带 orphan 标记 + 结构化日志（create_user_orphan）。
// R2 列表：跨页聚合（25 pending / 2 页，23 孤儿）+ 权限（仅系统管理员）。
// R3 清理：2 个孤儿 → purged=2、failed=[]、notes 含 user_task_queues；本地存在的 ID 拒绝（D5）。

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func writeJSON2364(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// startIAMMock2364 启动带 client token 的 IAM mock，并注册自定义路由。
func startIAMMock2364(t *testing.T, register func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON2364(w, map[string]interface{}{"access_token": "svc-2364", "expires_in": 7200, "token_type": "Bearer"})
	})
	register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	return srv
}

func injectActor2364(tenantID, orgID, userID, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: role}.InjectContext(c.Request.Context()))
		c.Next()
	}
}

// 本地事务失败注入：merchant_direct 但本地没有商户行 → transaction 返回
// "merchant not found for operator scope"；此时 IAM 用户已创建并绑定。
func TestCreateUserOrphanMark_2364(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2364", Role: "ADMIN", Status: "active",
	}).Error)

	iamUserID := uuid.New().String()
	purgeCalls := 0
	startIAMMock2364(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				// is_primary=false → 不被 #2247 误判为平台管理员
				writeJSON2364(w, map[string]interface{}{"id": tenantID, "name": "MerchantOrg", "is_primary": false})
				return
			}
			writeJSON2364(w, map[string]interface{}{"success": true})
		})
		mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
			writeJSON2364(w, map[string]interface{}{"data": map[string]interface{}{
				"user_id": iamUserID, "status": "pending",
			}})
		})
		mux.HandleFunc("/api/v1/users/"+iamUserID+"/purge", func(w http.ResponseWriter, r *http.Request) {
			purgeCalls++
			w.WriteHeader(http.StatusForbidden)
			writeJSON2364(w, map[string]string{"error": "only namespace admin can purge users"})
		})
		mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeJSON2364(w, map[string]string{"error": "not found"})
		})
	})

	var logBuf bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldWriter) })

	router := gin.New()
	router.Use(injectActor2364(tenantID, tenantID, operatorID, "ADMIN"))
	h := &UserStaffHandler{}
	router.POST("/users", h.CreateUser)

	body, _ := json.Marshal(map[string]interface{}{
		"user_type":     "merchant_direct",
		"username":      "orphan_2364",
		"name":          "孤儿标记",
		"phone":         "13800002364",
		"auto_generate": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "回滚失败必须给出可诊断 data：%v", resp)
	assert.Equal(t, true, data["orphan"], "必须显式标记 orphan=true")
	assert.Equal(t, iamUserID, data["iam_user_id"], "必须回传未能清理的 IAM user id")
	assert.Equal(t, 1, purgeCalls, "回滚应尝试 purge 一次")

	logs := logBuf.String()
	assert.Contains(t, logs, "create_user_orphan", "日志必须含可检索标记 create_user_orphan")
	assert.Contains(t, logs, iamUserID, "日志必须带 IAM user id")

	// 本地事务回滚 → 不得留下 users 行
	var n int64
	require.NoError(t, db.Model(&models.User{}).Where("iam_sub = ?", iamUserID).Count(&n).Error)
	assert.Zero(t, n)
}

// orphanFixture2364 构造 25 个 pending 用户：23 孤儿（无本地行）+ 2 有本地行。
func orphanFixture2364(t *testing.T, db *gorm.DB, tenantID string) (pending []map[string]interface{}, orphanIDs []string, localIDs []string) {
	t.Helper()
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	for i := 0; i < 25; i++ {
		id := uuid.New().String()
		pending = append(pending, map[string]interface{}{
			"id":         id,
			"username":   "pending_" + strconv.Itoa(i),
			"name":       "待定 " + strconv.Itoa(i),
			"email":      "pending" + strconv.Itoa(i) + "@example.com",
			"status":     "pending",
			"created_at": old,
		})
		if i < 23 {
			orphanIDs = append(orphanIDs, id)
		} else {
			localIDs = append(localIDs, id)
			require.NoError(t, db.Create(&models.User{
				ID: uuid.New().String(), IAMSub: id, TenantID: tenantID, OrgID: tenantID,
				Username: "local_" + strconv.Itoa(i), Name: "邀请中", Status: "pending",
			}).Error)
		}
	}
	return pending, orphanIDs, localIDs
}

// mockPagedPending2364 模拟 IAM 上限 20 条/页（服务端 page_size=20），
// 并要求客户端显式传 status=pending、include_orgs=true、page_size=100、page=N。
func mockPagedPending2364(t *testing.T, pending []map[string]interface{}, gotQueries *[]url.Values) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON2364(w, map[string]interface{}{"data": map[string]interface{}{"user_id": uuid.New().String(), "status": "pending"}})
			return
		}
		q := r.URL.Query()
		*gotQueries = append(*gotQueries, q)
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		const serverCap = 20
		start := (page - 1) * serverCap
		end := start + serverCap
		if start > len(pending) {
			start = len(pending)
		}
		if end > len(pending) {
			end = len(pending)
		}
		writeJSON2364(w, map[string]interface{}{
			"users":     pending[start:end],
			"total":     len(pending),
			"page":      page,
			"page_size": serverCap,
		})
	}
}

func TestListIAMOrphans_2364(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()
	pending, orphanIDs, _ := orphanFixture2364(t, db, tenantID)

	var gotQueries []url.Values
	startIAMMock2364(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/users", mockPagedPending2364(t, pending, &gotQueries))
	})

	router := gin.New()
	router.Use(injectActor2364(tenantID, tenantID, operatorID, "NAMESPACE_ADMIN"))
	h := NewIAMOrphanHandler()
	router.GET("/admin/iam-orphans", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/iam-orphans", nil)
	req.Header.Set("Authorization", "Bearer user-token-2364")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	list := data["list"].([]interface{})
	assert.Equal(t, 23, len(list), "两页聚合应返回 23 个孤儿（25 pending - 2 本地已登记）")
	assert.EqualValues(t, 23, data["total"])
	assert.EqualValues(t, 25, data["scanned"])

	gotIDs := make(map[string]bool, len(list))
	for _, it := range list {
		gotIDs[it.(map[string]interface{})["id"].(string)] = true
	}
	for _, id := range orphanIDs {
		assert.True(t, gotIDs[id], "孤儿 %s 必须出现在列表", id)
	}

	// D6：必须显式分页参数，且实际翻到第二页（mock 上限 20）
	require.Len(t, gotQueries, 2, "必须请求两页（page=1,2）")
	for i, q := range gotQueries {
		assert.Equal(t, "pending", q.Get("status"), "第 %d 页必须带 status=pending", i+1)
		assert.Equal(t, "true", q.Get("include_orgs"), "第 %d 页必须带 include_orgs=true", i+1)
		assert.Equal(t, "100", q.Get("page_size"), "第 %d 页必须显式 page_size=100（不得依赖 IAM 默认 20）", i+1)
		assert.Equal(t, strconv.Itoa(i+1), q.Get("page"), "第 %d 页 page 参数", i+1)
	}

	// 权限：商户管理员 403 40300（且不触发 IAM 调用）
	before := len(gotQueries)
	merchantRouter := gin.New()
	merchantRouter.Use(injectActor2364(tenantID, tenantID, operatorID, "ADMIN"))
	merchantRouter.GET("/admin/iam-orphans", h.List)
	req2 := httptest.NewRequest(http.MethodGet, "/admin/iam-orphans", nil)
	req2.Header.Set("Authorization", "Bearer user-token-2364")
	w2 := httptest.NewRecorder()
	merchantRouter.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusForbidden, w2.Code)
	var resp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.EqualValues(t, 40300, resp2["code"])
	assert.Equal(t, before, len(gotQueries), "权限拒绝后不得再调用 IAM")
}

func TestPurgeIAMOrphans_2364(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	operatorID := uuid.New().String()
	pending, orphanIDs, localIDs := orphanFixture2364(t, db, tenantID)
	require.Len(t, orphanIDs, 23)

	var gotQueries []url.Values
	purgedIDs := map[string]int{}
	startIAMMock2364(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/users", mockPagedPending2364(t, pending, &gotQueries))
		mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			id := r.URL.Path
			id = id[len("/api/v1/users/") : len(id)-len("/purge")]
			assert.Equal(t, "user-token-2364", r.Header.Get("Authorization")[len("Bearer "):], "必须用管理员用户 token 调用 purge")
			purgedIDs[id]++
			writeJSON2364(w, map[string]string{"status": "purged"})
		})
	})

	router := gin.New()
	router.Use(injectActor2364(tenantID, tenantID, operatorID, "NAMESPACE_ADMIN"))
	h := NewIAMOrphanHandler()
	router.POST("/admin/iam-orphans/purge", h.Purge)

	doPurge := func(userIDs []string) (int, map[string]interface{}) {
		body, _ := json.Marshal(map[string]interface{}{"user_ids": userIDs})
		req := httptest.NewRequest(http.MethodPost, "/admin/iam-orphans/purge", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer user-token-2364")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
		return w.Code, resp
	}

	code, resp := doPurge(orphanIDs[:2])
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.EqualValues(t, 2, data["purged"])
	assert.Empty(t, data["failed"], "两个合法孤儿不得有任何失败项")
	notes, _ := json.Marshal(data["notes"])
	assert.Contains(t, string(notes), "user_task_queues", "notes 必须提示 IAM 队列清理（D3/G5）")
	assert.Equal(t, 1, purgedIDs[orphanIDs[0]])
	assert.Equal(t, 1, purgedIDs[orphanIDs[1]])

	// D5：本地存在的 ID 一律拒绝（即使 IAM 侧 pending）
	code2, resp2 := doPurge([]string{localIDs[0]})
	require.Equal(t, http.StatusOK, code2)
	data2 := resp2["data"].(map[string]interface{})
	assert.EqualValues(t, 0, data2["purged"])
	failed := data2["failed"].([]interface{})
	require.Len(t, failed, 1)
	assert.Equal(t, localIDs[0], failed[0].(map[string]interface{})["user_id"])
	assert.Contains(t, failed[0].(map[string]interface{})["error"], "user exists locally")
	assert.Zero(t, purgedIDs[localIDs[0]], "本地存在用户不得被 purge")

	// 空 user_ids → 400（不静默成功）
	code3, resp3 := doPurge([]string{})
	assert.Equal(t, http.StatusBadRequest, code3)
	assert.EqualValues(t, 40001, resp3["code"])
}
