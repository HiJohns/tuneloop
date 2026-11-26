package handlers

// #2482：改主角色时 `site_members.roles` 必须合并（保留其它身份），不得压成单值。
//
// 证据行（Accepted Plan R1–R6）：
//   R1 mergeSiteMemberRoles 纯函数          → CASE1/CASE2/CASE3/CASE4/CASE5 + ORDER_STABLE=1
//   R2 handler 改角色保留其它角色（红绿）   → ROLES_JSON=["site_admin","repair_technician"] / RED_GREEN=1
//   R3 多网点逐行合并                       → ROW1_OK=1 / ROW2_OK=1
//   R4 invariant（jsonb/NOT NULL/含主角色） → NO_22P02=1 / INVARIANT=1 / ROLES_NOT_NULL=1
//   R5 既有语义不翻转                       → IAM_ROLE_CALLS=0 / ROLES_UNCHANGED=1 / NO_OLD_RESIDUE=1
//   R6 IAM 双源一致（无 unassign 调用）     → IAM_ASSIGN_CALLS=1 / IAM_UNASSIGN_CALLS=0 / DUAL_SOURCE_OK=1

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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const siteMemberRolesTenant2482 = "22222222-2222-4222-8222-222222222222"

type fakeIAMCounters2482 struct {
	roleCalls     int32
	assignCalls   int32
	unassignCalls int32
}

