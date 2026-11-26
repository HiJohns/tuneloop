package handlers

// #2527：归属缺失（nil UUID）行的批量重置密码分类 + IAM 归属推导。
//
// 缺陷：批量重置密码按 `users.tenant_id = ?` 过滤 → 归属缺失行被静默丢弃，响应仍是
// 200「成功」；改角色 / 调动直接把 `users.org_id`（nil UUID）当组织发给 IAM。
//
// Evidence rows: R1（零归属成员可重置）、R2（混合批次显式分类）、R3（全不可见显式空操作）、
// R5（角色变更用派生 org）、R6（调动用派生 org）、R7（无归属显式拒绝，不发 nil UUID）。
//
// Token lines use fmt.Printf so scripts/verify-2527.sh can grep them from `go test -v`.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type iamCall2527 struct {
	Method string
	Path   string
	Body   string
}

type fakeIAM2527 struct {
	mu    sync.Mutex
	calls []iamCall2527
}

func (f *fakeIAM2527) snapshot() []iamCall2527 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]iamCall2527, len(f.calls))
	copy(out, f.calls)
	return out
}

// installFakeIAM2527 启一个记录 method/path/body 的假 IAM；reset-password 回 sent=len(user_ids)。
func installFakeIAM2527(t *testing.T) *fakeIAM2527 {
	t.Helper()
	f := &fakeIAM2527{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, iamCall2527{Method: r.Method, Path: r.URL.Path, Body: string(body)})
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/reset-password") {
			var req struct {
				UserIDs []string `json:"user_ids"`
			}
			_ = json.Unmarshal(body, &req)
			_, _ = fmt.Fprintf(w, `{"code":20000,"data":{"sent":%d,"skipped":0}}`, len(req.UserIDs))
			return
		}
		_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
	}))
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return f
}

func (f *fakeIAM2527) resetSubIDs() []string {
	out := []string{}
	for _, c := range f.snapshot() {
		if !strings.HasSuffix(c.Path, "/reset-password") {
			continue
		}
		var req struct {
			UserIDs []string `json:"user_ids"`
		}
		_ = json.Unmarshal([]byte(c.Body), &req)
		out = append(out, req.UserIDs...)
	}
	return out
}

func (f *fakeIAM2527) resetCalls() int {
	n := 0
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/reset-password") {
			n++
		}
	}
	return n
}

// orgFromPath 抽出 `/api/v1/organizations/{org}/...` 里的组织 ID。
func orgFromPath(path string) string {
	const marker = "/organizations/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	rest := path[idx+len(marker):]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		return rest[:slash]
	}
	return rest
}

// bindCalls 返回 (org, action) 列表（action 来自 body：bind/unbind）。
func (f *fakeIAM2527) bindCalls() [][2]string {
	out := [][2]string{}
	for _, c := range f.snapshot() {
		if !strings.HasSuffix(c.Path, "/bind") {
			continue
		}
		var req struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal([]byte(c.Body), &req)
		out = append(out, [2]string{orgFromPath(c.Path), req.Action})
	}
	return out
}

// roleOrgs 返回角色同步调用（/users/{id}/role 与 /users/{id}/roles）使用的 org。
func (f *fakeIAM2527) roleOrgs() []string {
	out := []string{}
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/role") {
			out = append(out, orgFromPath(c.Path))
			continue
		}
		if strings.HasSuffix(c.Path, "/roles") {
			var req struct {
				OrgID string `json:"org_id"`
			}
			_ = json.Unmarshal([]byte(c.Body), &req)
			out = append(out, req.OrgID)
		}
	}
	return out
}

// nilUUIDOrgCalls 统计把 nil UUID 当组织发出去的调用数（应为 0）。
func (f *fakeIAM2527) nilUUIDOrgCalls() int {
	n := 0
	for _, c := range f.snapshot() {
		if orgFromPath(c.Path) == nilUUIDLiteral {
			n++
		}
	}
	return n
}

type fixture2527 struct {
	tid, orgA, orgB string
	siteA, siteB    string
	zeroTenant      string // tenant/org = nil UUID，但 membership 在 siteA
	otherTenant     string // 他商户成员（不可见负例）
	noDerivableOrg  string // 归属缺失且无 membership（可见旧口径，但 IAM org 派生不出）
}

