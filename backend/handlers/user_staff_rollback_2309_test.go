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
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
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
		"name": "回滚测试", "phone": "13900002309", "email": "rollback2309@example.com",
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
		"name": "绑定失败", "phone": "13900002311", "email": "bindfail2309@example.com",
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
		Status: "pending",
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
		Status: "pending",
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
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002314", Email: "pending2309c@example.com",
		Status: "pending",
	}).Error)

	// 注入 local users 更新失败（模拟 DB 错误），触发第三处回滚。
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_users_update_2309", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("forced users update failure"))
		}
	}))

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
}
