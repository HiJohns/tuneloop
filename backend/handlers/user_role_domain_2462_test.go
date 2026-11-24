package handlers

// #2462 R1–R6：商户直属员工角色落域
//   R1 创建直属员工：users.role = merchant_members.role = merchant_staff，无 site_members
//   R2 创建网点员工：users.role = site_members.role = site_member，两者一致
//   R3 创建维修师傅：users.role = repair_technician、technician_profiles 1 行、无 merchant_members
//   R4 列表：直属用户 role=merchant_staff（与 users.role 一致）、site_name 空
//   R5 编辑只改职位：users.role 不变、IAM 角色同步 0 次、role_errors 空
//   R6 编辑显式改角色：users.role/merchant_members.role 均更新、IAM 同步 1 次且映射 STAFF

import (
	"bytes"
	"encoding/json"
	"io"
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
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

type roleDomainMock2462 struct {
	mu sync.Mutex

	roleCalls int
	tmplCalls int
	lastRole  string
}

func setupRoleDomainMock2462(t *testing.T, m *roleDomainMock2462) {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "svc-token-2462", "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, []interface{}{})
			return
		}
		writeJSON(w, map[string]interface{}{"user_id": uuid.NewString(), "status": "active"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			m.mu.Lock()
			m.tmplCalls++
			m.mu.Unlock()
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/role") {
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				Role string `json:"role"`
			}
			_ = json.Unmarshal(raw, &body)
			role := body.Role
			// #2461 之后 role 走 JSON body；此前走查询参数。两条都容忍，
			// 本单只断言「调用次数 + 映射值」，不把 #2461 的契约计入验收。
			if role == "" {
				role = r.URL.Query().Get("role")
			}
			m.mu.Lock()
			m.roleCalls++
			m.lastRole = role
			m.mu.Unlock()
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/role-templates") {
			writeJSON(w, []map[string]string{
				{"id": "tmpl-1", "code": "merchant_staff"},
				{"id": "tmpl-2", "code": "site_member"},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"id": "ns-2462"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
}

// seedStaffEnv2462 建立商户管理员上下文：tenant T、商户 M(org=T)、操作员。
func seedStaffEnv2462(t *testing.T) (tenantID, merchantID, operatorID string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	merchantID = uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: "M2462",
		AdminUID: uuid.NewString(), Status: "active",
	}).Error)
	operatorID = uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2462", Role: "admin", Status: "active",
	}).Error)
	return tenantID, merchantID, operatorID
}

func staffRouter2462(t *testing.T, tenantID, operatorID string) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{
				TenantID: tenantID, OrgID: tenantID, UserID: operatorID,
				Role: "ADMIN", SysPerm: -1, CusPerm: -1,
			}.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := &UserStaffHandler{}
	r.POST("/users", h.CreateUser)
	r.PUT("/users/:id", h.UpdateUser)
	r.GET("/staff", h.ListStaff)
	return r
}

func doCreate2462(t *testing.T, r *gin.Engine, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2462")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// R1：创建直属员工 → users.role 与 merchant_members.role 均为 merchant_staff，无 site_members。
func TestCreateMerchantDirectRoleDomain_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, _, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	r := staffRouter2462(t, tenantID, operatorID)
	code, resp := doCreate2462(t, r, map[string]interface{}{
		"username": "u-direct-2462", "name": "测试直属", "user_type": "merchant_direct",
		"auto_generate": true,
	})
	require.Equal(t, http.StatusOK, code, "body=%v", resp)

	db := database.GetDB()
	var u models.User
	require.NoError(t, db.First(&u, "username = ?", "u-direct-2462").Error)
	assert.Equal(t, "merchant_staff", u.Role, "USERS_ROLE 必须是域角色")

	var mm models.MerchantMember
	require.NoError(t, db.First(&mm, "user_id = ?", u.ID).Error)
	assert.Equal(t, "merchant_staff", mm.Role, "MM_ROLE 不得再硬编码 site_member")

	var smRows int64
	require.NoError(t, db.Model(&models.SiteMember{}).Where("user_id = ?", u.ID).Count(&smRows).Error)
	assert.EqualValues(t, 0, smRows, "SITE_MEMBERS_ROWS=0")

	t.Logf("USERS_ROLE=%s MM_ROLE=%s SITE_MEMBERS_ROWS=%d", u.Role, mm.Role, smRows)
}

