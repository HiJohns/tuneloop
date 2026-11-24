package handlers

// #2461 R4–R8：调用点失败语义
//   R4 网点成员改角色：IAM 主角色失败 → 502 且本地不变（原实现只 log 后继续写）
//   R5 模板分配失败 → 200 + role_errors（不阻断本地）
//   R6 换商户管理员 demote 失败 → 502 + 补偿解绑新管理员 + 本地未提交
//   R7 换商户管理员 bind 失败 → 200 + admin_pending + 显式告警字段（禁止只 log）
//   R8 直属员工改角色 IAM 失败 → 200 + role_errors + users.role 已更新（#2375 契约不变）

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

type roleIAMMock2461 struct {
	mu sync.Mutex

	roleCalls   int
	tmplCalls   int
	bindCalls   int
	unbindCalls int

	roleStatus   int // 0 == 200
	tmplStatus   int
	bindStatus   int
	unbindStatus int

	templates []string

	lastRolePath string
	lastRoleBody string
	lastRoleRawQ string
}

func setupRoleIAMMock2461(t *testing.T, m *roleIAMMock2461) {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	statusOf := func(code int) int {
		if code == 0 {
			return http.StatusOK
		}
		return code
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "svc-token-2461", "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := readBody2461(r)
		switch {
		case strings.HasSuffix(r.URL.Path, "/role"):
			m.mu.Lock()
			m.roleCalls++
			m.lastRolePath = r.URL.Path
			m.lastRoleBody = raw
			m.lastRoleRawQ = r.URL.RawQuery
			status := statusOf(m.roleStatus)
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "role endpoint unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		default:
			// bind / unbind 共用同一路径，由 body.action 区分
			var req struct {
				Action string `json:"action"`
			}
			_ = json.Unmarshal([]byte(raw), &req)
			m.mu.Lock()
			var status int
			if req.Action == "unbind" {
				m.unbindCalls++
				status = statusOf(m.unbindStatus)
			} else {
				m.bindCalls++
				status = statusOf(m.bindStatus)
			}
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "bind endpoint unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		}
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.tmplCalls++
		status := statusOf(m.tmplStatus)
		m.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			writeJSON(w, map[string]string{"error": "role template assign unavailable"})
			return
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/role-templates") {
			codes := m.templates
			if len(codes) == 0 {
				codes = []string{"site_member", "site_admin", "repair_technician"}
			}
			out := make([]map[string]string, 0, len(codes))
			for _, c := range codes {
				out = append(out, map[string]string{"id": "tmpl-" + c, "code": c})
			}
			writeJSON(w, out)
			return
		}
		writeJSON(w, map[string]interface{}{"id": "ns-2461"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
}

func readBody2461(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.String(), err
}

func siteMemberRouter2461(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, "ns-2461")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewSiteMemberHandler()
	r.PUT("/api/sites/:id/members/:uid", h.UpdateMemberRole)
	return r
}

func seedSiteMember2461(t *testing.T) (tenantID, siteID, userID, orgID string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID = uuid.NewString()
	siteID = uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S2461", Type: "normal", Status: "active",
	}).Error)
	userID = uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: orgID,
		Username: "u-" + uuid.NewString()[:8], Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: userID, Role: "site_member",
		Roles: []string{"site_member"},
	}).Error)
	return tenantID, siteID, userID, orgID
}

func putSiteMemberRole2461(t *testing.T, r *gin.Engine, siteID, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/"+siteID+"/members/"+userID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2461")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// R4：IAM 主角色更新失败 → 502，本地 site_members.role 不变（不再静默降级）。
func TestUpdateMemberRoleIAMFailFast_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, siteID, userID, _ := seedSiteMember2461(t)
	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := siteMemberRouter2461(t, tenantID)
	// site_admin 走 bind 通路；这里用非管理员角色走「更新角色」通路（Issue 现场正是该路径）。
	w := putSiteMemberRole2461(t, r, siteID, userID, `{"role":"repair_technician"}`)

	require.Equal(t, http.StatusBadGateway, w.Code, "IAM 主角色失败必须 502：%s", w.Body.String())

	var m models.SiteMember
	require.NoError(t, database.GetDB().Where("site_id = ? AND user_id = ?", siteID, userID).First(&m).Error)
	assert.Equal(t, "site_member", m.Role, "LOCAL_UNCHANGED：本地角色不得被改写")

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, 1, mock.roleCalls, "IAM_ROLE_CALLS=1")
	t.Logf("STATUS=%d LOCAL_UNCHANGED=1 IAM_ROLE_CALLS=%d", w.Code, mock.roleCalls)
	assert.Contains(t, mock.lastRolePath, "/role")
	assert.Empty(t, mock.lastRoleRawQ, "客户端不得再用 ?role= query")
	assert.JSONEq(t, `{"role":"repair_technician"}`, mock.lastRoleBody, "IAM body 必须携带 role（toIAMRole 对 repair_technician 透传）")
}

