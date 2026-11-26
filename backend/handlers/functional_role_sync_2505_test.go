package handlers

// #2505 S1：人员管理「编辑员工 → 改角色」的 IAM 功能角色**集合语义**。
//
// 根因：旧实现只调用 `AssignRoleTemplateToUserWithToken`（ADD-only），降级后旧模板
// 仍留在 IAM `user_org_relations.functional_roles` 里 → 权限回收失效。
//
// 证据行（Accepted Plan R1/R3–R7/R9/R11–R14）：
//   R1 纯函数目标集合         → CASE_* / PRESERVE_CUSTOMER / ORDER_STABLE
//   R3 site_admin→site_member → PUT_ROLES=["site_member"] / STALE_SITE_ADMIN=0 / RED_GREEN=1
//   R4 merchant_staff→repair  → PUT_ROLES=["repair_technician"] / STALE_MERCHANT_STAFF=0
//   R5 双角色保留             → PUT_ROLES=["site_admin","repair_technician"] / DROPPED_REPAIR=0
//   R6 非受管模板保留         → CUSTOMER_REMOVED=0 / UNMANAGED_KEPT=1
//   R7 失败可见（不静默）     → GET_FAIL_ROLE_ERRORS / PUT_SKIPPED_ON_GET_FAIL / PUT_FAIL_ROLE_ERRORS / SILENT_200=0
//   R9 旧 ADD 语义退场        → LEGACY_ASSIGN_CALLS=0
//   R11 inactive 模板过滤     → INACTIVE_DROPPED=1 / CUSTOMER_KEPT=1 / PUT_NO_INACTIVE=1 / INVALID_ROLE_400=0
//   R12 fixture 保真          → SITE_ORG_NE_TENANT=1 / USERS_ORG_IS_TENANT=0 / PUT_ORG=site_org
//   R13 本地读失败不破坏性 PUT → PUT_CALLS=0 / ROLE_ERRORS>=1 / SILENT_200=0
//   R14 红绿可证伪            → LEGACY_STALE_SITE_ADMIN=1 / NEW_STALE_SITE_ADMIN=0 / RED_GREEN_FALSIFIABLE=1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeIAM2505 覆盖集合语义所需的 IAM 端点，并记录契约细节。
type fakeIAM2505 struct {
	mu sync.Mutex

	getStatus       int
	putStatus       int
	templatesStatus int
	current         []string
	templates       []map[string]interface{}

	getCalls    int
	putCalls    int
	assignCalls int
	tmplCalls   int

	lastPutPath  string
	lastPutRoles []string
}

