package handlers

import (
	"bytes"
	"encoding/json"
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
)

// #2309: 任何“新建 IAM 用户后回滚”的路径都必须 PurgeUser（硬删），
// 不能 DeleteUser（软删，仍占 username/phone/email 唯一键）。

func newIAMRollbackMock2309(t *testing.T, newUserID string) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var purgeCount int32
	var deleteCount int32
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2309", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID, "status": "active"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/purge") {
			atomic.AddInt32(&purgeCount, 1)
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deleteCount, 1)
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID}})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		// bind 失败 → 触发回滚；GetOrganization 也走这里，返回空对象即可。
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]interface{}{"error": "bind failed"})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
	})
	return httptest.NewServer(mux), &purgeCount, &deleteCount
}

func TestCreateUser2309_LocalTxFailurePurgesIAM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: orgID,
		Username: "op2309", Role: "admin", Status: "active",
	}).Error)

	newUserID := uuid.New().String()
	srv, purgeCount, deleteCount := newIAMRollbackMock2309(t, newUserID)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	// merchant_direct 需要本地 merchant；测试库没有 → 本地事务失败 → 必须 purge。
	raw, _ := json.Marshal(map[string]interface{}{
		"name": "回滚测试", "phone": "13900002309", "email": "rollback2309@example.com",
		"user_type": "merchant_direct",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, atomic.LoadInt32(purgeCount), "rollback must call PurgeUser")
	assert.EqualValues(t, 0, atomic.LoadInt32(deleteCount), "rollback must not call DeleteUser")
}

func TestActivateUser2309_BindFailurePurgesIAM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: orgID,
		Username: "op2309a", Role: "admin", Status: "active",
	}).Error)

	pendingID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "待激活", Phone: "13900002310", Email: "pending2309@example.com",
		Status: "pending",
	}).Error)

	newUserID := uuid.New().String()
	srv, purgeCount, deleteCount := newIAMRollbackMock2309(t, newUserID)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users/:id/activate", h.ActivateUser)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+pendingID+"/activate", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.EqualValues(t, 1, atomic.LoadInt32(purgeCount), "bind failure must call PurgeUser")
	assert.EqualValues(t, 0, atomic.LoadInt32(deleteCount), "bind failure must not call DeleteUser")
}