// R2：创建网点员工 → users.role 与 site_members.role 一致（site_member）。
func TestCreateSiteStaffRoleDomain_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, _, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	db := database.GetDB()
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S2462", Type: "normal", Status: "active",
	}).Error)
	// 该网点已有 1 名成员 → 新员工不是首用户，角色为 site_member。
	seedUserID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: seedUserID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: tenantID,
		Username: "seed2462", Role: "site_admin", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: seedUserID,
		Role: "site_admin", Roles: []string{"site_admin"},
	}).Error)

	r := staffRouter2462(t, tenantID, operatorID)
	code, resp := doCreate2462(t, r, map[string]interface{}{
		"username": "u-site-2462", "name": "测试网点", "user_type": "site_staff",
		"site_id": siteID, "auto_generate": true,
	})
	require.Equal(t, http.StatusOK, code, "body=%v", resp)

	var u models.User
	require.NoError(t, db.First(&u, "username = ?", "u-site-2462").Error)
	var sm models.SiteMember
	require.NoError(t, db.First(&sm, "user_id = ?", u.ID).Error)
	assert.Equal(t, "site_member", u.Role, "USERS_ROLE")
	assert.Equal(t, "site_member", sm.Role, "SM_ROLE")
	assert.Equal(t, u.Role, sm.Role, "ROLE_MATCH=1")

	t.Logf("USERS_ROLE=%s SM_ROLE=%s ROLE_MATCH=1", u.Role, sm.Role)
}

// R3：创建维修师傅 → users.role=repair_technician、师傅档案 1 行、无 merchant_members。
func TestCreateTechnicianRoleDomain_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, _, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	r := staffRouter2462(t, tenantID, operatorID)
	code, resp := doCreate2462(t, r, map[string]interface{}{
		"username": "u-tech-2462", "name": "测试师傅", "user_type": "repair_technician",
		"auto_generate": true,
	})
	require.Equal(t, http.StatusOK, code, "body=%v", resp)

	db := database.GetDB()
	var u models.User
	require.NoError(t, db.First(&u, "username = ?", "u-tech-2462").Error)
	assert.Equal(t, "repair_technician", u.Role, "USERS_ROLE")

	var tpRows int64
	require.NoError(t, db.Model(&models.TechnicianProfile{}).Where("user_id = ?", u.ID).Count(&tpRows).Error)
	assert.EqualValues(t, 1, tpRows, "TECH_PROFILES=1")

	var mmRows int64
	require.NoError(t, db.Model(&models.MerchantMember{}).Where("user_id = ?", u.ID).Count(&mmRows).Error)
	assert.EqualValues(t, 0, mmRows, "MM_ROWS=0")

	t.Logf("USERS_ROLE=%s TECH_PROFILES=%d MM_ROWS=%d", u.Role, tpRows, mmRows)
}

// R4：列表对直属用户返回 merchant_staff（与 users.role 一致）、site_name 为空。
func TestListStaffDirectRole_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, merchantID, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	db := database.GetDB()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: tenantID,
		Username: "u-list-2462", Name: "列表直属", Role: "merchant_staff", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: tenantID, MerchantID: merchantID, UserID: userID,
		Role: "merchant_staff", Status: "active",
	}).Error)

	r := staffRouter2462(t, tenantID, operatorID)
	req := httptest.NewRequest(http.MethodGet, "/staff?page=1&page_size=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	var row map[string]interface{}
	for _, item := range resp.Data.List {
		// 列表项不含 username（#2462 现状），按 id 定位。
		if item["id"] == userID {
			row = item
			break
		}
	}
	require.NotNil(t, row, "列表必须包含该直属用户：%s", w.Body.String())
	assert.Equal(t, "merchant_staff", row["role"], "LIST_ROLE")

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.Equal(t, u.Role, row["role"], "ROLE_MATCH=1")
	_, hasSite := row["site_name"]
	assert.False(t, hasSite, "SITE_NAME_EMPTY=1")

	t.Logf("LIST_ROLE=%v ROLE_MATCH=1 SITE_NAME_EMPTY=1", row["role"])
}