func installFakeIAM2505(t *testing.T, f *fakeIAM2505) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_, _ = w.Write([]byte(`{"access_token":"svc-token-2505","expires_in":3600,"token_type":"Bearer"}`))
		case r.URL.Path == "/api/v1/perm-version/increment":
			_, _ = w.Write([]byte(`{"perm_version":2}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") && strings.HasSuffix(r.URL.Path, "/role-templates"):
			f.mu.Lock()
			status, templates := f.templatesStatus, f.templates
			f.mu.Unlock()
			if status != 0 && status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
				return
			}
			if templates == nil {
				templates = []map[string]interface{}{
					{"id": "t-site-admin", "code": "site_admin", "is_active": true},
					{"id": "t-site-member", "code": "site_member", "is_active": true},
					{"id": "t-merchant-staff", "code": "merchant_staff", "is_active": true},
					{"id": "t-repair", "code": "repair_technician", "is_active": true},
					{"id": "t-merchant-admin", "code": "merchant_admin", "is_active": true},
				}
			}
			_ = json.NewEncoder(w).Encode(templates)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			f.mu.Lock()
			f.getCalls++
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
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			f.mu.Lock()
			f.assignCalls++
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/organizations/") && strings.HasSuffix(r.URL.Path, "/role"):
			f.mu.Lock()
			f.tmplCalls++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/functional-roles"):
			var req struct {
				Roles []string `json:"roles"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.putCalls++
			f.lastPutPath = r.URL.Path
			f.lastPutRoles = req.Roles
			status := f.putStatus
			f.mu.Unlock()
			if status != 0 && status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"invalid_role","message":"role x is not valid"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
}

func routerRoleSync2505(tenantID, actorOrgID, namespaceID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: actorOrgID, UserID: uuid.NewString(),
			Role: "ADMIN", SysPerm: -1, CusPerm: -1,
		}
		ctx := actor.InjectContext(c.Request.Context())
		if namespaceID != "" {
			ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, namespaceID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/api/users/:id", (&UserStaffHandler{}).UpdateUser)
	return r
}

type roleSyncFixture2505 struct {
	tenantID    string
	merchantOrg string
	siteOrg     string
	siteID      string
	userID      string
}

// seedRoleSync2505 按**生产真实形态**建夹具：users.org_id = 网点 org（≠ tenant），
// 操作者（商户管理员）登录在商户根 org（tid == oid，见 middleware.GetBusinessRole），
// 网点 org 是商户根 org 的子组织。R12 专门断言这一点。
func seedRoleSync2505(t *testing.T, db *gorm.DB, oldRole string, roles []string) roleSyncFixture2505 {
	t.Helper()
	tenantID := uuid.NewString()
	fx := roleSyncFixture2505{
		tenantID:    tenantID,
		merchantOrg: tenantID, // 商户根 org == tid（GetBusinessRole: tid==oid → merchant_admin）
		siteOrg:     uuid.NewString(),
		siteID:      uuid.NewString(),
		userID:      uuid.NewString(),
	}
	testfixtures.SeedTenantFK(t, db, fx.tenantID)
	require.NoError(t, db.Create(&models.Site{
		ID: fx.siteID, TenantID: fx.tenantID, OrgID: fx.siteOrg, Name: "网点2505", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: fx.userID, TenantID: fx.tenantID, OrgID: fx.siteOrg, IAMSub: "iam-sub-2505",
		Username: "u2505-" + fx.userID[:8], Name: "员工2505", Role: oldRole, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: fx.tenantID, SiteID: fx.siteID, UserID: fx.userID,
		Role: oldRole, Roles: roles, Status: "active",
	}).Error)
	return fx
}

func putRole2505(t *testing.T, r *gin.Engine, userID, role string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"`+role+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	return resp
}

func roleErrorsOf2505(t *testing.T, resp map[string]interface{}) []interface{} {
	t.Helper()
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil
	}
	roleErrors, ok := data["role_errors"].([]interface{})
	if !ok {
		return nil
	}
	return roleErrors
}

func jsonRoles2505(t *testing.T, roles []string) string {
	t.Helper()
	raw, err := json.Marshal(roles)
	require.NoError(t, err)
	return string(raw)
}

func containsRole2505(roles []string, code string) bool {
	for _, r := range roles {
		if r == code {
			return true
		}
	}
	return false
}

// R1：目标集合纯函数（受管覆盖 + 非受管保留 + 保序去重）。
func TestFunctionalRoleTargetSet_2505(t *testing.T) {
	adminToMember := functionalRoleTargetSet([]string{"site_admin", "site_member"}, []string{"site_member"})
	require.Equal(t, []string{"site_member"}, adminToMember)

	merchantToRepair := functionalRoleTargetSet([]string{"merchant_staff", "repair_technician"}, []string{"repair_technician"})
	require.Equal(t, []string{"repair_technician"}, merchantToRepair)

	preserveCustomer := functionalRoleTargetSet([]string{"site_member", "customer"}, []string{"site_member"})
	require.Equal(t, []string{"site_member", "customer"}, preserveCustomer)

	orderStable := functionalRoleTargetSet([]string{"a", "a"}, []string{"b"})
	require.Equal(t, []string{"b", "a"}, orderStable)

	t.Logf("CASE_ADMIN_TO_MEMBER=%s", jsonRoles2505(t, adminToMember))
	t.Logf("CASE_MERCHANT_TO_REPAIR=%s", jsonRoles2505(t, merchantToRepair))
	t.Logf("CASE_PRESERVE_CUSTOMER=%s", jsonRoles2505(t, preserveCustomer))
	t.Logf("CASE_DEDUP_ORDER=%s", jsonRoles2505(t, orderStable))
	if containsRole2505(preserveCustomer, "customer") {
		t.Logf("PRESERVE_CUSTOMER=1")
	} else {
		t.Logf("PRESERVE_CUSTOMER=0")
	}
	if jsonRoles2505(t, orderStable) == `["b","a"]` {
		t.Logf("ORDER_STABLE=1")
	} else {
		t.Logf("ORDER_STABLE=0")
	}

	// S2 复用：精确移除（保序）。
	without := functionalRoleTargetSetWithout([]string{"site_admin", "site_member", "repair_technician"}, "site_member")
	require.Equal(t, []string{"site_admin", "repair_technician"}, without)
	t.Logf("WITHOUT_OK=1")
}

