package handlers

// #2375：创建时角色分配失败不阻断（role_errors）；管理员改角色落库 + IAM 同步；
// 白名单拒绝提权 + 跨租户 404。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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

type iamRoleMock2375 struct {
	mu        sync.Mutex
	roleCalls int
	tmplCalls int
}

func setupRoleMock2375(t *testing.T, rec *iamRoleMock2375, templatesStatus int) {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2375", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, map[string]interface{}{"data": []interface{}{}})
			return
		}
		writeJSON(w, map[string]interface{}{"user_id": uuid.New().String(), "status": "pending"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rec.mu.Lock()
			rec.tmplCalls++
			rec.mu.Unlock()
			writeJSON(w, map[string]interface{}{"code": 20000})
			return
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && bytes.Contains([]byte(r.URL.Path), []byte("/role")) {
			rec.mu.Lock()
			rec.roleCalls++
			rec.mu.Unlock()
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if templatesStatus != http.StatusOK {
			w.WriteHeader(templatesStatus)
			writeJSON(w, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, []map[string]string{{"id": "tmpl-2375", "code": "repair_technician"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
}

func roleRouter2375(tenantID, userID, role string, register func(r *gin.Engine, h *UserStaffHandler)) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: userID, Role: role}.InjectContext(c.Request.Context()))
		c.Next()
	})
	register(r, &UserStaffHandler{})
	return r
}

func TestCreateUserRoleErrors_2375(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	opID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: opID, IAMSub: opID, TenantID: tenantID, OrgID: tenantID, Username: "op2375", Role: "ADMIN", Status: "active",
	}).Error)
	rec := &iamRoleMock2375{}
	setupRoleMock2375(t, rec, http.StatusNotFound)

	r := roleRouter2375(tenantID, opID, "ADMIN", func(r *gin.Engine, h *UserStaffHandler) {
		r.POST("/users", h.CreateUser)
	})
	body, _ := json.Marshal(map[string]interface{}{
		"username": "roleerr2375", "name": "角色失败", "auto_generate": true, "user_type": "repair_technician",
	})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	roleErrors, ok := data["role_errors"].([]interface{})
	require.True(t, ok, "role_errors must be present: %v", data)
	assert.GreaterOrEqual(t, len(roleErrors), 1)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&count).Error)
	assert.EqualValues(t, 2, count, "operator + created user (creation must not roll back)")
}

func TestUpdateUserRole_2375(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	targetID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: targetID, IAMSub: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		Username: "u2375", Name: "改角色", Role: "site_member", Status: "active",
	}).Error)
	rec := &iamRoleMock2375{}
	setupRoleMock2375(t, rec, http.StatusOK)

	r := roleRouter2375(tenantID, uuid.New().String(), "ADMIN", func(r *gin.Engine, h *UserStaffHandler) {
		r.PUT("/users/:id", h.UpdateUser)
	})
	body, _ := json.Marshal(map[string]interface{}{"role": "repair_technician"})
	req := httptest.NewRequest(http.MethodPut, "/users/"+targetID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var got models.User
	require.NoError(t, db.Where("id = ?", targetID).First(&got).Error)
	assert.Equal(t, "repair_technician", got.Role)

	rec.mu.Lock()
	roleCalls, tmplCalls := rec.roleCalls, rec.tmplCalls
	rec.mu.Unlock()
	assert.Equal(t, 1, roleCalls, "IAM role call once")
	assert.Equal(t, 1, tmplCalls, "IAM template call once")

	var audit int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ? AND resource_id = ?", "user.role_update", targetID).Count(&audit).Error)
	assert.EqualValues(t, 1, audit, "audit log for role update")
}

func TestUpdateUserRoleForbidden_2375(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	targetID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: targetID, IAMSub: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		Username: "u2375f", Name: "越权", Role: "site_member", Status: "active",
	}).Error)
	otherTenant := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, otherTenant)
	foreignID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: foreignID, IAMSub: uuid.New().String(), TenantID: otherTenant, OrgID: otherTenant,
		Username: "foreign2375", Name: "外商户", Role: "site_member", Status: "active",
	}).Error)
	rec := &iamRoleMock2375{}
	setupRoleMock2375(t, rec, http.StatusOK)

	r := roleRouter2375(tenantID, uuid.New().String(), "ADMIN", func(r *gin.Engine, h *UserStaffHandler) {
		r.PUT("/users/:id", h.UpdateUser)
	})
	put := func(id string, payload map[string]interface{}) int {
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/users/"+id, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusForbidden, put(targetID, map[string]interface{}{"role": "merchant_admin"}), "whitelist must reject merchant_admin")
	assert.Equal(t, http.StatusNotFound, put(foreignID, map[string]interface{}{"role": "repair_technician"}), "cross-tenant must be 404")
	rec.mu.Lock()
	assert.Zero(t, rec.roleCalls, "no IAM call for rejected requests")
	rec.mu.Unlock()
}
