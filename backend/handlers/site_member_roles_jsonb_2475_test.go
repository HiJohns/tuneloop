package handlers

// #2475：site_members.roles 是 jsonb，GORM map Update 绕过 serializer:json。
// 任何 map 写入 roles 必须走 siteMemberRolesExpr（json.Marshal + ?::jsonb），
// 否则 pq 按 SQL 数组渲染 → SQLSTATE 22P02（"invalid input syntax for type json"）。
//
// 证据行（Accepted Plan R1–R4）：
//   R1 user_staff 改角色同步 roles → NO_22P02=1 / ROLES_JSON=["site_admin"] / ROLE=site_admin / ROLE_ERRORS=0
//   R2 iam_proxy ensureSiteMember 纠偏 → NO_22P02=1 / ROLES_JSON=["site_admin"]
//   R3 helper 归一化 → DEDUP / EMPTY=[] / NIL=[]（空集合不得写 NULL）
//   R4 既有多角色路径回归 → STATUS=200 / ROLES_KEPT=1（不被压成单角色）

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	siteMemberRolesTenant2475 = "11111111-1111-4111-8111-111111111111"
	siteMemberRolesUserToken  = "user-token-2475"
)

// fakeIAMRoles2475 覆盖 UpdateUser 改角色所需的两个 IAM 端点（#2375/#2462 契约）。
func fakeIAMRoles2475(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2475", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/role"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return srv
}

func rolesRouter2475(tenantID, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: tenantID, UserID: userID, Role: "ADMIN", SysPerm: -1, CusPerm: -1,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.PUT("/api/users/:id", (&UserStaffHandler{}).UpdateUser)
	r.PUT("/api/sites/:id/members/:uid", NewSiteMemberHandler().UpdateMemberRole)
	return r
}

// readSiteMemberRoles2475 直接读列文本，避免 serializer 掩盖写入形态。
func readSiteMemberRoles2475(t *testing.T, db *gorm.DB, userID string) (role, rolesJSON string) {
	t.Helper()
	row := db.Raw("SELECT role, roles::text FROM site_members WHERE user_id = ? AND tenant_id = ?",
		userID, siteMemberRolesTenant2475).Row()
	require.NoError(t, row.Scan(&role, &rolesJSON))
	var compact bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(rolesJSON)))
	return role, compact.String()
}

func seedStaffRoles2475(t *testing.T, db *gorm.DB, role string, iamSub string) (userID, siteID, tenantID string) {
	t.Helper()
	tenantID = siteMemberRolesTenant2475
	userID = uuid.NewString()
	siteID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: uuid.NewString(), Name: "网点2475", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userID, TenantID: tenantID, OrgID: tenantID, IAMSub: iamSub,
		Name: "员工2475", Phone: "13800002475", Role: role, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: userID,
		Role: role, Roles: []string{role}, Status: "active",
	}).Error)
	return userID, siteID, tenantID
}

// R1：改角色 → roles 按 jsonb 写入（旧实现必报 22P02）。
func TestUpdateUser_RoleSyncWritesJSONB_2475(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	fakeIAMRoles2475(t)
	userID, _, tenantID := seedStaffRoles2475(t, db, "site_member", "iam-sub-2475")

	r := rolesRouter2475(tenantID, uuid.NewString())
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"site_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+siteMemberRolesUserToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	require.Equal(t, http.StatusOK, w.Code, "%v", resp)
	require.NotContains(t, w.Body.String(), "22P02")
	require.NotContains(t, w.Body.String(), "invalid input syntax for type json")
	data, _ := resp["data"].(map[string]interface{})
	roleErrors, hasErrors := data["role_errors"]
	require.False(t, hasErrors, "role_errors 必须为空（不得吞错也不得报错）: %v", roleErrors)
	role, rolesJSON := readSiteMemberRoles2475(t, db, userID)
	require.Equal(t, "site_admin", role)
	require.JSONEq(t, `["site_admin"]`, rolesJSON)

	no22P02 := 0
	if !strings.Contains(w.Body.String(), "22P02") && rolesJSON == `["site_admin"]` {
		no22P02 = 1
	}
	require.Equal(t, 1, no22P02)

	t.Logf("NO_22P02=%d", no22P02)
	t.Logf("ROLES_JSON=%s", rolesJSON)
	t.Logf("ROLE=%s", role)
	t.Logf("ROLE_ERRORS=0")
}