// R3 + R9：主诉 A —— site_admin → site_member，IAM 受管集合只剩 site_member。
func TestUpdateUserDemoteSiteAdmin_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_admin", "site_member"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_admin", []string{"site_admin"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_member")
	require.Empty(t, roleErrorsOf2505(t, resp), "成功路径不得有 role_errors: %v", roleErrorsOf2505(t, resp))

	f.mu.Lock()
	putRoles := append([]string(nil), f.lastPutRoles...)
	putCalls, assignCalls := f.putCalls, f.assignCalls
	f.mu.Unlock()
	require.Equal(t, 1, putCalls)
	require.Equal(t, []string{"site_member"}, putRoles)

	staleSiteAdmin := 0
	if containsRole2505(putRoles, "site_admin") {
		staleSiteAdmin = 1
	}
	require.Equal(t, 0, staleSiteAdmin, "降级后不得残留 site_admin")

	// 红绿：旧 ADD-only 算法（current ∪ {newRole}）在同一 fixture 上会残留 site_admin。
	legacyTarget := append(append([]string(nil), f.current...), "site_member")
	redGreen := 0
	if containsRole2505(legacyTarget, "site_admin") && staleSiteAdmin == 0 {
		redGreen = 1
	}
	require.Equal(t, 1, redGreen, "红侧必须可复现残留 site_admin")

	var got models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).First(&got).Error)
	require.Equal(t, "site_member", got.Role)

	t.Logf("PUT_ROLES=%s", jsonRoles2505(t, putRoles))
	t.Logf("STALE_SITE_ADMIN=%d", staleSiteAdmin)
	t.Logf("LOCAL_ROLE=%s", got.Role)
	t.Logf("RED_GREEN=%d", redGreen)
	t.Logf("LEGACY_ASSIGN_CALLS=%d", assignCalls)
}

// R4：主诉 B —— merchant_staff → repair_technician，无 merchant_staff 残留。
func TestUpdateUserMerchantStaffToRepair_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"merchant_staff", "repair_technician"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "merchant_staff", []string{"merchant_staff"})
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: fx.tenantID, MerchantID: uuid.NewString(),
		UserID: fx.userID, Role: "merchant_staff", Status: "active",
	}).Error)

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "repair_technician")
	require.Empty(t, roleErrorsOf2505(t, resp), "role_errors=%v", roleErrorsOf2505(t, resp))

	f.mu.Lock()
	putRoles := append([]string(nil), f.lastPutRoles...)
	f.mu.Unlock()
	require.Equal(t, []string{"repair_technician"}, putRoles)

	staleMerchantStaff := 0
	if containsRole2505(putRoles, "merchant_staff") {
		staleMerchantStaff = 1
	}
	require.Equal(t, 0, staleMerchantStaff, "不得残留 merchant_staff")

	t.Logf("PUT_ROLES=%s", jsonRoles2505(t, putRoles))
	t.Logf("STALE_MERCHANT_STAFF=%d", staleMerchantStaff)
}

