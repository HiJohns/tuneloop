package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
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
		if strings.HasSuffix(r.URL.Path, "/customer-permissions") && m.failPerms {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"error": "permission update failed"})
			return
		}
		if m.failBind {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"error": "bind failed"})
			return
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	// #2565：激活路径在「可见性 → 本地 pending 校验」之后会拉取命名空间角色模板并分配。
	// 旧 mock 返回 `{"data": []}` → ListRoleTemplates 解析失败（wrapped.role_templates 与
	// 顶层数组都取不到）→ ActivateUser 在「角色模板缺失」回滚处就返回 500，
	// 与用例名（bind / 权限 / 本地写回失败）不符，形成「名称 ≠ 覆盖」的假绿。
	// 这里返回 IAM 真实形态的模板数组，让三条激活用例各自走到目标失败分支。
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]interface{}{{"id": "rt-site-member", "code": "site_member"}})
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
	// #2565 D2：夹具必须是 #2462 D1 之后真实员工形态（`users.role` 恒写员工角色）。
	// 空 role 会被 #2526 的 isCustomerUserRole 判为顾客 → by-id 动作一律 404，
	// 于是本用例会停在可见性闸门、永远验不到「bind 失败后 PurgeUser」。
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
}

func TestActivateUser2309_LocalUpdateFailurePurgesIAM(t *testing.T) {
	db, tenantID, orgID, _, _, r := setupStaffRollback2309(t)
	pendingID := uuid.New().String()
	// #2565 D2/D6：真实员工形态（role 非空）+ 把「本地写回失败」分支真正走到。
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002314", Email: "pending2309c@example.com",
		Role: "site_member", Status: "pending",
	}).Error)

	// 注入 local users 更新失败（模拟 DB 错误），触发第三处回滚。
	// #2565 D6：用原子计数证明该回调**确实被命中**——否则本用例会在更早的
	// 「角色模板缺失」处 500 返回，名称写「本地写回失败」却从未覆盖该分支。
	var localWritebackAttempts int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_users_update_2309", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			atomic.AddInt32(&localWritebackAttempts, 1)
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
	require.GreaterOrEqual(t, atomic.LoadInt32(&localWritebackAttempts), int32(1),
		"local write-back branch must be reached (users UPDATE callback hit)")
	t.Logf("LOCAL_WRITEBACK_ATTEMPT=%d", atomic.LoadInt32(&localWritebackAttempts))
}
