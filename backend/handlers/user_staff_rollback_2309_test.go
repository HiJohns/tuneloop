package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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

// #2309: 任何“新建 IAM 用户后回滚”的路径都必须 PurgeUser（硬删），
// 不能 DeleteUser（软删，仍占 username/phone/email 唯一键）。

type iamRollbackMock2309 struct {
	srv       *httptest.Server
	purge     int32
	soft      int32
	creates   int32
	binds     int32
	unbinds   int32
	perms     int32
	failBind  bool
	failPerms bool
}

func newIAMRollbackMock2309(t *testing.T, newUserID string) *iamRollbackMock2309 {
	t.Helper()
	m := &iamRollbackMock2309{}
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2309", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/perm-version/increment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"ok": true})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			atomic.AddInt32(&m.creates, 1)
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID, "status": "active"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/purge") {
			atomic.AddInt32(&m.purge, 1)
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&m.soft, 1)
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID}})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/customer-permissions") {
			atomic.AddInt32(&m.perms, 1)
			if m.failPerms {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, map[string]interface{}{"error": "permission update failed"})
				return
			}
			writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
			return
		}
		// bind / unbind 共用同一 path，只能按 body 的 action 区分（#2555 B3）。
		var body struct {
			Action string `json:"action"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		switch body.Action {
		case "unbind":
			atomic.AddInt32(&m.unbinds, 1)
		case "bind":
			atomic.AddInt32(&m.binds, 1)
		}
		if m.failBind {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"error": "bind failed"})
			return
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	// #2555 D3：与 #2540 的 `12cc82cec` 同内容——激活路径可能先查角色模板，
	// 缺模板会让「本地更新失败」用例提前止步于模板查询（假绿）。
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/role-templates") {
			writeJSON(w, map[string]interface{}{
				"role_templates": []map[string]interface{}{
					{"id": "rt-site_member", "code": "site_member"},
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
	})
	m.srv = httptest.NewServer(mux)
	return m
}

func (m *iamRollbackMock2309) close() { m.srv.Close() }
func (m *iamRollbackMock2309) purges() int32 {
	return atomic.LoadInt32(&m.purge)
}
func (m *iamRollbackMock2309) softDeletes() int32 {
	return atomic.LoadInt32(&m.soft)
}
func (m *iamRollbackMock2309) counts() (creates, binds, unbinds, perms int32) {
	return atomic.LoadInt32(&m.creates), atomic.LoadInt32(&m.binds),
		atomic.LoadInt32(&m.unbinds), atomic.LoadInt32(&m.perms)
}

func setupStaffRollback2309(t *testing.T) (*gorm.DB, string, string, string, *services.IAMClient, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: orgID,
		Username: "op2309", Role: "admin", Status: "active",
	}).Error)
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)
	r.POST("/users/:id/activate", h.ActivateUser)
	return db, tenantID, orgID, operatorID, nil, r
}

func TestCreateUser2309_LocalTxFailurePurgesIAM(t *testing.T) {
	db, tenantID, orgID, operatorID, _, r := setupStaffRollback2309(t)
	_ = db
	_ = tenantID
	_ = orgID
	_ = operatorID

	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	raw, _ := json.Marshal(map[string]interface{}{
		"name": "回滚测试", "username": "rollback2309", "phone": "13900002309", "email": "rollback2309@example.com",
		"user_type": "merchant_direct",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, m.purges(), "local tx failure must call PurgeUser")
	assert.EqualValues(t, 0, m.softDeletes(), "local tx failure must not call DeleteUser")
}

func TestCreateUser2309_BindFailurePurgesIAM(t *testing.T) {
	_, _, _, _, _, r := setupStaffRollback2309(t)
	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	m.failBind = true
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	raw, _ := json.Marshal(map[string]interface{}{
		"name": "绑定失败", "username": "bindfail2309", "phone": "13900002311", "email": "bindfail2309@example.com",
		"user_type": "merchant_direct",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, m.purges(), "bind failure must call PurgeUser")
	assert.EqualValues(t, 0, m.softDeletes(), "bind failure must not call DeleteUser")
}

func TestActivateUser2309_BindFailurePurgesIAM(t *testing.T) {
	db, tenantID, orgID, _, _, r := setupStaffRollback2309(t)
	pendingID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002312", Email: "pending2309@example.com",
		Role: "site_member", Status: "pending",
	}).Error)

	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	m.failBind = true
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+pendingID+"/activate", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, m.purges(), "bind failure must call PurgeUser")
	assert.EqualValues(t, 0, m.softDeletes(), "bind failure must not call DeleteUser")

	creates, binds, unbinds, perms := m.counts()
	fmt.Printf("ACTIVATE2309_CASE=bind_failure HTTP=%d PURGE=%d SOFT=%d IAM_CREATE=%d IAM_BIND=%d IAM_PERM=%d IAM_UNBIND=%d\n",
		w.Code, m.purges(), m.softDeletes(), creates, binds, perms, unbinds)
}

func TestActivateUser2309_PermissionFailurePurgesIAM(t *testing.T) {
	db, tenantID, orgID, _, _, r := setupStaffRollback2309(t)
	pendingID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002313", Email: "pending2309b@example.com",
		Role: "site_member", Status: "pending",
	}).Error)

	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	m.failPerms = true
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+pendingID+"/activate", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, m.purges(), "permission failure must call PurgeUser")
	assert.EqualValues(t, 0, m.softDeletes(), "permission failure must not call DeleteUser")

	creates, binds, unbinds, perms := m.counts()
	fmt.Printf("ACTIVATE2309_CASE=permission_failure HTTP=%d PURGE=%d SOFT=%d IAM_CREATE=%d IAM_BIND=%d IAM_PERM=%d IAM_UNBIND=%d\n",
		w.Code, m.purges(), m.softDeletes(), creates, binds, perms, unbinds)
}

func TestActivateUser2309_LocalUpdateFailurePurgesIAM(t *testing.T) {
	db, tenantID, orgID, _, _, r := setupStaffRollback2309(t)
	pendingID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002314", Email: "pending2309c@example.com",
		Role: "site_member", Status: "pending",
	}).Error)

	// 注入 local users 更新失败（模拟 DB 错误），触发第三处回滚。
	var localUpdateAttempts int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_users_update_2309", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			atomic.AddInt32(&localUpdateAttempts, 1)
			tx.AddError(errors.New("forced users update failure"))
		}
	}))
	// #2336: GORM callbacks live on the shared *gorm.DB registry. Without this
	// cleanup every later users UPDATE in the process fails, which made
	// TestStaffOrderActions / TestUpdateUser_SiteIDNoChange fail only in the
	// full suite (order-dependent pollution).
	t.Cleanup(func() {
		db.Callback().Update().Remove("fail_users_update_2309")
	})

	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+pendingID+"/activate", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, m.purges(), "local update failure must call PurgeUser")
	assert.EqualValues(t, 0, m.softDeletes(), "local update failure must not call DeleteUser")

	creates, binds, unbinds, perms := m.counts()
	fmt.Printf("ACTIVATE2309_CASE=local_update_failure HTTP=%d PURGE=%d SOFT=%d IAM_CREATE=%d IAM_BIND=%d IAM_PERM=%d IAM_UNBIND=%d LOCAL_UPDATE_ATTEMPT=%d\n",
		w.Code, m.purges(), m.softDeletes(), creates, binds, perms, unbinds, localUpdateAttempts)
	assert.EqualValues(t, 1, localUpdateAttempts, "the third rollback must be triggered by a real users UPDATE attempt")
}

// R2（#2555）：mock 的 role-templates 必须能被真实客户端解析。
// 旧响应 `{"data": []}` 会让 ListRoleTemplates 解析失败，进而让「本地更新失败」用例在
// 模板查询处提前回滚（假绿）。这里直连 mock，不依赖 #2540 是否已合入。
func TestActivateUser2309_IAMMockServesRoleTemplates(t *testing.T) {
	newUserID := uuid.New().String()
	m := newIAMRollbackMock2309(t, newUserID)
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	templates, err := services.NewIAMClient().ListRoleTemplates("ns-2555")
	require.NoError(t, err, "mock must serve a parseable role_templates payload")

	code := ""
	if len(templates) == 1 {
		code = templates[0].Code
	}
	fmt.Printf("IAM_ROLE_TEMPLATE_COUNT=%d IAM_ROLE_TEMPLATE_CODE=%s\n", len(templates), code)
	require.Equal(t, 1, len(templates))
	require.Equal(t, "site_member", code)
}

// R3（#2555）：红条件固定 —— 空 role 行仍按 #2526 判纯顾客（404 且不打 IAM），
// 员工 role 行才可达回滚分支。
func TestActivateUser2309_FixtureVisibilityPrecondition(t *testing.T) {
	db, tenantID, orgID, _, _, r := setupStaffRollback2309(t)
	customerID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: customerID, TenantID: tenantID, OrgID: orgID,
		Name: "空角色", Phone: "13900002315", Email: "emptyrole2309@example.com",
		Status: "pending",
	}).Error)

	newUserID := uuid.New().String()
	customerMock := newIAMRollbackMock2309(t, newUserID)
	defer customerMock.close()
	services.SetIAMInternalURLForTesting(customerMock.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/users/"+customerID+"/activate", nil))
	customerCreates, _, _, _ := customerMock.counts()

	// users.iam_sub 有唯一约束，两行都不能留空串；先清掉红条件行再建绿条件行
	// （同一 DB/路由，仅顺序执行，语义不变）。
	require.NoError(t, db.Exec("DELETE FROM users WHERE id = ?", customerID).Error)
	staffID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: staffID, TenantID: tenantID, OrgID: orgID,
		Name: "员工角色", Phone: "13900002316", Email: "staffrole2309@example.com",
		Role: "site_member", Status: "pending",
	}).Error)

	staffMock := newIAMRollbackMock2309(t, uuid.New().String())
	staffMock.failBind = true
	defer staffMock.close()
	services.SetIAMInternalURLForTesting(staffMock.srv.URL)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/users/"+staffID+"/activate", nil))

	fmt.Printf("EMPTY_ROLE_STATUS=%d EMPTY_ROLE_IAM_CREATE=%d STAFF_ROLE_STATUS=%d STAFF_ROLE_PURGE=%d\n",
		w.Code, customerCreates, w2.Code, staffMock.purges())
	require.Equal(t, http.StatusNotFound, w.Code)
	require.EqualValues(t, 0, customerCreates, "an invisible customer row must not reach IAM")
	require.Equal(t, http.StatusInternalServerError, w2.Code)
	require.EqualValues(t, 1, staffMock.purges())
}