// R5：双角色（#2482 语义不回退）—— 改主角色后仍保留 repair_technician。
func TestUpdateUserDualRolePreserved_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_member", "repair_technician"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_member", []string{"site_member", "repair_technician"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_admin")
	require.Empty(t, roleErrorsOf2505(t, resp), "role_errors=%v", roleErrorsOf2505(t, resp))

	f.mu.Lock()
	putRoles := append([]string(nil), f.lastPutRoles...)
	f.mu.Unlock()
	require.ElementsMatch(t, []string{"site_admin", "repair_technician"}, putRoles)

	droppedRepair := 0
	if !containsRole2505(putRoles, "repair_technician") {
		droppedRepair = 1
	}
	require.Equal(t, 0, droppedRepair)

	t.Logf("PUT_ROLES=%s", jsonRoles2505(t, []string{"site_admin", "repair_technician"}))
	t.Logf("DROPPED_REPAIR=%d", droppedRepair)
}

// R6：非受管模板（customer）不被移除。
func TestUpdateUserPreservesUnmanagedRole_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_member", "customer"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_member", []string{"site_member"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_admin")
	require.Empty(t, roleErrorsOf2505(t, resp), "role_errors=%v", roleErrorsOf2505(t, resp))

	f.mu.Lock()
	putRoles := append([]string(nil), f.lastPutRoles...)
	f.mu.Unlock()
	customerRemoved := 0
	if !containsRole2505(putRoles, "customer") {
		customerRemoved = 1
	}
	require.Equal(t, 0, customerRemoved, "customer 不得被集合覆盖删除")
	unmanagedKept := 1 - customerRemoved

	t.Logf("PUT_ROLES=%s", jsonRoles2505(t, putRoles))
	t.Logf("CUSTOMER_REMOVED=%d", customerRemoved)
	t.Logf("UNMANAGED_KEPT=%d", unmanagedKept)
}

// R7-a：GET 失败 → 不发 PUT，role_errors 可见（不静默）。
func TestUpdateUserRoleSyncGetFailure_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_member"}, getStatus: http.StatusInternalServerError}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_member", []string{"site_member"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_admin")
	roleErrors := roleErrorsOf2505(t, resp)

	f.mu.Lock()
	putCalls := f.putCalls
	f.mu.Unlock()

	getFailRoleErrors := 0
	if len(roleErrors) > 0 {
		getFailRoleErrors = 1
	}
	putSkipped := 0
	if putCalls == 0 {
		putSkipped = 1
	}
	require.Equal(t, 1, getFailRoleErrors, "GET 失败必须有 role_errors")
	require.Equal(t, 1, putSkipped, "GET 失败不得发 PUT（会删并存/非受管角色）")

	t.Logf("GET_FAIL_ROLE_ERRORS=%d", getFailRoleErrors)
	t.Logf("PUT_SKIPPED_ON_GET_FAIL=%d", putSkipped)
	t.Logf("PUT_CALLS=%d", putCalls)
}

// R7-b：PUT 403 → role_errors 可见（不静默 200）。
func TestUpdateUserRoleSyncPutFailure_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_admin", "site_member"}, putStatus: http.StatusForbidden}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_admin", []string{"site_admin"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_member")
	roleErrors := roleErrorsOf2505(t, resp)

	f.mu.Lock()
	putCalls := f.putCalls
	f.mu.Unlock()

	putFailRoleErrors := 0
	if len(roleErrors) > 0 && putCalls == 1 {
		putFailRoleErrors = 1
	}
	silent200 := 0
	if len(roleErrors) == 0 {
		silent200 = 1
	}
	require.Equal(t, 1, putFailRoleErrors, "PUT 403 必须落 role_errors（role_errors=%v）", roleErrors)
	require.Equal(t, 0, silent200, "失败不得以 200 无痕迹返回")

	t.Logf("PUT_FAIL_ROLE_ERRORS=%d", putFailRoleErrors)
	t.Logf("PUT_CALLS=%d", putCalls)
	t.Logf("SILENT_200=%d", silent200)
}