func setup2527(t *testing.T) fixture2527 {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	f := fixture2527{
		tid:   uuid.New().String(),
		orgA:  uuid.New().String(),
		orgB:  uuid.New().String(),
		siteA: uuid.New().String(),
		siteB: uuid.New().String(),
	}
	testfixtures.SeedTenantFK(t, db, f.tid)
	require.NoError(t, db.Create(&models.Site{ID: f.siteA, TenantID: f.tid, OrgID: f.orgA, Name: "网点A2527", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.siteB, TenantID: f.tid, OrgID: f.orgB, Name: "网点B2527", Status: "active"}).Error)

	f.zeroTenant = uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: f.zeroTenant, IAMSub: f.zeroTenant, TenantID: nilUUIDLiteral, OrgID: nilUUIDLiteral,
		Username: "zero2527", Name: "零归属成员", Email: "zero2527@example.com",
		Role: "merchant_staff", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: f.tid, SiteID: f.siteA, UserID: f.zeroTenant,
		Role: "merchant_staff", Roles: []string{"merchant_staff"}, Status: "active",
	}).Error)

	otherTid := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, otherTid)
	f.otherTenant = uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: f.otherTenant, IAMSub: f.otherTenant, TenantID: otherTid, OrgID: otherTid,
		Username: "other2527", Name: "他商户成员", Email: "other2527@example.com",
		Role: "site_member", Status: "active",
	}).Error)

	f.noDerivableOrg = uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: f.noDerivableOrg, IAMSub: f.noDerivableOrg, TenantID: f.tid, OrgID: nilUUIDLiteral,
		Username: "noorg2527", Name: "无归属成员", Email: "noorg2527@example.com",
		Role: "merchant_staff", Status: "active",
	}).Error)

	return f
}

func router2527(f fixture2527) *gin.Engine {
	withActor := func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: f.tid, OrgID: f.tid, UserID: uuid.New().String(),
			Role: "ADMIN", SysPerm: -1, CusPerm: -1,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}
	h := &UserStaffHandler{}
	r := gin.New()
	r.POST("/users/reset-password", withActor, h.ResetPassword)
	r.PUT("/users/:id", withActor, h.UpdateUser)
	return r
}

type wireResp2527 struct {
	status int
	code   int
	data   map[string]interface{}
}

func serve2527(t *testing.T, r *gin.Engine, method, path string, payload interface{}) wireResp2527 {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-user-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return wireResp2527{status: w.Code, code: resp.Code, data: resp.Data}
}

func ids2527(t *testing.T, data map[string]interface{}, key string) []string {
	t.Helper()
	raw, ok := data[key]
	if !ok || raw == nil {
		return []string{}
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func flag2527(cond bool) int {
	if cond {
		return 1
	}
	return 0
}

// R1：零归属（tenant/org = nil UUID）成员可被 reset-password 覆盖（不再被静默过滤）。
func TestResetPasswordZeroTenantVisible_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	resp := serve2527(t, r, http.MethodPost, "/users/reset-password",
		map[string]interface{}{"user_ids": []string{f.zeroTenant}})
	require.Equal(t, http.StatusOK, resp.status)
	require.EqualValues(t, 20000, resp.code)

	sent, _ := resp.data["sent"].(float64)
	subs := fake.resetSubIDs()
	invisible := ids2527(t, resp.data, "skipped_invisible")
	idMatch := len(subs) == 1 && subs[0] == f.zeroTenant

	fmt.Printf("R1 STATUS=%d SENT=%d IAM_IDS=%d IAM_ID_MATCH=%d SKIPPED_INVISIBLE=%d\n",
		resp.status, int(sent), len(subs), flag2527(idMatch), len(invisible))
	require.EqualValues(t, 1, sent, "body=%v", resp.data)
	require.Equal(t, 0, len(invisible))
}

// R2：混合批次（1 可见 + 1 跨租户）→ 只重置可见行，跨租户行显式进 skipped_invisible。
func TestResetPasswordMixedClassification_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	resp := serve2527(t, r, http.MethodPost, "/users/reset-password",
		map[string]interface{}{"user_ids": []string{f.zeroTenant, f.otherTenant}})
	require.Equal(t, http.StatusOK, resp.status)
	require.EqualValues(t, 20000, resp.code)

	subs := fake.resetSubIDs()
	invisible := ids2527(t, resp.data, "skipped_invisible")
	crossTenantIncluded := false
	for _, sub := range subs {
		if sub == f.otherTenant {
			crossTenantIncluded = true
		}
	}

	fmt.Printf("R2 STATUS=%d IAM_IDS=%d SKIPPED_INVISIBLE=%d IAM_INCLUDES_CROSS_TENANT=%d\n",
		resp.status, len(subs), len(invisible), flag2527(crossTenantIncluded))
	require.Equal(t, 1, len(subs))
	require.Equal(t, []string{f.otherTenant}, invisible)
	require.False(t, crossTenantIncluded)
}

// R3：全不可见 → 空操作必须显式（skipped_invisible），且不调用 IAM。
func TestResetPasswordAllInvisible_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	resp := serve2527(t, r, http.MethodPost, "/users/reset-password",
		map[string]interface{}{"user_ids": []string{f.otherTenant}})
	require.Equal(t, http.StatusOK, resp.status)
	require.EqualValues(t, 20000, resp.code)

	sent, _ := resp.data["sent"].(float64)
	invisible := ids2527(t, resp.data, "skipped_invisible")
	calls := fake.resetCalls()

	fmt.Printf("R3 STATUS=%d SENT=%d SKIPPED_INVISIBLE=%d IAM_CALLS=%d\n",
		resp.status, int(sent), len(invisible), calls)
	require.EqualValues(t, 0, sent)
	require.Equal(t, []string{f.otherTenant}, invisible)
	require.Equal(t, 0, calls)
}

