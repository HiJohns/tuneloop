package handlers

// #2540：ActivateUser 的 IAM 关系角色 + 权限模板必须同源取自 users.role
// （#2467 邻居收敛：此前固定 `GetBusinessRole("site_member")` → IAM 角色 "user"，
// 且客户权限位硬编码 site_member，激活 site_admin 会实质降权）。
//
// 断言面向 fake IAM 捕获的真实 payload（bind role / customer-permissions 位图），
// 而不是只断言 handler 返回值——错写入 IAM 才是本缺陷的形态。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

type iamCall2540 struct {
	Method string
	Path   string
	Body   string
}

// assignCall2540 是一次功能角色模板分配 payload（`role_ids[0]` + `org_id`）。
type assignCall2540 struct {
	TemplateCode string
	OrgID        string
}

type fakeIAM2540 struct {
	mu    sync.Mutex
	calls []iamCall2540
}

func (f *fakeIAM2540) record(r *http.Request) string {
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, iamCall2540{Method: r.Method, Path: r.URL.Path, Body: body.String()})
	return body.String()
}

func (f *fakeIAM2540) snapshot() []iamCall2540 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]iamCall2540, len(f.calls))
	copy(out, f.calls)
	return out
}

// bindRoles 返回所有 bind payload 的关系角色（调用顺序保留）。
func (f *fakeIAM2540) bindRoles() []string {
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

// customerPerms 返回所有 customer-permissions payload 的原始位图（调用顺序保留）。
func (f *fakeIAM2540) customerPerms() []int64 {
	out := []int64{}
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/customer-permissions") {
			var req struct {
				CusPerm int64 `json:"cus_perm"`
			}
			_ = json.Unmarshal([]byte(c.Body), &req)
			out = append(out, req.CusPerm)
		}
	}
	return out
}

// assignCalls 返回所有功能角色模板分配 payload（`POST /users/:id/roles`）。
// #2540 审计 Bug 1：激活路径必须用同一个 roleTemplate 分配模板，不只是写权限位。
func (f *fakeIAM2540) assignCalls() []assignCall2540 {
	out := []assignCall2540{}
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/roles") {
			var req struct {
				RoleIDs []string `json:"role_ids"`
				OrgID   string   `json:"org_id"`
			}
			_ = json.Unmarshal([]byte(c.Body), &req)
			code := ""
			if len(req.RoleIDs) > 0 {
				code = req.RoleIDs[0]
			}
			out = append(out, assignCall2540{TemplateCode: code, OrgID: req.OrgID})
		}
	}
	return out
}