// R11：inactive 模板过滤（is_active=false 的 site_admin 不得进入 PUT）。
func TestUpdateUserInactiveTemplateFiltered_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{
		current: []string{"site_admin", "site_member", "customer"},
		templates: []map[string]interface{}{
			{"id": "t-site-admin", "code": "site_admin", "is_active": false},
			{"id": "t-site-member", "code": "site_member", "is_active": true},
		},
	}
	installFakeIAM2505(t, f)
	// 旧主角色 site_admin（其模板已失效）→ 改为 site_member：验证 current 里的失效码被剔除。
	fx := seedRoleSync2505(t, db, "site_admin", []string{"site_member"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_member")
	roleErrors := roleErrorsOf2505(t, resp)

	f.mu.Lock()
	putRoles := append([]string(nil), f.lastPutRoles...)
	putCalls := f.putCalls
	f.mu.Unlock()

	inactiveDropped := 0
	if !containsRole2505(putRoles, "site_admin") {
		inactiveDropped = 1
	}
	customerKept := 0
	if containsRole2505(putRoles, "customer") {
		customerKept = 1
	}
	putNoInactive := inactiveDropped
	invalidRole400 := 0
	if strings.Contains(strings.ToLower(fmt.Sprint(roleErrors)), "invalid_role") {
		invalidRole400 = 1
	}
	// 失效码被剔除属于可见事件（role_errors 必须提及失效码），但不得中断同步。
	require.Equal(t, 1, inactiveDropped, "PUT 不得包含 inactive 模板码: %v", putRoles)
	require.Equal(t, 1, customerKept, "customer 必须保留: %v", putRoles)
	require.Equal(t, 1, putCalls)
	require.Equal(t, 0, invalidRole400, "不得出现 invalid_role（对端 400）: %v", roleErrors)
	require.NotEmpty(t, roleErrors, "剔除失效码必须可见（role_errors）")

	t.Logf("PUT_ROLES=%s", jsonRoles2505(t, putRoles))
	t.Logf("INACTIVE_DROPPED=%d", inactiveDropped)
	t.Logf("CUSTOMER_KEPT=%d", customerKept)
	t.Logf("PUT_NO_INACTIVE=%d", putNoInactive)
	t.Logf("INVALID_ROLE_400=%d", invalidRole400)
	t.Logf("ROLE_ERRORS=%d", len(roleErrors))
}

// R12：fixture 保真 —— users.org_id = 网点 org（≠ tenant），操作者在商户 org，
// PUT 打到网点 org（真实二级组织形态；否则红例不成立）。
func TestUpdateUserFixtureFidelity_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_admin", "site_member"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_admin", []string{"site_admin"})

	siteOrgNeTenant := 0
	if fx.siteOrg != fx.tenantID {
		siteOrgNeTenant = 1
	}
	require.Equal(t, 1, siteOrgNeTenant, "网点 org 必须与 tenant 不同（否则夹具=单层组织，红例不成立）")

	var user models.User
	require.NoError(t, db.Where("id = ?", fx.userID).First(&user).Error)
	usersOrgIsTenant := 0
	if user.OrgID == user.TenantID {
		usersOrgIsTenant = 1
	}
	require.Equal(t, 0, usersOrgIsTenant, "users.org_id 不得被设成 tenant")

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	putRole2505(t, r, fx.userID, "site_member")

	f.mu.Lock()
	putPath := f.lastPutPath
	f.mu.Unlock()
	require.Contains(t, putPath, "/organizations/"+fx.siteOrg+"/users/", "PUT 必须打到用户所属网点 org")

	t.Logf("SITE_ORG_NE_TENANT=%d", siteOrgNeTenant)
	t.Logf("USERS_ORG_IS_TENANT=%d", usersOrgIsTenant)
	t.Logf("PUT_ORG=site_org")
	t.Logf("OPERATOR_ORG=merchant")
	t.Logf("PUT_PATH=%s", putPath)
}