// R2：IAM 纠偏（ensureSiteMember）写 roles 也必须是合法 jsonb。
func TestEnsureSiteMember_RoleCorrectionWritesJSONB_2475(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := siteMemberRolesTenant2475
	userID := uuid.NewString()
	iamOrgID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, TenantID: tenantID, OrgID: tenantID, IAMSub: "iam-sub-2475-b",
		Name: "员工2475-b", Phone: "13800002476", Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: iamOrgID, Name: "网点2475-b", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: userID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	corrected := ensureSiteMember(db, userID, iamOrgID, "ADMIN", tenantID)
	require.True(t, corrected, "纠偏必须写入成功（旧实现 UPDATE 报 22P02 时仍返回 true 但 roles 陈旧）")

	role, rolesJSON := readSiteMemberRoles2475(t, db, userID)
	require.Equal(t, "site_admin", role)
	require.JSONEq(t, `["site_admin"]`, rolesJSON)

	no22P02 := 0
	if corrected && rolesJSON == `["site_admin"]` {
		no22P02 = 1
	}
	require.Equal(t, 1, no22P02)

	t.Logf("NO_22P02=%d", no22P02)
	t.Logf("ROLES_JSON=%s", rolesJSON)
	t.Logf("ROLE=%s", role)
}

// R3：helper 归一化语义（去空白/去重保序；空集合写 []，不能是 NULL）。
func TestSiteMemberRolesExpr_Normalization_2475(t *testing.T) {
	expr, err := siteMemberRolesExpr([]string{"site_member", "site_member", "", "repair_technician"})
	require.NoError(t, err)
	dedup, ok := expr.Vars[0].(string)
	require.True(t, ok)
	require.JSONEq(t, `["site_member","repair_technician"]`, dedup)
	require.Contains(t, expr.SQL, "jsonb")

	emptyExpr, err := siteMemberRolesExpr([]string{})
	require.NoError(t, err)
	empty, _ := emptyExpr.Vars[0].(string)
	require.Equal(t, `[]`, empty)

	nilExpr, err := siteMemberRolesExpr(nil)
	require.NoError(t, err)
	nilRoles, _ := nilExpr.Vars[0].(string)
	require.Equal(t, `[]`, nilRoles)

	t.Logf("DEDUP=%s", dedup)
	t.Logf("EMPTY=%s", empty)
	t.Logf("NIL=%s", nilRoles)
}

// R4：既有 `PUT /api/sites/:id/members/:uid` 多角色路径行为不变。
func TestUpdateMemberRole_MultiRoleKept_2475(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	userID, siteID, tenantID := seedStaffRoles2475(t, db, "site_member", "")
	// 多角色全集：主角色 site_member + repair_technician。
	require.NoError(t, db.Exec(
		"UPDATE site_members SET roles = ?::jsonb WHERE site_id = ? AND user_id = ?",
		`["site_member","repair_technician"]`, siteID, userID).Error)

	r := rolesRouter2475(tenantID, uuid.NewString())
	body := `{"role":"site_member","roles":["site_member","repair_technician"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/"+siteID+"/members/"+userID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	require.Equal(t, http.StatusOK, w.Code, "%v", resp)

	role, rolesJSON := readSiteMemberRoles2475(t, db, userID)
	require.Equal(t, "site_member", role)
	require.JSONEq(t, `["site_member","repair_technician"]`, rolesJSON)

	rolesKept := 0
	var parsed []string
	require.NoError(t, json.Unmarshal([]byte(rolesJSON), &parsed))
	if len(parsed) == 2 {
		rolesKept = 1
	}
	require.Equal(t, 1, rolesKept, "多角色不得被压成单角色")

	t.Logf("STATUS=%d", w.Code)
	t.Logf("ROLES_JSON=%s", rolesJSON)
	t.Logf("ROLES_KEPT=%d", rolesKept)
}