func seedDirectUser2462(t *testing.T, tenantID, merchantID, role string) string {
	t.Helper()
	db := database.GetDB()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: tenantID,
		Username: "u-" + uuid.NewString()[:8], Name: "直属编辑", Role: role, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: tenantID, MerchantID: merchantID, UserID: userID,
		Role: role, Status: "active",
	}).Error)
	return userID
}

func doUpdate2462(t *testing.T, r *gin.Engine, userID string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/users/"+userID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2462")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// R5：编辑只改职位（role 提交值与现值相同）→ 不写角色、不调 IAM、role_errors 空。
func TestUpdateStaffRoleNoop_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, merchantID, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	userID := seedDirectUser2462(t, tenantID, merchantID, "merchant_staff")
	r := staffRouter2462(t, tenantID, operatorID)
	code, resp := doUpdate2462(t, r, userID, map[string]interface{}{
		"role": "merchant_staff", "position": "首席",
	})
	require.Equal(t, http.StatusOK, code, "body=%v", resp)

	db := database.GetDB()
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.Equal(t, "merchant_staff", u.Role, "USERS_ROLE_UNCHANGED=1")
	assert.Equal(t, "首席", u.Position, "POSITION")

	mock.mu.Lock()
	roleCalls := mock.roleCalls
	mock.mu.Unlock()
	assert.Zero(t, roleCalls, "IAM_ROLE_CALLS=0（无变化不得同步）")

	data, _ := resp["data"].(map[string]interface{})
	_, hasErrors := data["role_errors"]
	assert.False(t, hasErrors, "ROLE_ERRORS=0")

	t.Logf("USERS_ROLE_UNCHANGED=1 IAM_ROLE_CALLS=%d ROLE_ERRORS=0 POSITION=%s", roleCalls, u.Position)
}

// R6：编辑显式改角色 → users.role/merchant_members.role 更新，IAM 同步 1 次且映射 STAFF。
func TestUpdateStaffRoleChanged_2462(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, merchantID, operatorID := seedStaffEnv2462(t)
	mock := &roleDomainMock2462{}
	setupRoleDomainMock2462(t, mock)

	userID := seedDirectUser2462(t, tenantID, merchantID, "site_member")
	r := staffRouter2462(t, tenantID, operatorID)
	code, resp := doUpdate2462(t, r, userID, map[string]interface{}{"role": "merchant_staff"})
	require.Equal(t, http.StatusOK, code, "body=%v", resp)

	db := database.GetDB()
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.Equal(t, "merchant_staff", u.Role, "USERS_ROLE")

	var mm models.MerchantMember
	require.NoError(t, db.First(&mm, "user_id = ?", userID).Error)
	assert.Equal(t, "merchant_staff", mm.Role, "MM_ROLE")

	mock.mu.Lock()
	roleCalls, lastRole := mock.roleCalls, mock.lastRole
	mock.mu.Unlock()
	assert.Equal(t, 1, roleCalls, "IAM_ROLE_CALLS=1")
	assert.Equal(t, "STAFF", lastRole, "IAM_ROLE_VALUE=STAFF")

	t.Logf("USERS_ROLE=%s MM_ROLE=%s IAM_ROLE_CALLS=%d IAM_ROLE_VALUE=%s", u.Role, mm.Role, roleCalls, lastRole)
}