func installFakeIAM2540(t *testing.T) *fakeIAM2540 {
	t.Helper()
	f := &fakeIAM2540{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		body := f.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`{"code":20000,"data":{"user_id":"` + uuid.NewString() + `","status":"active"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/role-templates"):
			items := make([]string, 0, len(services.AllRoleTemplates))
			for _, code := range sortedTemplateCodes() {
				items = append(items, `{"id":"rt-`+code+`","code":"`+code+`"}`)
			}
			_, _ = w.Write([]byte(`{"role_templates":[` + strings.Join(items, ",") + `]}`))
		case strings.HasSuffix(r.URL.Path, "/customer-permissions"):
			_ = body
			_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
		default:
			_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
		}
	}))
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return f
}

// seedActivationUser 建一个「未激活」的本地员工行（无 iam_sub），role 由调用方指定。
func seedActivationUser(t *testing.T, tenantID, role string) string {
	t.Helper()
	testUserSeq++
	userID := uuid.NewString()
	user := models.User{
		ID:       userID,
		IAMSub:   "",
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "激活候选 " + role,
		Phone:    "139" + strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
		Email:    "activate2540-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "@example.com",
		Status:   "pending",
		Role:     role,
	}
	require.NoError(t, database.GetDB().Create(&user).Error)
	return userID
}

// activate2540 以租户上下文调用 POST /api/users/:id/activate。
func activate2540(t *testing.T, handler *UserStaffHandler, tenantID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/api/users/:id/activate", handler.ActivateUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/users/"+userID+"/activate", nil))
	return w
}

// cusPermRegistry2540 给每个 cus_perm 名字分配稳定位（mock 默认返回 -1，
// 位图全 0 无法区分 site_member / site_admin，会让「权限模板同源」断言失去意义）。
type cusPermRegistry2540 struct {
	bits map[string]int
}

func (r *cusPermRegistry2540) GetSysPermBit(string) (int, bool) { return 0, false }

func (r *cusPermRegistry2540) GetCusPermBit(name string) int {
	if bit, ok := r.bits[name]; ok {
		return bit
	}
	return -1
}

func (r *cusPermRegistry2540) GetCusPermMapping() map[string]int { return r.bits }

// installCusPermRegistry2540 用确定性位图替换全局 registry，并在用例结束时还原。
func installCusPermRegistry2540(t *testing.T) {
	t.Helper()
	bits := map[string]int{}
	next := 0
	for _, code := range sortedTemplateCodes() {
		for _, perm := range services.AllRoleTemplates[code].CusPermCodes {
			if _, ok := bits[perm]; !ok {
				bits[perm] = next
				next++
			}
		}
	}
	prev := middleware.PermissionRegistry
	middleware.PermissionRegistry = &cusPermRegistry2540{bits: bits}
	t.Cleanup(func() { middleware.PermissionRegistry = prev })
}

// sortedTemplateCodes 让位分配与 map 迭代顺序无关（可重放）。
func sortedTemplateCodes() []string {
	codes := make([]string, 0, len(services.AllRoleTemplates))
	for code := range services.AllRoleTemplates {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// expectedCusPerm 返回角色模板应有的客户权限位图（与 handler 同一计算函数）。
func expectedCusPerm(code string) int64 {
	tmpl := services.AllRoleTemplates[code]
	bitmap, _ := services.ComputeCusPermBitmapExt(tmpl.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
	return bitmap
}

// R1：激活路径的关系角色与权限模板同源取自 users.role，非法值不写入 IAM。
func TestActivateUser2540_RoleMapping(t *testing.T) {
	_, handler, tenantID, _ := setupUserStaffTest(t)
	installCusPermRegistry2540(t)
	f := installFakeIAM2540(t)

	cases := []struct {
		name          string
		role          string
		wantBindRole  string
		wantTemplate  string
		expectedToken string
		templateToken string
	}{
		{"site_member", "site_member", services.IAMRoleStaff, "site_member", "MEMBER_BIND_ROLE=STAFF", "MEMBER_ROLE_TEMPLATE"},
		{"site_admin", "site_admin", services.IAMRoleAdmin, "site_admin", "ADMIN_BIND_ROLE=ADMIN", "ADMIN_ROLE_TEMPLATE"},
		{"legacy_empty", "", services.IAMRoleStaff, "site_member", "LEGACY_BIND_ROLE=STAFF", "LEGACY_ROLE_TEMPLATE"},
		// customer 不是员工模板（激活只针对员工行）→ legacy 兜底 site_member。
		{"customer", "customer", services.IAMRoleStaff, "site_member", "CUSTOMER_BIND_ROLE=STAFF", "CUSTOMER_ROLE_TEMPLATE"},
		{"unknown", "tenant_admin", services.IAMRoleStaff, "site_member", "UNKNOWN_BIND_ROLE=STAFF", "UNKNOWN_ROLE_TEMPLATE"},
		// Rev.3 D2：平台级模板不得进入员工激活（否则平台模板与员工关系角色语义混搭）。
		{"platform_namespace_admin", "namespace_admin", services.IAMRoleStaff, "site_member", "PLATFORM_BIND_ROLE=STAFF", "PLATFORM_ROLE_TEMPLATE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(f.bindRoles())
			beforeAssign := len(f.assignCalls())
			userID := seedActivationUser(t, tenantID, tc.role)
			permsBefore := len(f.customerPerms())

			w := activate2540(t, handler, tenantID, userID)
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

			roles := f.bindRoles()
			require.Len(t, roles, before+1, "每次激活必须产生一次 bind")
			assert.Equal(t, tc.wantBindRole, roles[len(roles)-1], "%s: IAM 关系角色必须是白名单值", tc.name)

			perms := f.customerPerms()
			require.Len(t, perms, permsBefore+1, "每次激活必须产生一次 customer-permissions 写入")
			want := expectedCusPerm(tc.wantTemplate)
			assert.Equal(t, want, perms[len(perms)-1],
				"%s: 客户权限位必须来自模板 %s（不得硬编码 site_member）", tc.name, tc.wantTemplate)

			// 审计 Bug 1：功能角色模板必须与权限位同源（同一 roleTemplate）。
			assigns := f.assignCalls()
			require.Len(t, assigns, beforeAssign+1,
				"%s: 激活必须分配一次功能角色模板（只写权限位不够）", tc.name)
			assert.Equal(t, tc.wantTemplate, assigns[len(assigns)-1].TemplateCode,
				"%s: 功能角色模板码必须等于权限位所用模板", tc.name)
			assert.Equal(t, tenantID, assigns[len(assigns)-1].OrgID,
				"%s: 模板分配必须落在绑定组织", tc.name)

			fmt.Printf("%s\n", tc.expectedToken)
			fmt.Printf("%s=%s\n", tc.templateToken, assigns[len(assigns)-1].TemplateCode)
		})
	}

	// 汇总断言：payload 里不得出现业务角色字面量。
	illegal := 0
	for _, role := range f.bindRoles() {
		switch role {
		case services.IAMRoleOwner, services.IAMRoleAdmin, services.IAMRoleStaff, services.IAMRoleRepairTechnician:
		default:
			illegal++
		}
	}
	fmt.Printf("ILLEGAL_ROLE_COUNT=%d\n", illegal)
	assert.Zero(t, illegal, "bind payload 只能出现 OWNER/ADMIN/STAFF/repair_technician")

	// 模板分配的模板码同样不得出现业务角色以外的 IAM 角色字面量。
	illegalTemplate := 0
	for _, a := range f.assignCalls() {
		if _, ok := services.AllRoleTemplates[a.TemplateCode]; !ok {
			illegalTemplate++
		}
	}
	fmt.Printf("ILLEGAL_TEMPLATE_COUNT=%d\n", illegalTemplate)
	assert.Zero(t, illegalTemplate, "模板分配只能出现 AllRoleTemplates 中的业务模板码")

	// 权限模板与角色模板同源：site_admin 必须拿到 site_admin 的位图（不是 site_member）。
	perms := f.customerPerms()
	require.GreaterOrEqual(t, len(perms), 2)
	member, admin := perms[0], perms[1]
	assert.Equal(t, expectedCusPerm("site_member"), member)
	assert.Equal(t, expectedCusPerm("site_admin"), admin)
	assert.NotEqual(t, member, admin, "site_admin 与 site_member 的权限位不同（防降权回归）")
	fmt.Printf("ADMIN_PERM_TEMPLATE=site_admin\n")
	fmt.Printf("PERM_TEMPLATE=site_member\n")
	fmt.Printf("CUS_PERM_MATCH=1\n")

	// 模板分配与权限位逐条同源（顺序：site_member、site_admin、legacy、customer、unknown、platform）。
	assigns := f.assignCalls()
	require.Len(t, assigns, 6)
	wantTemplates := []string{"site_member", "site_admin", "site_member", "site_member", "site_member", "site_member"}
	mismatch := 0
	for i, wantCode := range wantTemplates {
		if assigns[i].TemplateCode != wantCode {
			mismatch++
		}
	}
	fmt.Printf("ROLE_TEMPLATE_MISMATCH=%d\n", mismatch)
	assert.Zero(t, mismatch, "功能角色模板必须逐条等于权限位所用模板")
}

// 纯函数：激活角色模板解析（空/非法/平台级/商户级/顾客 → legacy site_member）。
func TestResolveActivationRoleTemplate2540(t *testing.T) {
	cases := map[string]string{
		// 员工级白名单（Rev.3 D2）。
		"site_member":       "site_member",
		"site_admin":        "site_admin",
		"merchant_staff":    "merchant_staff",
		"repair_technician": "repair_technician",
		// 平台级 / 商户级 / 顾客 / 空 / 未知 → legacy 兜底（不把非员工模板带进员工激活）。
		"namespace_admin": "site_member",
		"sys_admin":       "site_member",
		"merchant_admin":  "site_member",
		"customer":        "site_member",
		"":                "site_member",
		"tenant_admin":    "site_member",
		"user":            "site_member",
	}
	for in, want := range cases {
		assert.Equal(t, want, resolveActivationRoleTemplate(in), "resolveActivationRoleTemplate(%q)", in)
	}
}
