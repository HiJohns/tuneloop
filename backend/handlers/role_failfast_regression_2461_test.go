package handlers

// #2461 R9：既有 fail-fast 调用点回归（handler 不改，只验证未被降级成成功）。
//   - 中转网点成员改角色（transit_site_admin.go）→ IAM 失败 502
//   - 权限管理设置角色（permission_manage.go）→ IAM 失败 500
//   - 商户直属成员改角色（merchant_member.go）→ IAM 失败 500

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
	"tuneloop-backend/testutil"
)

func ctxRouter2461(t *testing.T, tenantID, namespaceID string, register func(r *gin.Engine)) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, namespaceID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	register(r)
	return r
}

func doJSON2461(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2461")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 中转网点成员改角色：IAM 500 → 502（保持既有 fail-fast，不得降级为 200）。
func TestTransitSiteMemberRoleFailFast_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID := uuid.NewString()
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "T2461", Type: transitSiteType, Status: "active",
	}).Error)
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: orgID,
		Username: "u-" + uuid.NewString()[:8], Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: tenantID, SiteID: siteID, UserID: userID, Role: "site_member",
		Roles: []string{"site_member"},
	}).Error)

	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	r := ctxRouter2461(t, tenantID, "ns-2461", func(r *gin.Engine) {
		r.PUT("/transit-sites/:id/members/:user_id", UpdateTransitSiteMemberRole)
	})
	w := doJSON2461(t, r, http.MethodPut, "/transit-sites/"+siteID+"/members/"+userID, `{"role":"site_member"}`)
	assert.Equal(t, http.StatusBadGateway, w.Code, "TRANSIT=502：%s", w.Body.String())
	t.Logf("TRANSIT=%d", w.Code)
}

// 权限管理设置角色：IAM 500 → 500（保持既有 fail-fast）。
func TestPermissionManageSetUserRoleFailFast_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID := uuid.NewString()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: orgID,
		Username: "u-" + uuid.NewString()[:8], Role: "site_member", Status: "active",
	}).Error)

	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	h := NewPermissionManageHandler(db, services.NewIAMClient(), nil)
	r := ctxRouter2461(t, tenantID, "ns-2461", func(r *gin.Engine) {
		r.PUT("/admin/users/:id/roles", h.SetUserRole)
	})
	w := doJSON2461(t, r, http.MethodPut, "/admin/users/"+userID+"/roles", `{"role_code":"site_member"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code, "PERMISSION=500：%s", w.Body.String())
	t.Logf("PERMISSION=%d", w.Code)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, 1, mock.roleCalls, "IAM role 调用发生一次")
	assert.Empty(t, mock.lastRoleRawQ, "不得再使用 ?role= query")
}

// 商户直属成员改角色：IAM 500 → 500（#2250 既有 fail-fast 保持不变）。
func TestMerchantMemberRoleFailFast_2461(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID := uuid.NewString()

	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: orgID, Name: "M2461R9", Status: "active",
		AdminUID: uuid.NewString(),
	}).Error)
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.NewString(), TenantID: tenantID, OrgID: orgID,
		Username: "u-" + uuid.NewString()[:8], Role: "merchant_staff", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: tenantID, MerchantID: merchantID, UserID: userID, Role: "merchant_staff",
	}).Error)

	mock := &roleIAMMock2461{roleStatus: http.StatusInternalServerError}
	setupRoleIAMMock2461(t, mock)

	h := NewMerchantMemberHandler()
	r := ctxRouter2461(t, tenantID, "ns-2461", func(r *gin.Engine) {
		r.PUT("/admin/merchants/:id/members/:uid", h.UpdateMemberRole)
	})
	w := doJSON2461(t, r, http.MethodPut, "/admin/merchants/"+merchantID+"/members/"+userID, `{"role":"merchant_staff"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code, "MERCHANT_MEMBER=500：%s", w.Body.String())
	t.Logf("MERCHANT_MEMBER=%d", w.Code)

	var mm models.MerchantMember
	require.NoError(t, database.GetDB().Where("merchant_id = ? AND user_id = ?", merchantID, userID).First(&mm).Error)
	assert.Equal(t, "merchant_staff", mm.Role, "IAM 失败时本地角色不得改写")
}