// R5：主角色成功、模板分配失败 → 200 + role_errors，本地已更新。
func TestUpdateMemberRoleTemplateErrorsVisible_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, siteID, userID, _ := seedSiteMember2461(t)
	mock := &roleIAMMock2461{tmplStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := siteMemberRouter2461(t, tenantID)
	w := putSiteMemberRole2461(t, r, siteID, userID, `{"role":"site_admin"}`)
	require.Equal(t, http.StatusOK, w.Code, "模板失败不得阻断：%s", w.Body.String())

	var m models.SiteMember
	require.NoError(t, database.GetDB().Where("site_id = ? AND user_id = ?", siteID, userID).First(&m).Error)
	assert.Equal(t, "site_admin", m.Role, "LOCAL_ROLE=site_admin")

	var resp struct {
		Data struct {
			RoleErrors []map[string]interface{} `json:"role_errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.RoleErrors, "ROLE_ERRORS=1：模板失败必须随响应可见")
	t.Logf("STATUS=%d ROLE_ERRORS=1 LOCAL_ROLE=%s", w.Code, m.Role)
}

func merchantRouter2461(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, "ns-2461")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewMerchantHandler()
	r.PUT("/merchants/:id", h.UpdateMerchant)
	return r
}

func seedMerchantWithAdmin2461(t *testing.T) (tenantID, merchantID, oldAdminID, newAdminID, orgID string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID = uuid.NewString()

	oldAdminID = uuid.NewString()
	newAdminID = uuid.NewString()
	for _, uid := range []string{oldAdminID, newAdminID} {
		require.NoError(t, db.Create(&models.User{
			ID: uid, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: orgID,
			Username: "u-" + uuid.NewString()[:8], Role: "merchant_admin", Status: "active",
		}).Error)
	}
	merchantID = uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: orgID, Name: "M2461",
		AdminUID: oldAdminID, Status: "active",
	}).Error)
	return tenantID, merchantID, oldAdminID, newAdminID, orgID
}

func putMerchantAdmin2461(t *testing.T, r *gin.Engine, merchantID, newAdminID string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"admin_uid": newAdminID})
	req := httptest.NewRequest(http.MethodPut, "/merchants/"+merchantID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2461")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// R6：bind 新管理员成功、demote 旧管理员失败 → 502 + 补偿解绑新管理员 + 本地未提交。
func TestUpdateMerchantAdminDemoteFailureCompensates_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, merchantID, oldAdminID, newAdminID, _ := seedMerchantWithAdmin2461(t)
	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := merchantRouter2461(t, tenantID)
	w := putMerchantAdmin2461(t, r, merchantID, newAdminID)
	require.Equal(t, http.StatusBadGateway, w.Code, "demote 失败必须 502：%s", w.Body.String())

	var resp struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "旧管理员降级失败", "MSG_CONTAINS")

	var m models.Merchant
	require.NoError(t, database.GetDB().Where("id = ?", merchantID).First(&m).Error)
	assert.Equal(t, oldAdminID, m.AdminUID, "ADMIN_UID_UNCHANGED：失败时不得提交本地换管理员")

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, 1, mock.bindCalls)
	assert.Equal(t, 1, mock.unbindCalls, "IAM_UNBIND_CALLS=1：必须补偿解绑新管理员")
	t.Logf("STATUS=%d MSG_CONTAINS=旧管理员降级失败 IAM_UNBIND_CALLS=%d ADMIN_UID_UNCHANGED=1", w.Code, mock.unbindCalls)
}

// R7：bind 新管理员失败 → 200 + admin_pending=true + 显式告警字段（禁止只 log）。
func TestUpdateMerchantAdminBindFailureVisible_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, merchantID, _, newAdminID, _ := seedMerchantWithAdmin2461(t)
	mock := &roleIAMMock2461{bindStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := merchantRouter2461(t, tenantID)
	w := putMerchantAdmin2461(t, r, merchantID, newAdminID)
	require.Equal(t, http.StatusOK, w.Code, "bind 失败不得整体失败（保留 admin_pending 语义）：%s", w.Body.String())

	var resp struct {
		Data struct {
			AdminPending bool `json:"admin_pending"`
		} `json:"data"`
		AdminSyncWarning string `json:"admin_sync_warning"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Data.AdminPending, "ADMIN_PENDING=true")
	assert.NotEmpty(t, resp.AdminSyncWarning, "bind 失败必须有告警字段，不得只写日志")
	t.Logf("STATUS=%d ADMIN_PENDING=%t ADMIN_SYNC_WARNING=1", w.Code, resp.Data.AdminPending)
}

// R8：直属员工改角色 IAM 失败 → 200 + role_errors + users.role 已更新（#2375 契约保持不变）。
func TestUpdateStaffRoleIAMFailRoleErrors_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	targetID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: targetID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: tenantID,
		Username: "u2461r8", Name: "R8", Role: "site_member", Status: "active",
	}).Error)
	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError, tmplStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := roleRouter2375(tenantID, uuid.NewString(), "ADMIN", func(r *gin.Engine, h *UserStaffHandler) {
		r.PUT("/users/:id", h.UpdateUser)
	})
	body, _ := json.Marshal(map[string]interface{}{"role": "repair_technician"})
	req := httptest.NewRequest(http.MethodPut, "/users/"+targetID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "role_errors 契约：不得 5xx：%s", w.Body.String())
	var resp struct {
		Data struct {
			RoleErrors []map[string]interface{} `json:"role_errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.RoleErrors, "ROLE_ERRORS=1")

	var got models.User
	require.NoError(t, db.Where("id = ?", targetID).First(&got).Error)
	assert.Equal(t, "repair_technician", got.Role, "USERS_ROLE_UPDATED=1")
	t.Logf("STATUS=%d ROLE_ERRORS=1 USERS_ROLE_UPDATED=1", w.Code)
}