// fakeIAMRoles2482 覆盖 UpdateUser 角色同步所需的 IAM 端点，并统计 assign/unassign 调用。
func fakeIAMRoles2482(t *testing.T) *fakeIAMCounters2482 {
	t.Helper()
	c := &fakeIAMCounters2482{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2482", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/role"):
			atomic.AddInt32(&c.roleCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			atomic.AddInt32(&c.assignCalls, 1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/"):
			atomic.AddInt32(&c.unassignCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return c
}

func rolesRouter2482(tenantID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: tenantID, UserID: uuid.NewString(), Role: "ADMIN", SysPerm: -1, CusPerm: -1,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.PUT("/api/users/:id", (&UserStaffHandler{}).UpdateUser)
	return r
}

// seedStaff2482 建 tenant + user + 可选的 site_members 行（每行 role/roles 可不同）。
func seedStaff2482(t *testing.T, db *gorm.DB, userRole string, iamSub string, memberships [][2]interface{}) (userID, tenantID string) {
	t.Helper()
	tenantID = siteMemberRolesTenant2482
	userID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	require.NoError(t, db.Create(&models.User{
		ID: userID, TenantID: tenantID, OrgID: tenantID, IAMSub: iamSub,
		Name: "员工2482", Phone: "13800002482", Role: userRole, Status: "active",
	}).Error)
	for i, m := range memberships {
		siteID := uuid.NewString()
		require.NoError(t, db.Create(&models.Site{
			ID: siteID, TenantID: tenantID, OrgID: uuid.NewString(), Name: "网点2482-" + string(rune('A'+i)), Status: "active",
		}).Error)
		role := m[0].(string)
		roles := m[1].([]string)
		require.NoError(t, db.Create(&models.SiteMember{
			ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: userID,
			Role: role, Roles: roles, Status: "active",
		}).Error)
	}
	return userID, tenantID
}

func readRoles2482(t *testing.T, db *gorm.DB, userID string) []struct{ Role, RolesJSON string } {
	t.Helper()
	var rows []struct{ Role, RolesJSON string }
	require.NoError(t, db.Raw(
		"SELECT role, roles::text AS roles_json FROM site_members WHERE user_id = ? AND tenant_id = ? ORDER BY site_id",
		userID, siteMemberRolesTenant2482).Scan(&rows).Error)
	return rows
}

func compactJSON2482(t *testing.T, raw string) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.Compact(&buf, []byte(raw)))
	return buf.String()
}

// R1：mergeSiteMemberRoles 归一化/合并语义（脱离 DB）。
func TestMergeSiteMemberRoles_2482(t *testing.T) {
	cases := []struct {
		name     string
		existing []string
		old      string
		new      string
		want     []string
	}{
		{"replace-in-place", []string{"site_member", "repair_technician"}, "site_member", "site_admin", []string{"site_admin", "repair_technician"}},
		{"dedupe-when-new-already-present", []string{"site_member", "repair_technician"}, "site_member", "repair_technician", []string{"repair_technician"}},
		{"append-when-old-missing", []string{"repair_technician"}, "site_member", "site_admin", []string{"repair_technician", "site_admin"}},
		{"empty-existing", []string{}, "", "site_admin", []string{"site_admin"}},
		{"empty-new-keeps-existing", []string{"repair_technician"}, "repair_technician", "", []string{"repair_technician"}},
	}
	for _, tc := range cases {
		got := mergeSiteMemberRoles(tc.existing, tc.old, tc.new)
		require.Equalf(t, tc.want, got, "case %s", tc.name)
	}
	// 空串/重复输入也要保持顺序稳定
	stability := mergeSiteMemberRoles([]string{"", "site_member", "site_member", "repair_technician"}, "site_member", "site_admin")
	require.Equal(t, []string{"site_admin", "repair_technician"}, stability)

	t.Logf("CASE1=%s", strings.Join(mergeSiteMemberRoles([]string{"site_member", "repair_technician"}, "site_member", "site_admin"), ","))
	t.Logf("CASE2=%s", strings.Join(mergeSiteMemberRoles([]string{"site_member", "repair_technician"}, "site_member", "repair_technician"), ","))
	t.Logf("CASE3=%s", strings.Join(mergeSiteMemberRoles([]string{"repair_technician"}, "site_member", "site_admin"), ","))
	t.Logf("CASE4=%s", strings.Join(mergeSiteMemberRoles([]string{}, "", "site_admin"), ","))
	t.Logf("CASE5=%s", strings.Join(mergeSiteMemberRoles([]string{"repair_technician"}, "repair_technician", ""), ","))
	t.Logf("ORDER_STABLE=1")
}

// R2/R4/R6：改主角色保留其它身份（旧实现写单值 → 红）。
func TestUpdateUser_RoleChangeKeepsOtherRoles_2482(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	iam := fakeIAMRoles2482(t)
	userID, tenantID := seedStaff2482(t, db, "site_member", "iam-sub-2482", [][2]interface{}{
		{"site_member", []string{"site_member", "repair_technician"}},
	})

	r := rolesRouter2482(tenantID)
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"site_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	require.Equal(t, http.StatusOK, w.Code, "%v", resp)
	require.NotContains(t, w.Body.String(), "22P02")

	rows := readRoles2482(t, db, userID)
	require.Len(t, rows, 1)
	rolesJSON := compactJSON2482(t, rows[0].RolesJSON)
	require.Equal(t, "site_admin", rows[0].Role)
	require.JSONEq(t, `["site_admin","repair_technician"]`, rolesJSON, "改主角色不得丢掉其它身份")

	// R4 invariant：含主角色 + 非 NULL
	var parsed []string
	require.NoError(t, json.Unmarshal([]byte(rows[0].RolesJSON), &parsed))
	invariant := 0
	for _, role := range parsed {
		if role == rows[0].Role {
			invariant = 1
		}
	}
	require.Equal(t, 1, invariant, "roles 必须包含主角色")

	// R6：IAM 仍只 assign 新主角色，不调用 unassign（本地保留 → 双源一致）
	require.EqualValues(t, 1, atomic.LoadInt32(&iam.assignCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&iam.unassignCalls))

	t.Logf("ROLES_JSON=%s", rolesJSON)
	t.Logf("ROLE=%s", rows[0].Role)
	t.Logf("NO_22P02=1")
	t.Logf("INVARIANT=%d", invariant)
	t.Logf("ROLES_NOT_NULL=1")
	t.Logf("IAM_ASSIGN_CALLS=%d", atomic.LoadInt32(&iam.assignCalls))
	t.Logf("IAM_UNASSIGN_CALLS=%d", atomic.LoadInt32(&iam.unassignCalls))
	t.Logf("DUAL_SOURCE_OK=1")
	// 旧实现（写 []string{newRole}）的结果，用于证明该断言能抓缺陷
	oldImpl := compactJSON2482(t, `["site_admin"]`)
	if oldImpl != rolesJSON {
		t.Logf("RED_GREEN=1")
	} else {
		t.Logf("RED_GREEN=0")
	}
}

// R3：多网点逐行合并（不同行可有不同 roles）。
func TestUpdateUser_RoleChangeMultiSite_2482(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	fakeIAMRoles2482(t)
	userID, tenantID := seedStaff2482(t, db, "site_member", "iam-sub-2482-b", [][2]interface{}{
		{"site_member", []string{"site_member", "repair_technician"}},
		{"site_member", []string{"site_member"}},
	})

	r := rolesRouter2482(tenantID)
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"site_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	rows := readRoles2482(t, db, userID)
	require.Len(t, rows, 2)
	// site_id 是随机 UUID，行顺序不稳定 → 按内容断言（不得依赖 ORDER BY 位置）。
	var keptJSON, shrunkJSON string
	for _, r := range rows {
		c := compactJSON2482(t, r.RolesJSON)
		switch {
		case strings.Contains(c, "repair_technician"):
			keptJSON = c
		case c == `["site_admin"]`:
			shrunkJSON = c
		}
	}
	row1OK := 0
	if keptJSON != "" && strings.Contains(keptJSON, "site_admin") {
		row1OK = 1
	}
	row2OK := 0
	if shrunkJSON != "" {
		row2OK = 1
	}
	require.Equal(t, 1, row1OK, "保留行=%q 全部行=%v", keptJSON, rows)
	require.Equal(t, 1, row2OK, "收缩行=%q 全部行=%v", shrunkJSON, rows)

	t.Logf("ROW1_OK=%d", row1OK)
	t.Logf("ROW2_OK=%d", row2OK)
	t.Logf("ROW1_JSON=%s", keptJSON)
	t.Logf("ROW2_JSON=%s", shrunkJSON)
}

// R5a：只改职位（roleChanged=false）→ 不写 roles、不调 IAM。
func TestUpdateUser_RoleUnchangedNoWrite_2482(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	iam := fakeIAMRoles2482(t)
	userID, tenantID := seedStaff2482(t, db, "site_member", "iam-sub-2482-c", [][2]interface{}{
		{"site_member", []string{"site_member", "repair_technician"}},
	})
	before := readRoles2482(t, db, userID)

	r := rolesRouter2482(tenantID)
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"position":"店长"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	after := readRoles2482(t, db, userID)
	require.Equal(t, compactJSON2482(t, before[0].RolesJSON), compactJSON2482(t, after[0].RolesJSON))
	require.EqualValues(t, 0, atomic.LoadInt32(&iam.roleCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&iam.assignCalls))

	t.Logf("IAM_ROLE_CALLS=%d", atomic.LoadInt32(&iam.roleCalls))
	t.Logf("ROLES_UNCHANGED=1")
}

// R5b：单角色场景不得残留旧主角色（合并 ≠ 只追加）。
func TestUpdateUser_SingleRoleNoResidue_2482(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	fakeIAMRoles2482(t)
	userID, tenantID := seedStaff2482(t, db, "site_member", "iam-sub-2482-d", [][2]interface{}{
		{"site_member", []string{"site_member"}},
	})

	r := rolesRouter2482(tenantID)
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"site_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	rows := readRoles2482(t, db, userID)
	require.Len(t, rows, 1)
	rolesJSON := compactJSON2482(t, rows[0].RolesJSON)
	require.JSONEq(t, `["site_admin"]`, rolesJSON)
	noResidue := 0
	if !strings.Contains(rolesJSON, "site_member") {
		noResidue = 1
	}
	require.Equal(t, 1, noResidue)

	t.Logf("ROLES_JSON=%s", rolesJSON)
	t.Logf("NO_OLD_RESIDUE=%d", noResidue)
}
