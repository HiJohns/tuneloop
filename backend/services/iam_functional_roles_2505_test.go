package services

// #2505 S1 R2：IAM 功能角色集合语义的客户端契约。
//
// 断言（machine-checkable）：
//   GET_PATH=/api/v1/users/USER/roles
//   GET_QUERY=org_id=ORG
//   PUT_PATH=/api/v1/organizations/ORG/users/USER/functional-roles
//   PUT_BODY={"roles":["site_member"]}
//   FORBIDDEN_ERR=1 / NOTFOUND_ERR=1 / BADREQUEST_ERR=1（失败不得吞成 nil）
//   TEMPLATE_INACTIVE_PARSED=1（ListRoleTemplates 必须解析 is_active，D7）
//
// 反例：403 被吞成 nil → 前端「改角色成功」但 IAM 未变（权限回收失效）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type functionalRolesFake2505 struct {
	mu sync.Mutex

	getStatus int
	putStatus int
	current   []string

	getCalls     int
	putCalls     int
	permCalls    int
	lastGetPath  string
	lastGetQuery string
	lastPutPath  string
	lastPutBody  string
	lastPutRoles []string
}

func newFunctionalRolesFake2505(t *testing.T) *functionalRolesFake2505 {
	t.Helper()
	f := &functionalRolesFake2505{current: []string{"site_admin", "site_member"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_, _ = w.Write([]byte(`{"access_token":"svc-token-2505","expires_in":3600,"token_type":"Bearer"}`))
		case r.URL.Path == "/api/v1/perm-version/increment":
			f.mu.Lock()
			f.permCalls++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"perm_version":2}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") && strings.HasSuffix(r.URL.Path, "/role-templates"):
			// D7：列表接口会返回 is_active=false 的模板，消费方必须解析。
			_, _ = w.Write([]byte(`[{"id":"t1","code":"site_admin","is_active":false},` +
				`{"id":"t2","code":"site_member","is_active":true}]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			f.mu.Lock()
			f.getCalls++
			f.lastGetPath = r.URL.Path
			f.lastGetQuery = r.URL.RawQuery
			status, current := f.getStatus, append([]string(nil), f.current...)
			f.mu.Unlock()
			if status != 0 && status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
				return
			}
			body := make([]map[string]interface{}, 0, 1)
			if len(current) > 0 {
				body = append(body, map[string]interface{}{
					"org_id":           r.URL.Query().Get("org_id"),
					"functional_roles": current,
				})
			}
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/functional-roles"):
			raw, _ := io.ReadAll(r.Body)
			var req struct {
				Roles []string `json:"roles"`
			}
			_ = json.Unmarshal(raw, &req)
			f.mu.Lock()
			f.putCalls++
			f.lastPutPath = r.URL.Path
			f.lastPutBody = string(raw)
			f.lastPutRoles = req.Roles
			status := f.putStatus
			f.mu.Unlock()
			if status != 0 && status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"invalid_role","message":"role site_admin is not valid"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })
	return f
}

func TestFunctionalRolesContract_2505(t *testing.T) {
	f := newFunctionalRolesFake2505(t)
	client := NewIAMClient()

	// 1) GET：路径 + org_id 查询参数；只取匹配 org 的集合。
	current, err := client.GetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG")
	require.NoError(t, err)
	require.Equal(t, []string{"site_admin", "site_member"}, current)

	f.mu.Lock()
	getPath, getQuery := f.lastGetPath, f.lastGetQuery
	f.mu.Unlock()
	require.Equal(t, "/api/v1/users/USER/roles", getPath)
	require.Equal(t, "org_id=ORG", getQuery)
	t.Logf("GET_PATH=%s", getPath)
	t.Logf("GET_QUERY=%s", getQuery)

	// 2) PUT：集合覆盖 body。
	require.NoError(t, client.SetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG", []string{"site_member"}))
	f.mu.Lock()
	putPath, putBody, putRoles, permCalls := f.lastPutPath, f.lastPutBody, append([]string(nil), f.lastPutRoles...), f.permCalls
	f.mu.Unlock()
	require.Equal(t, "/api/v1/organizations/ORG/users/USER/functional-roles", putPath)
	require.Equal(t, []string{"site_member"}, putRoles)
	require.JSONEq(t, `{"roles":["site_member"]}`, putBody)
	require.GreaterOrEqual(t, permCalls, 1, "成功后必须 IncrementPermVersion")
	t.Logf("PUT_PATH=%s", putPath)
	t.Logf("PUT_BODY=%s", putBody)
	t.Logf("PERM_VERSION_CALLS=%d", permCalls)

	// 3) 失败语义：403 / 404 / 400 必须返回 error（不得吞成 nil）。
	f.mu.Lock()
	f.putStatus = http.StatusForbidden
	f.mu.Unlock()
	err = client.SetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG", []string{"site_member"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	forbidden := 1
	t.Logf("FORBIDDEN_ERR=%d", forbidden)

	f.mu.Lock()
	f.putStatus = http.StatusNotFound
	f.mu.Unlock()
	err = client.SetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG", []string{"site_member"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	t.Logf("NOTFOUND_ERR=1")

	f.mu.Lock()
	f.putStatus = http.StatusBadRequest
	f.mu.Unlock()
	err = client.SetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG", []string{"site_admin"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid functional roles")
	t.Logf("BADREQUEST_ERR=1")

	f.mu.Lock()
	f.getStatus = http.StatusInternalServerError
	f.mu.Unlock()
	_, err = client.GetUserFunctionalRolesWithToken("user-token-2505", "USER", "ORG")
	require.Error(t, err, "GET 5xx 不得返回空集合冒充成功")
	t.Logf("GET_ERROR_ERR=1")

	// 4) ListRoleTemplates 必须解析 is_active（否则失效码会被发送 → 整组 400）。
	f.mu.Lock()
	f.getStatus = 0
	f.mu.Unlock()
	templates, err := client.ListRoleTemplates("NS")
	require.NoError(t, err)
	require.Len(t, templates, 2)
	inactiveParsed := 0
	for _, tmpl := range templates {
		if tmpl.Code == "site_admin" && tmpl.IsActive != nil && !*tmpl.IsActive {
			inactiveParsed = 1
		}
	}
	require.Equal(t, 1, inactiveParsed, "ListRoleTemplates 必须暴露 is_active=false")
	t.Logf("TEMPLATE_INACTIVE_PARSED=%d", inactiveParsed)

	// 5) 空 org_id → 显式 error，不发请求（D6/损坏输入拒绝）。
	f.mu.Lock()
	putsBefore := f.putCalls
	f.mu.Unlock()
	_, err = client.GetUserFunctionalRolesWithToken("user-token-2505", "USER", "")
	require.Error(t, err)
	require.Error(t, client.SetUserFunctionalRolesWithToken("user-token-2505", "USER", "", []string{"site_member"}))
	f.mu.Lock()
	putsAfter := f.putCalls
	f.mu.Unlock()
	require.Equal(t, putsBefore, putsAfter, "空 org_id 不得发出 PUT")
	t.Logf("EMPTY_ORG_REJECTED=1")
}
