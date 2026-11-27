package handlers

// #2552 R1/R3：中转网点的本地租户作用域（D2 违反）与建站 IAM 契约。
//
// 缺陷（#2547 D2 被违反）：`AddTransitSiteMember` 把 `site.OrgID`（中转网点的 branch
// 子组织）当作 `addMembersCore` 的第 4 参数（**本地租户作用域**）传入：
//   - `site_members.tenant_id` / `users.tenant_id` 被写成分支组织 ID → 租户作用域分裂；
//   - 幂等判定按 org 计数，而唯一键 `(tenant_id, site_id, user_id)` 含 tenant →
//     同一用户被重复插入第二条成员行。
// IAM 绑定目标本来就由 core 固定取 `site.OrgID`，与第 4 参数无关。
//
// R3：建站契约缺 `type=branch`、父组织未解析、名冲突返回 502、空租户先建组织、
// 本地落库失败留孤儿组织。
//
// Token lines are printed with fmt.Printf so scripts/verify-2552.sh can grep them
// regardless of go test -v indentation.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// transitOrgCall 记录一次「组织内用户操作」，用于区分两条不同域的参数：
// IAM 目标组织（`sites.org_id`）与本地租户（`sites.tenant_id`）。
//
// #2552 审计 Bug 1：bind 一条路径不足以锁定语义——role / unbind 同样必须指向
// 站点自己的组织；只断言 bind 时，把 role/unbind 误传租户的实现仍会「全绿」。
type transitOrgCall struct {
	Method string
	Org    string // 路径里的 /organizations/{org}/ 段
	User   string // 路径里的 /users/{user}/ 段
	Kind   string // bind | role
	Action string // 仅 Kind=bind：bind | unbind（取自请求体）
}

// transitIAMRecorder 记录中转相关 IAM 调用，并可配置建组织的行为。
type transitIAMRecorder struct {
	mu sync.Mutex

	binds       []string // bind 调用的 org 段
	orgCalls    []transitOrgCall
	deletes     []string // DELETE 组织调用命中的 org
	creates     []map[string]interface{}
	createdOrgs []string
	primaryID   string
	// createMode: "ok" | "conflict" | "error"
	createMode string
	// primaries 决定 ListOrganizations 返回几条 is_primary（0/1/2）。
	primaries int
	createSeq int
}

