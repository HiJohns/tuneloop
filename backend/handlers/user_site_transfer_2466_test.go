package handlers

// #2466：编辑员工切换归属网点——site_members 迁移 + IAM 正确重绑 + functional_roles 重建。
// R1–R8 覆盖：同 org 本地迁移（IAM 0 次）、跨 org 角色映射（STAFF/ADMIN）、多网点目标
// 已隶属/未隶属、IAM bind/模板失败无半态、读回一致。

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
)

type iamCall2466 struct {
	Method string
	Path   string
	Body   string
}

type fakeIAM2466 struct {
	mu              sync.Mutex
	calls           []iamCall2466
	failBindOrg     string
	failTemplateOrg string
}

func (f *fakeIAM2466) record(r *http.Request) string {
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, iamCall2466{Method: r.Method, Path: r.URL.Path, Body: body.String()})
	return body.String()
}

func (f *fakeIAM2466) snapshot() []iamCall2466 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]iamCall2466, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeIAM2466) count(kind string) int {
	n := 0
	for _, c := range f.snapshot() {
		switch kind {
		case "bind":
			if strings.HasSuffix(c.Path, "/bind") && strings.Contains(c.Body, `"action":"bind"`) {
				n++
			}
		case "unbind":
			if strings.HasSuffix(c.Path, "/bind") && strings.Contains(c.Body, `"action":"unbind"`) {
				n++
			}
		case "template":
			if strings.HasSuffix(c.Path, "/roles") {
				n++
			}
		}
	}
	return n
}