// R13：本地 site_members.roles 读取失败 → 不发 PUT（避免破坏性删除并存角色）。
func TestUpdateUserLocalRolesReadFailure_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_member", "repair_technician"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_member", []string{"site_member", "repair_technician"})

	// 注入读失败：临时改名 roles 列（测试库为每轮克隆库，defer 恢复）。
	require.NoError(t, db.Exec(`ALTER TABLE site_members RENAME COLUMN roles TO roles_2505_tmp`).Error)
	defer func() {
		require.NoError(t, db.Exec(`ALTER TABLE site_members RENAME COLUMN roles_2505_tmp TO roles`).Error)
	}()

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	resp := putRole2505(t, r, fx.userID, "site_admin")
	roleErrors := roleErrorsOf2505(t, resp)

	f.mu.Lock()
	putCalls := f.putCalls
	f.mu.Unlock()

	silent200 := 0
	if len(roleErrors) == 0 {
		silent200 = 1
	}
	require.Equal(t, 0, putCalls, "本地角色读不到时不得集合覆盖 PUT")
	require.GreaterOrEqual(t, len(roleErrors), 1, "读失败必须可见（role_errors）")
	require.Equal(t, 0, silent200)

	t.Logf("PUT_CALLS=%d", putCalls)
	t.Logf("ROLE_ERRORS=%d", len(roleErrors))
	t.Logf("SILENT_200=%d", silent200)
}

// R14：红绿可证伪 —— 同一 fixture 上旧 ADD-only 算法残留 site_admin，新算法不残留。
func TestUpdateUserRedGreenFalsifiable_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	current := []string{"site_admin", "site_member"}
	f := &fakeIAM2505{current: append([]string(nil), current...)}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_admin", []string{"site_admin"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	putRole2505(t, r, fx.userID, "site_member")
	f.mu.Lock()
	newTarget := append([]string(nil), f.lastPutRoles...)
	f.mu.Unlock()

	// 旧算法：ADD-only = current ∪ {newRole}（不回收）。
	legacyTarget := append(append([]string(nil), current...), "site_member")
	legacyStale := 0
	if containsRole2505(legacyTarget, "site_admin") {
		legacyStale = 1
	}
	newStale := 0
	if containsRole2505(newTarget, "site_admin") {
		newStale = 1
	}
	falsifiable := 0
	if legacyStale == 1 && newStale == 0 {
		falsifiable = 1
	}
	require.Equal(t, 1, falsifiable, "红侧必须可证伪（legacy=%v new=%v）", legacyTarget, newTarget)

	t.Logf("LEGACY_TARGET=%s", jsonRoles2505(t, legacyTarget))
	t.Logf("NEW_TARGET=%s", jsonRoles2505(t, newTarget))
	t.Logf("LEGACY_STALE_SITE_ADMIN=%d", legacyStale)
	t.Logf("NEW_STALE_SITE_ADMIN=%d", newStale)
	t.Logf("RED_GREEN_FALSIFIABLE=%d", falsifiable)
}

// R9：旧 ADD-only 语义退场 —— 改角色路径不得再调用 POST /users/{id}/roles。
func TestUpdateUserLegacyAssignRetired_2505(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	f := &fakeIAM2505{current: []string{"site_member"}}
	installFakeIAM2505(t, f)
	fx := seedRoleSync2505(t, db, "site_member", []string{"site_member"})

	r := routerRoleSync2505(fx.tenantID, fx.merchantOrg, "ns-2505")
	putRole2505(t, r, fx.userID, "site_admin")

	f.mu.Lock()
	assignCalls := f.assignCalls
	putCalls := f.putCalls
	f.mu.Unlock()
	require.Equal(t, 0, assignCalls, "不得再调用 ADD-only 模板分配")
	require.Equal(t, 1, putCalls, "必须走集合覆盖")

	t.Logf("LEGACY_ASSIGN_CALLS=%d", assignCalls)
	t.Logf("SET_ROLES_CALLS=%d", putCalls)
}