func (f *transitIAMRecorder) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	// 组织列表：建站解析命名空间主组织（GET /api/v1/organizations?namespace_id=...）
	mux.HandleFunc("/api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		orgs := make([]map[string]interface{}, 0, 3)
		for i := 0; i < f.primaries; i++ {
			orgs = append(orgs, map[string]interface{}{
				"id":         f.primaryID,
				"name":       "平台根组织",
				"is_primary": true,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"organizations": orgs})
	})
	// bind / unbind / role（PUT /api/v1/organizations/{org}/users/{user}/{bind|role}）
	// #2552 审计 Bug 1：按 method + path + body.action 分类记录，才能分别锁定
	// bind / role / unbind 三条路径的 org 段。
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
		parts := strings.Split(trimmed, "/")
		if len(parts) >= 4 && parts[1] == "users" {
			org, user, kind := parts[0], parts[2], parts[3]
			action := ""
			if kind == "bind" {
				raw := new(bytes.Buffer)
				_, _ = raw.ReadFrom(r.Body)
				var parsed map[string]interface{}
				_ = json.Unmarshal(raw.Bytes(), &parsed)
				if s, ok := parsed["action"].(string); ok {
					action = s
				}
			}
			f.mu.Lock()
			f.orgCalls = append(f.orgCalls, transitOrgCall{
				Method: r.Method, Org: org, User: user, Kind: kind, Action: action,
			})
			if kind == "bind" {
				f.binds = append(f.binds, org)
			}
			f.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	// 用户列表 / 创建（new_users 路径）
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			// CreateOrGetUser 直接反序列化到 CreateUserResult（扁平结构）。
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"user_id": uuid.New().String(), "status": "active",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": []interface{}{}})
	})
	// 用户角色模板（POST /api/v1/users/{id}/roles）
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	// 命名空间子树：namespace 解析 / 建组织 / 删组织 / 角色模板
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/role-templates"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": uuid.New().String(), "code": "site_member"},
				{"id": uuid.New().String(), "code": "site_admin"},
			})
		case strings.Contains(r.URL.Path, "/organizations/"):
			// DELETE 组织（补偿）
			orgID := ""
			if i := strings.LastIndex(r.URL.Path, "/"); i >= 0 {
				orgID = r.URL.Path[i+1:]
			}
			f.mu.Lock()
			f.deletes = append(f.deletes, orgID)
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/organizations"):
			body := new(bytes.Buffer)
			_, _ = body.ReadFrom(r.Body)
			var parsed map[string]interface{}
			_ = json.Unmarshal(body.Bytes(), &parsed)
			f.mu.Lock()
			f.creates = append(f.creates, parsed)
			mode := f.createMode
			newOrgID := uuid.New().String()
			f.mu.Unlock()
			switch mode {
			case "conflict":
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"code":40900,"message":"organization name exists"}`))
				return
			case "error":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			f.mu.Lock()
			f.createdOrgs = append(f.createdOrgs, newOrgID)
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id":   newOrgID,
				"admin_id": uuid.New().String(),
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.New().String()})
		}
	})
	return mux
}

func (f *transitIAMRecorder) snapshot() (binds, deletes []string, creates []map[string]interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := append([]string(nil), f.binds...)
	d := append([]string(nil), f.deletes...)
	c := make([]map[string]interface{}, len(f.creates))
	copy(c, f.creates)
	return b, d, c
}

// orgCallsSnapshot 返回组织内用户操作的分类记录（bind / unbind / role）。
func (f *transitIAMRecorder) orgCallsSnapshot() []transitOrgCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]transitOrgCall, len(f.orgCalls))
	copy(out, f.orgCalls)
	return out
}

func installTransitIAM(t *testing.T, rec *transitIAMRecorder) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(rec.handler())
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv
}

// transitActorRouter2552 用请求头注入 actor，便于同一 router 测「空租户」等场景。
func transitActorRouter2552() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tid := c.GetHeader("X-Test-Tenant")
		actor := testutil.TestActor{
			TenantID: tid, OrgID: tid, UserID: uuid.New().String(), Role: "admin",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/admin/transit-sites", CreateAdminTransitSite)
	r.POST("/api/admin/transit-sites/:id/members", AddTransitSiteMember)
	r.PUT("/api/admin/transit-sites/:id/members/:user_id", UpdateTransitSiteMemberRole)
	r.DELETE("/api/admin/transit-sites/:id/members/:user_id", RemoveTransitSiteMember)
	return r
}

func doTransitJSON2552(t *testing.T, r *gin.Engine, method, path, tenantID string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-user-token")
	if tenantID != "" {
		req.Header.Set("X-Test-Tenant", tenantID)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp
}

// seedTransitSite2552 建一个「租户与组织不同域」的中转网点（#2547 形态）。
func seedTransitSite2552(t *testing.T, tenantID string) models.Site {
	t.Helper()
	site := models.Site{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    uuid.New().String(), // branch 子组织 ≠ 平台租户
		Name:     "上海中转站",
		Type:     "transit",
		Status:   "active",
	}
	require.NoError(t, database.GetDB().Create(&site).Error)
	return site
}

// R1：成员行的 tenant 列 = 平台租户；IAM 绑定目标 = 站点 org；重复添加不产生第二行。
func TestTransitMemberScope2552_RepeatedAddKeepsSingleRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2552aa0c000a")
	testfixtures.SeedTenantFK(t, db, tenantID)

	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	installTransitIAM(t, rec)

	site := seedTransitSite2552(t, tenantID)
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: site.OrgID,
		Name: "中转成员", Email: "transit2552@example.com", Role: "site_member", Status: "active",
	}).Error)

	router := transitActorRouter2552()
	path := "/api/admin/transit-sites/" + site.ID + "/members"

	status, resp := doTransitJSON2552(t, router, http.MethodPost, path, tenantID,
		map[string]interface{}{"user_id": userID, "role": "site_member"})
	require.Equal(t, http.StatusOK, status, "first add: %v", resp)

	// 重复添加同一用户：幂等判定必须命中（旧行为会插入第二条行）。
	status, resp = doTransitJSON2552(t, router, http.MethodPost, path, tenantID,
		map[string]interface{}{"user_id": userID, "role": "site_member"})
	require.Equal(t, http.StatusBadRequest, status, "second add should be a no-op: %v", resp)

	var members []models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", site.ID, userID).Find(&members).Error)

	binds, _, _ := rec.snapshot()
	bindToSiteOrg := 0
	for _, org := range binds {
		if org == site.OrgID {
			bindToSiteOrg = 1
		}
	}
	memberTenantIsLocal := 0
	if len(members) == 1 && members[0].TenantID == tenantID {
		memberTenantIsLocal = 1
	}

	fmt.Printf("MEMBER_ROWS=%d\n", len(members))
	fmt.Printf("MEMBER_TENANT_IS_LOCAL_TENANT=%d\n", memberTenantIsLocal)
	fmt.Printf("MEMBER_IAM_ORG_IS_SITE_ORG=%d\n", bindToSiteOrg)
	fmt.Printf("NO_DUPLICATE_MEMBER_ROW=%d\n", flag2526(len(members) == 1))

	require.Equal(t, 1, len(members), "重复添加不得产生第二条成员行")
	require.Equal(t, tenantID, members[0].TenantID, "site_members.tenant_id 必须是本地平台租户")
	require.Equal(t, 1, bindToSiteOrg, "IAM 绑定目标必须是站点自己的组织")
}

// R1（续）：new_users 路径下 users.tenant_id 同样必须是平台租户。
func TestTransitMemberScope2552_NewUserTenantIsLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2552aa0c000b")
	testfixtures.SeedTenantFK(t, db, tenantID)

	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	installTransitIAM(t, rec)

	site := seedTransitSite2552(t, tenantID)
	router := transitActorRouter2552()

	status, resp := doTransitJSON2552(t, router, http.MethodPost,
		"/api/admin/transit-sites/"+site.ID+"/members", tenantID,
		map[string]interface{}{
			"new_users": []map[string]interface{}{{
				"name": "新中转成员", "email": "newtransit2552@example.com", "phone": "13800002552", "role": "site_member",
			}},
		})
	require.Equal(t, http.StatusOK, status, "new user add: %v", resp)

	var created []models.User
	require.NoError(t, db.Where("email = ?", "newtransit2552@example.com").Find(&created).Error)
	require.Len(t, created, 1, "新建用户必须落库")

	userTenantIsLocal := 0
	if created[0].TenantID == tenantID {
		userTenantIsLocal = 1
	}
	fmt.Printf("USER_TENANT_IS_LOCAL_TENANT=%d\n", userTenantIsLocal)
	fmt.Printf("USER_ORG_IS_SITE_ORG=%d\n", flag2526(created[0].OrgID == site.OrgID))
	require.Equal(t, tenantID, created[0].TenantID, "users.tenant_id 必须是本地平台租户，不是分支组织")
}

// R1b：bind / role / unbind 三条 IAM 路径的 org 段都必须是站点自己的组织
// （`sites.org_id`），而不是本地租户（`sites.tenant_id`）。
//
// #2552 审计 Bug 1：计划 P1-1 要求「补断言锁定 bind/role/unbind 三路径 org 段 =
// sites.org_id」，但 R1 只锁了 bind；role 路径完全未记录，unbind 未区分 action。
// 夹具必须满足 `site.OrgID ≠ site.TenantID`，否则断言无法区分两条不同域的参数
// （`seedTransitSite1935` 的 `OrgID = tenantID` 即使断言也区分不了）。
func TestTransitMemberOrgPaths2552_RoleAndUnbind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2552aa0c000c")
	testfixtures.SeedTenantFK(t, db, tenantID)

	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	installTransitIAM(t, rec)

	site := seedTransitSite2552(t, tenantID)
	require.NotEqual(t, site.OrgID, site.TenantID, "夹具前提：站点组织与平台租户必须是不同域的值")

	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: site.OrgID,
		Name: "中转成员路径", Email: "paths2552@example.com", Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: site.ID, UserID: userID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	router := transitActorRouter2552()
	memberPath := "/api/admin/transit-sites/" + site.ID + "/members/" + userID

	// 改角色 → PUT /organizations/{org}/users/{uid}/role
	status, resp := doTransitJSON2552(t, router, http.MethodPut, memberPath, tenantID,
		map[string]interface{}{"role": "site_admin"})
	require.Equal(t, http.StatusOK, status, "role update: %v", resp)

	// 移除成员 → PUT /organizations/{org}/users/{uid}/bind action=unbind
	status, resp = doTransitJSON2552(t, router, http.MethodDelete, memberPath, tenantID, nil)
	require.Equal(t, http.StatusOK, status, "remove member: %v", resp)

	calls := rec.orgCallsSnapshot()
	roleOrgIsSiteOrg, unbindOrgIsSiteOrg := 0, 0
	for _, call := range calls {
		if call.User != userID {
			continue
		}
		switch {
		case call.Kind == "role" && call.Method == http.MethodPut && call.Org == site.OrgID:
			roleOrgIsSiteOrg = 1
		case call.Kind == "bind" && call.Action == "unbind" && call.Org == site.OrgID:
			unbindOrgIsSiteOrg = 1
		}
	}

	fmt.Printf("MEMBER_ROLE_ORG_IS_SITE_ORG=%d\n", roleOrgIsSiteOrg)
	fmt.Printf("MEMBER_UNBIND_ORG_IS_SITE_ORG=%d\n", unbindOrgIsSiteOrg)

	require.Equal(t, 1, roleOrgIsSiteOrg,
		"IAM 改角色路径的 org 段必须是站点组织（sites.org_id），不是本地租户")
	require.Equal(t, 1, unbindOrgIsSiteOrg,
		"IAM 解绑路径的 org 段必须是站点组织（sites.org_id），不是本地租户")
}

// R3：建站契约（type=branch / parent=主组织 / 无 admin_info）+ 空租户前置拒绝 + 补偿。
func TestTransitSiteCreate2552_Contract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2552bb0c000a")
	testfixtures.SeedTenantFK(t, db, tenantID)

	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	installTransitIAM(t, rec)
	router := transitActorRouter2552()

	status, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
		map[string]interface{}{"name": "广州中转站", "address": "广州大道1号", "phone": "020-1234"})
	require.Equal(t, http.StatusOK, status, "create: %v", resp)

	_, _, creates := rec.snapshot()
	require.Len(t, creates, 1, "必须发起一次建组织")
	body := creates[0]
	typeBranch := 0
	if body["type"] == "branch" {
		typeBranch = 1
	}
	parentIsNs := 0
	if body["parent_id"] == rec.primaryID {
		parentIsNs = 1
	}
	noAdmin := 0
	if _, ok := body["admin_info"]; !ok {
		noAdmin = 1
	}
	fmt.Printf("CREATE_TYPE_BRANCH=%d\n", typeBranch)
	fmt.Printf("CREATE_PARENT_IS_NAMESPACE=%d\n", parentIsNs)
	fmt.Printf("CREATE_NO_ADMIN_INFO=%d\n", noAdmin)
	require.Equal(t, 1, typeBranch, "建组织必须带 type=branch")
	require.Equal(t, 1, parentIsNs, "parent_id 必须是命名空间主组织")
	require.Equal(t, 1, noAdmin, "中转网点无 owner，不得带 admin_info")

	// 名冲突 → 409/40900（旧行为 502/50200）
	rec.mu.Lock()
	rec.createMode = "conflict"
	rec.mu.Unlock()
	status, resp = doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
		map[string]interface{}{"name": "广州中转站", "address": "广州大道1号", "phone": "020-1234"})
	dup409 := 0
	if status == http.StatusConflict {
		if code, ok := resp["code"].(float64); ok && int(code) == 40900 {
			dup409 = 1
		}
	}
	fmt.Printf("DUPLICATE_NAME_409=%d\n", dup409)
	require.Equal(t, 1, dup409, "同名组织必须映射为 409/40900")

	// 空租户 → 400/40002 且**不调 IAM**、不建站
	_, _, createsBefore := rec.snapshot()
	status, resp = doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", "",
		map[string]interface{}{"name": "无租户中转站", "address": "x", "phone": "1"})
	_, _, createsAfter := rec.snapshot()
	var sitesAfterEmpty int64
	require.NoError(t, db.Model(&models.Site{}).Where("name = ?", "无租户中转站").Count(&sitesAfterEmpty).Error)
	emptyTenantOK := 0
	if status == http.StatusBadRequest && len(createsAfter) == len(createsBefore) && sitesAfterEmpty == 0 {
		emptyTenantOK = 1
	}
	fmt.Printf("NO_SITE_ON_EMPTY_TENANT=%d\n", emptyTenantOK)
	require.Equal(t, 1, emptyTenantOK, "空租户必须 400/40002 且不建组织/不建站")
	require.NotNil(t, resp)
}

// R3：主组织不可解析（0 个 / 2 个 is_primary）→ 500/50014 且不建站。
//
// #2597 翻转：旧断言把「配置不变量违反」钉在 50000（与瞬时故障同码同文案），
// 现改用专用码 50014 + 可行动文案（含「联系平台管理员」、不含「请稍后再试」）。
func TestTransitSiteCreate2552_PrimaryUnresolved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2552bb0c000b")
	testfixtures.SeedTenantFK(t, db, tenantID)

	for _, primaries := range []int{0, 2} {
		rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: primaries}
		srv := installTransitIAM(t, rec)
		router := transitActorRouter2552()

		status, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
			map[string]interface{}{"name": fmt.Sprintf("主组织异常站%d", primaries), "address": "x", "phone": "1"})
		_, _, creates := rec.snapshot()
		var sites int64
		require.NoError(t, db.Model(&models.Site{}).
			Where("name = ?", fmt.Sprintf("主组织异常站%d", primaries)).Count(&sites).Error)
		// #2597：配置不变量违反使用专用码 50014；文案必须可行动（联系平台管理员），
		// 且不得出现误导性的「请稍后再试」。组织 id / 计数只进日志，不进响应。
		code, _ := resp["code"].(float64)
		msg, _ := resp["message"].(string)
		ok := status == http.StatusInternalServerError && len(creates) == 0 && sites == 0 &&
			int(code) == 50014 &&
			strings.Contains(msg, "联系平台管理员") &&
			!strings.Contains(msg, "请稍后再试")
		unresolvedToken := 0
		if ok {
			unresolvedToken = int(code)
		}
		fmt.Printf("PRIMARY_ORG_UNRESOLVED=%d\n", unresolvedToken)
		fmt.Printf("PRIMARY_ORG_CASE primaries=%d status=%d\n", primaries, status)
		require.True(t, ok, "is_primary 非恰好 1 个时必须 500/50014 + 可行动文案且不建站/不建组织: %v", resp)
		srv.Close()
	}
}

// R3：本地落库失败 → best-effort 删除已建组织，不留孤儿。
func TestTransitSiteCreate2552_OrphanCompensated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	installTransitIAM(t, rec)
	router := transitActorRouter2552()

	// 非空但不存在的租户：绕过空租户前置校验，让 sites 插入触发 FK 失败。
	ghostTenant := uuid.New().String()
	// 超长站点名（>255）会让 `sites.name`（varchar(255)）插入失败：
	// 用于在「IAM 组织已建成功」之后注入本地落库失败，验证补偿路径。
	longName := strings.Repeat("长", 300)
	status, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", ghostTenant,
		map[string]interface{}{"name": longName, "address": "x", "phone": "1"})

	_, deletes, creates := rec.snapshot()
	compensated := 0
	if len(creates) == 1 && len(deletes) == 1 {
		compensated = 1
	}
	var sites int64
	require.NoError(t, db.Model(&models.Site{}).Where("length(name) > 255").Count(&sites).Error)

	fmt.Printf("ORPHAN_ORG_COMPENSATED=%d\n", compensated)
	fmt.Printf("SITE_ROWS_ON_FAILURE=%d\n", sites)
	fmt.Printf("CREATE_STATUS=%d\n", status)
	require.Equal(t, http.StatusInternalServerError, status, "本地落库失败应 500: %v", resp)
	require.Equal(t, 1, compensated, "本地落库失败必须补偿删除已建组织")
	require.Zero(t, sites, "失败时不得留下 sites 行")
}