func (f *fakeIAM2466) bindRoles() []string {
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

func (f *fakeIAM2466) templateRoles() []string {
	out := []string{}
	for _, c := range f.snapshot() {
		if strings.HasSuffix(c.Path, "/roles") {
			var req struct {
				RoleIDs []string `json:"role_ids"`
			}
			_ = json.Unmarshal([]byte(c.Body), &req)
			out = append(out, req.RoleIDs...)
		}
	}
	return out
}

func installFakeIAM2466(t *testing.T) *fakeIAM2466 {
	t.Helper()
	f := &fakeIAM2466{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		body := f.record(r)
		if f.failBindOrg != "" && strings.HasSuffix(r.URL.Path, "/bind") &&
			strings.Contains(body, `"action":"bind"`) && strings.Contains(r.URL.Path, "/organizations/"+f.failBindOrg+"/") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"injected bind failure"}`))
			return
		}
		if f.failTemplateOrg != "" && strings.HasSuffix(r.URL.Path, "/roles") && strings.Contains(body, `"org_id":"`+f.failTemplateOrg+`"`) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"injected template failure"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
	}))
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return f
}

type siteTransferFixture struct {
	userID string
	siteA  string
	siteB  string
	orgA   string
	orgB   string
}

// seedSiteTransferFixture：tenant + 两个网点（A/B 可同 org 或异 org）+ 用户主隶属 A。
func seedSiteTransferFixture(t *testing.T, sameOrg bool, memberRole string, memberRoles []string) siteTransferFixture {
	t.Helper()
	_, _, tenantID, _ := setupUserStaffTest(t)
	db := database.GetDB()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2466"}).Error)
	testfixtures.SeedTenantFK(t, db, tenantID)

	orgA := uuid.New().String()
	orgB := orgA
	if !sameOrg {
		orgB = uuid.New().String()
	}
	siteA := uuid.New().String()
	siteB := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgA, Name: "网点A", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgB, Name: "新网点", Status: "active"}).Error)

	userID := createTestUser(t, db, tenantID)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"org_id": orgA, "role": "site_member"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteA, UserID: userID,
		Role: memberRole, Roles: memberRoles, Status: "active",
	}).Error)
	return siteTransferFixture{userID: userID, siteA: siteA, siteB: siteB, orgA: orgA, orgB: orgB}
}

func putSiteTransfer(t *testing.T, handler *UserStaffHandler, tenantID, userID, targetSite string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/api/users/:id", handler.UpdateUser)
	raw, _ := json.Marshal(map[string]interface{}{"site_id": targetSite})
	req := httptest.NewRequest("PUT", "/api/users/"+userID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// R1：同 org 切网点 → 本地迁移，IAM 0 次。
func TestSiteTransfer_SameOrg_LocalOnly_2466(t *testing.T) {
	f := installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, true, "site_member", []string{"site_member"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	tenantID := fx.orgA
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	tenantID = u.TenantID

	w := putSiteTransfer(t, handler, tenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).First(&row).Error)
	assert.Equal(t, fx.siteB, row.SiteID, "SITE_ID=B")
	assert.Equal(t, 0, f.count("unbind")+f.count("bind")+f.count("template"), "IAM_CALLS=0")
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	assert.Equal(t, fx.orgA, u.OrgID, "ORG_UNCHANGED=1")
	t.Logf("SITE_ID=B ORG_UNCHANGED=1 IAM_CALLS=0")
}

// R2：跨 org，主行 role=site_member → bind STAFF + 模板 site_member + 权限设置。
func TestSiteTransfer_CrossOrg_Staff_2466(t *testing.T) {
	f := installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, false, "site_member", []string{"site_member"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).First(&row).Error)
	assert.Equal(t, fx.siteB, row.SiteID)
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	assert.Equal(t, fx.orgB, u.OrgID, "USERS_ORG=O2")
	require.Equal(t, []string{"STAFF"}, f.bindRoles(), "BIND_ROLE=STAFF")
	assert.Equal(t, 1, f.count("unbind"), "IAM_ORDER=unbind_then_bind")
	assert.Contains(t, f.templateRoles(), "site_member", "FUNC_ROLES_CONTAINS=site_member")
	t.Logf("SITE_ID=B USERS_ORG=%s BIND_ROLE=STAFF FUNC_ROLES_CONTAINS=site_member", u.OrgID)
}

// R3：site_admin 不降权 → bind ADMIN + 模板 site_admin。
func TestSiteTransfer_CrossOrg_Admin_2466(t *testing.T) {
	f := installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, false, "site_admin", []string{"site_admin"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, []string{"ADMIN"}, f.bindRoles(), "BIND_ROLE=ADMIN")
	assert.Contains(t, f.templateRoles(), "site_admin", "FUNC_ROLES_CONTAINS=site_admin")
	t.Logf("BIND_ROLE=ADMIN FUNC_ROLES_CONTAINS=site_admin")
}

// R4：多网点切到已隶属网点 → 删旧主行，无唯一键冲突。
func TestSiteTransfer_TargetAlreadyMember_2466(t *testing.T) {
	installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, true, "site_member", []string{"site_member"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: u.TenantID, SiteID: fx.siteB, UserID: fx.userID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var rows []models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).Find(&rows).Error)
	require.Len(t, rows, 1, "ROWS_AFTER=1")
	assert.Equal(t, fx.siteB, rows[0].SiteID, "TARGET_ROW_KEPT=1")
	t.Logf("ROWS_AFTER=1 TARGET_ROW_KEPT=1 UNIQUE_VIOLATION=0")
}

// R5：多网点切到新网点 → 主行迁移，其他行不变。
func TestSiteTransfer_NewTarget_2466(t *testing.T) {
	installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, true, "site_member", []string{"site_member"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	siteC := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteC, TenantID: u.TenantID, OrgID: fx.orgA, Name: "网点C", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: u.TenantID, SiteID: fx.siteB, UserID: fx.userID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, siteC)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var rows []models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).Order("created_at ASC").Find(&rows).Error)
	require.Len(t, rows, 2, "ROWS_AFTER=2")
	sites := []string{rows[0].SiteID, rows[1].SiteID}
	assert.Contains(t, sites, siteC, "PRIMARY_SITE=C")
	assert.Contains(t, sites, fx.siteB, "OTHER_SITE_UNCHANGED=B")
	t.Logf("ROWS_AFTER=2 PRIMARY_SITE=C OTHER_SITE_UNCHANGED=B")
}

// R6：IAM bind 失败 → 502，本地不变，补偿 rebind 旧组织。
func TestSiteTransfer_IAMBindFail_NoHalfState_2466(t *testing.T) {
	f := installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, false, "site_member", []string{"site_member"})
	f.failBindOrg = fx.orgB
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	var row models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).First(&row).Error)
	assert.Equal(t, fx.siteA, row.SiteID, "SITE_UNCHANGED=1")
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)
	assert.Equal(t, fx.orgA, u.OrgID, "ORG_UNCHANGED=1")
	assert.GreaterOrEqual(t, f.count("bind"), 2, "IAM_COMPENSATED=1")
	t.Logf("STATUS=502 SITE_UNCHANGED=1 ORG_UNCHANGED=1 IAM_COMPENSATED=1")
}

// R7：模板分配失败 → 502 + 文案，unbind 新组织，本地不变。
func TestSiteTransfer_TemplateFail_Explicit_2466(t *testing.T) {
	f := installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, false, "site_member", []string{"site_member"})
	f.failTemplateOrg = fx.orgB
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "切换网点失败", "MSG_CONTAINS=切换网点失败")
	var row models.SiteMember
	require.NoError(t, db.Where("user_id = ?", fx.userID).First(&row).Error)
	assert.Equal(t, fx.siteA, row.SiteID, "SITE_UNCHANGED=1")
	assert.GreaterOrEqual(t, f.count("unbind"), 1, "IAM_UNBIND_NEW=1")
	t.Logf("STATUS=502 MSG_CONTAINS=切换网点失败 IAM_UNBIND_NEW=1 SITE_UNCHANGED=1")
}

// R8：迁移后详情/列表读回新网点。
func TestSiteTransfer_Readback_2466(t *testing.T) {
	installFakeIAM2466(t)
	fx := seedSiteTransferFixture(t, false, "site_member", []string{"site_member"})
	db := database.GetDB()
	handler := &UserStaffHandler{}
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", fx.userID).Error)

	w := putSiteTransfer(t, handler, u.TenantID, fx.userID, fx.siteB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, u.TenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, fx.userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/admin/staff/:id", handler.GetStaffUser)
	r.GET("/staff", handler.ListStaff)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/admin/staff/"+fx.userID, nil))
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var detail struct {
		Data struct {
			SiteID   string `json:"site_id"`
			SiteName string `json:"site_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &detail))
	assert.Equal(t, fx.siteB, detail.Data.SiteID, "DETAIL_SITE_ID=B")
	assert.Equal(t, "新网点", detail.Data.SiteName, "DETAIL_SITE_NAME=新网点")

	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("GET", "/staff?page=1&pageSize=100", nil))
	require.Equal(t, http.StatusOK, w3.Code, w3.Body.String())
	assert.Contains(t, w3.Body.String(), fx.siteB, "LIST_SITE_MATCH=1")
	t.Logf("DETAIL_SITE_ID=B DETAIL_SITE_NAME=新网点 LIST_SITE_MATCH=1")
}
