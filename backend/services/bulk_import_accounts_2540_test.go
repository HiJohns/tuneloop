package services

// #2540：批量导入账号的 IAM 关系角色必须经 ToIAMRole + 白名单兜底（#2467 邻居收敛）。
// 此前 `GetBusinessRole(acc.Role)` 把业务角色（site_member→"user"、site_admin→
// "organization_admin"）当 IAM 角色写入，对端 roleToPermission 不识别 → 权限 None。
//
// 断言面向 fake IAM 捕获的 bind payload（真实写库入参），不是函数返回值。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type iamCall2540Svc struct {
	Method string
	Path   string
	Body   string
}

type fakeIAM2540Svc struct {
	mu    sync.Mutex
	calls []iamCall2540Svc
}

func (f *fakeIAM2540Svc) record(r *http.Request) {
	body := ""
	if r.Body != nil {
		buf := make([]byte, 0, 512)
		tmp := make([]byte, 512)
		for {
			n, err := r.Body.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		body = string(buf)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, iamCall2540Svc{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (f *fakeIAM2540Svc) snapshot() []iamCall2540Svc {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]iamCall2540Svc, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeIAM2540Svc) bindRoles() []string {
	out := []string{}
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/bind") && strings.Contains(c.Body, `"action":"bind"`) {
			var req struct {
				Role string `json:"role"`
			}
			_ = json.Unmarshal([]byte(c.Body), &req)
			out = append(out, req.Role)
		}
	}
	return out
}

func installFakeIAM2540Svc(t *testing.T) *fakeIAM2540Svc {
	t.Helper()
	f := &fakeIAM2540Svc{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		f.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`{"users":[]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`{"code":20000,"data":{"user_id":"` + uuid.NewString() + `","status":"active"}}`))
		default:
			_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
		}
	}))
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return f
}

// testDBName2540 让服务层测试连上共享测试模板库（handlers 包 TestMain 维护的
// `tuneloop_test_template`）。服务层没有 TestMain，且不得自行 CREATE DATABASE，
// 因此这里只读使用模板库 + 事务回滚（见 TestBulkImportIAMRole2540）。
func testDBName2540(t *testing.T) {
	t.Helper()
	if testDBNameRe2540.MatchString(strings.TrimSpace(os.Getenv("TUNELOOP_DB"))) {
		return
	}
	for _, candidate := range []string{os.Getenv("TUNELOOP_TEST_DB"), "tuneloop_test_template"} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !testDBNameRe2540.MatchString(candidate) {
			continue
		}
		require.NoError(t, os.Setenv("TUNELOOP_DB", candidate))
		return
	}
	t.Fatalf("no test database available: set TUNELOOP_DB/TUNELOOP_TEST_DB to tuneloop_test_* (#2540 R2 不得跳过)")
}

var testDBNameRe2540 = regexp.MustCompile(`^tuneloop_test(_[a-z0-9_]+)?$`)

// TestBulkImportIAMRole2540：导入 site_member / site_admin / 空角色三行，
// bind payload 的关系角色只允许 STAFF/ADMIN。
//
// 所有本地写入都在事务内并回滚：模板库是共享 fixture，本用例不留残留数据。
func TestBulkImportIAMRole2540(t *testing.T) {
	testDBName2540(t)
	base, err := database.InitDB(database.LoadConfig())
	require.NoError(t, err, "测试 Postgres 不可用（#2540 R2：不得跳过，skip 视为 FAIL）")
	tx := base.Begin()
	require.NoError(t, tx.Error)
	database.SetDB(tx)
	t.Cleanup(func() {
		database.SetDB(base)
		_ = tx.Rollback().Error
	})
	// 导入路径用包级 GlobalPermissionRegistry 计算客户权限位；纯服务层测试没有 main
	// 初始化，必须显式建立（否则 nil deref，不是业务缺陷）。
	if GlobalPermissionRegistry == nil {
		GlobalPermissionRegistry = NewPermissionRegistry()
		t.Cleanup(func() { GlobalPermissionRegistry = nil })
	}

	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	siteID := uuid.NewString()
	require.NoError(t, tx.Create(&models.Tenant{ID: tenantID, Name: "T2540"}).Error)
	require.NoError(t, tx.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "网点2540",
		OrganizationCode: "SITE2540", Status: "active",
	}).Error)

	f := installFakeIAM2540Svc(t)

	// 场景 1：两行显式业务角色（site_member / site_admin）。
	csv := "email,name,username,site,phone,role\n" +
		"member2540@example.com,成员甲,member2540,SITE2540,13800000001,site_member\n" +
		"admin2540@example.com,管理员乙,admin2540,SITE2540,13800000002,site_admin\n"

	result, err := ImportAccountsCSV(context.Background(), strings.NewReader(csv), tenantID, NewIAMClient(), nil, false, false, "zh")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, result.Summary.Created, "两行都应创建成功: %+v", result.Details)

	roles := f.bindRoles()
	require.Len(t, roles, 2, "每行一次 bind: %v", roles)
	fmt.Printf("BIND_ROLES=%s\n", strings.Join(roles, ","))
	assert.Equal(t, []string{IAMRoleStaff, IAMRoleAdmin}, roles, "site_member→STAFF、site_admin→ADMIN")

	// 场景 2：legacy 空角色行 → 兜底 STAFF。
	csvLegacy := "email,name,username,site,phone,role\n" +
		"legacy2540@example.com,旧账号丙,legacy2540,SITE2540,13800000003,\n"
	legacyResult, err := ImportAccountsCSV(context.Background(), strings.NewReader(csvLegacy), tenantID, NewIAMClient(), nil, false, false, "zh")
	require.NoError(t, err)
	require.Equal(t, 1, legacyResult.Summary.Created, "空角色行应创建: %+v", legacyResult.Details)

	allRoles := f.bindRoles()
	require.Len(t, allRoles, 3, "三次 bind 合计: %v", allRoles)
	legacyRole := allRoles[len(allRoles)-1]
	fmt.Printf("LEGACY_BIND_ROLE=%s\n", legacyRole)
	assert.Equal(t, IAMRoleStaff, legacyRole, "空角色必须兜底 STAFF")

	// 负例：任何 bind payload 都不得出现业务角色字面量。
	illegal := 0
	for _, role := range allRoles {
		switch role {
		case IAMRoleOwner, IAMRoleAdmin, IAMRoleStaff, IAMRoleRepairTechnician:
		default:
			illegal++
			t.Logf("非法 IAM 关系角色: %q", role)
		}
	}
	fmt.Printf("BUSINESS_ROLE_AS_IAM=%d\n", illegal)
	assert.Zero(t, illegal, "bind payload 不得出现 user/organization_admin/tenant_admin/staff")
}

// TestToIAMRoleMapping2540：映射与白名单兜底的纯函数契约。
func TestToIAMRoleMapping2540(t *testing.T) {
	mapping := map[string]string{
		"merchant_admin":    IAMRoleOwner,
		"site_admin":        IAMRoleAdmin,
		"site_member":       IAMRoleStaff,
		"merchant_staff":    IAMRoleStaff,
		"repair_technician": IAMRoleRepairTechnician,
	}
	for in, want := range mapping {
		assert.Equal(t, want, ToIAMRole(in), "ToIAMRole(%q)", in)
	}
	// 未知码原样返回（映射层不做校验），由 Normalize 兜底。
	assert.Equal(t, "tenant_admin", ToIAMRole("tenant_admin"))
	assert.Equal(t, IAMRoleStaff, NormalizeIAMRelationRole(ToIAMRole("tenant_admin")))
	assert.Equal(t, IAMRoleStaff, NormalizeIAMRelationRole(""))
	assert.Equal(t, IAMRoleStaff, NormalizeIAMRelationRole("user"))
	assert.Equal(t, IAMRoleAdmin, NormalizeIAMRelationRole("ADMIN"))
}