// R5：零归属成员改角色 → IAM org 取自 membership（orgA），不发 nil UUID。
func TestUpdateUserRoleUsesDerivedOrg_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	resp := serve2527(t, r, http.MethodPut, "/users/"+f.zeroTenant,
		map[string]interface{}{"role": "site_member"})
	require.Equal(t, http.StatusOK, resp.status, "data=%v", resp.data)

	orgs := fake.roleOrgs()
	matches := len(orgs) > 0
	for _, org := range orgs {
		if org != f.orgA {
			matches = false
		}
	}
	nilCalls := fake.nilUUIDOrgCalls()
	roleErrors, _ := resp.data["role_errors"].([]interface{})
	// 证据 token 用符号名（plan G0：IAM_ORG=orgA），实际 UUID 见 fixture。
	iamOrgLabel := "mismatch"
	if matches {
		iamOrgLabel = "orgA"
	}

	fmt.Printf("R5 STATUS=%d IAM_ORG=%s IAM_ORG_MATCHES_MEMBERSHIP=%d NIL_UUID_CALLS=%d ROLE_ERRORS=%d\n",
		resp.status, iamOrgLabel, flag2527(matches), nilCalls, len(roleErrors))
	require.NotEmpty(t, orgs, "expected IAM role sync through the derived org")
	require.Equal(t, 0, nilCalls)
	require.Equal(t, 0, len(roleErrors))
}

// R6：零归属成员跨 org 调动 → unbind orgA / bind orgB（不出现 nil UUID）。
func TestSiteTransferUsesDerivedOrg_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	resp := serve2527(t, r, http.MethodPut, "/users/"+f.zeroTenant,
		map[string]interface{}{"site_id": f.siteB})
	require.Equal(t, http.StatusOK, resp.status, "data=%v", resp.data)

	var unbindOrg, bindOrg string
	for _, call := range fake.bindCalls() {
		switch call[1] {
		case "unbind":
			unbindOrg = call[0]
		case "bind":
			bindOrg = call[0]
		}
	}
	nilCalls := fake.nilUUIDOrgCalls()
	unbindLabel, bindLabel := "mismatch", "mismatch"
	if unbindOrg == f.orgA {
		unbindLabel = "orgA"
	}
	if bindOrg == f.orgB {
		bindLabel = "orgB"
	}

	fmt.Printf("R6 STATUS=%d UNBIND_ORG=%s BIND_ORG=%s UNBIND_ORG_MATCHES=%d BIND_ORG_MATCHES=%d NIL_UUID_CALLS=%d\n",
		resp.status, unbindLabel, bindLabel, flag2527(unbindOrg == f.orgA), flag2527(bindOrg == f.orgB), nilCalls)
	require.Equal(t, f.orgA, unbindOrg)
	require.Equal(t, f.orgB, bindOrg)
	require.Equal(t, 0, nilCalls)
}

// R7：无法推导归属 → 调动显式 40002、角色变更写 role_errors；两者都不发 nil UUID。
func TestNoAttributionRejectedExplicitly_2527(t *testing.T) {
	f := setup2527(t)
	fake := installFakeIAM2527(t)
	r := router2527(f)

	transfer := serve2527(t, r, http.MethodPut, "/users/"+f.noDerivableOrg,
		map[string]interface{}{"site_id": f.siteB})

	roleResp := serve2527(t, r, http.MethodPut, "/users/"+f.noDerivableOrg,
		map[string]interface{}{"role": "site_member"})
	roleErrors, _ := roleResp.data["role_errors"].([]interface{})
	attributionError := false
	for _, item := range roleErrors {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if msg, ok := entry["error"].(string); ok && strings.Contains(msg, "attribution missing") {
			attributionError = true
		}
	}

	// 同类路径：清空归属网点（site_id = nil UUID）也不得把 nil UUID 发给 IAM。
	clearResp := serve2527(t, r, http.MethodPut, "/users/"+f.zeroTenant,
		map[string]interface{}{"site_id": uuid.Nil.String()})
	var clearUnbindOrg string
	for _, call := range fake.bindCalls() {
		if call[1] == "unbind" {
			clearUnbindOrg = call[0]
		}
	}
	nilCalls := fake.nilUUIDOrgCalls()

	fmt.Printf("R7 TRANSFER_STATUS=%d TRANSFER_HTTP=%d ROLE_STATUS=%d ROLE_ERRORS_ATTRIBUTION=%d NIL_UUID_CALLS=%d CLEAR_STATUS=%d CLEAR_UNBIND_ORG_MATCHES=%d\n",
		transfer.code, transfer.status, roleResp.status, flag2527(attributionError), nilCalls,
		clearResp.status, flag2527(clearUnbindOrg == f.orgA))
	require.Equal(t, http.StatusBadRequest, transfer.status)
	require.EqualValues(t, 40002, transfer.code)
	require.Equal(t, http.StatusOK, roleResp.status)
	require.True(t, attributionError, "role_errors=%v", roleErrors)
	require.Equal(t, 0, nilCalls)
	require.Equal(t, http.StatusOK, clearResp.status)
	require.Equal(t, f.orgA, clearUnbindOrg, "clear-site unbind must use the derived org")
}
