package handlers

// #2386：全体成员聚合视图（去重/权限降级/跨商户隔离）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type staffFixture2386 struct {
	db       *gorm.DB
	tenantID string
	merchant *models.Merchant
	siteA    *models.Site
	siteB    *models.Site
	users    map[string]*models.User
}

func newStaffFixture2386(t *testing.T) *staffFixture2386 {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)

	f := &staffFixture2386{db: db, tenantID: tenantID, users: map[string]*models.User{}}
	f.merchant = &models.Merchant{ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID, AdminUID: uuid.New().String(), Name: "M2386-" + tenantID[:8], Status: "active"}
	require.NoError(t, db.Create(f.merchant).Error)
	f.siteA = &models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: uuid.New().String(), Name: "SiteA2386"}
	f.siteB = &models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: uuid.New().String(), Name: "SiteB2386"}
	require.NoError(t, db.Create(f.siteA).Error)
	require.NoError(t, db.Create(f.siteB).Error)
	return f
}

func (f *staffFixture2386) user(t *testing.T, key, role string) *models.User {
	t.Helper()
	u := &models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: f.tenantID, OrgID: f.tenantID,
		Username: "u2386_" + key, Name: "成员 " + key, Role: role, Status: "active",
	}
	require.NoError(t, f.db.Create(u).Error)
	f.users[key] = u
	return u
}

func (f *staffFixture2386) direct(t *testing.T, u *models.User, role string, created time.Time) {
	t.Helper()
	require.NoError(t, f.db.Create(&models.MerchantMember{
		ID: uuid.New().String(), TenantID: f.tenantID, MerchantID: f.merchant.ID,
		UserID: u.ID, Role: role, Status: "active", CreatedAt: created, UpdatedAt: created,
	}).Error)
}

func (f *staffFixture2386) siteMember(t *testing.T, site *models.Site, u *models.User, role string, roles []string, created time.Time) {
	t.Helper()
	testfixtures.SeedTenantFK(t, f.db, f.tenantID) // #2443: site_members tenant FK
	require.NoError(t, f.db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: f.tenantID, SiteID: site.ID, UserID: u.ID,
		Role: role, Roles: roles, CreatedAt: created, UpdatedAt: created,
	}).Error)
}

func staffRouter2386(tenantID, orgID, userID, role string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: role}.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewStaffMembersHandler()
	r.GET("/staff/members", h.List)
	return r
}

func getStaffMembers2386(t *testing.T, r *gin.Engine, url string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

// R1：直属 2 + 网点 A 3（u3 兼 B）= 5 行，去重且 sites 正确。
func TestStaffMembersAggregate_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-48 * time.Hour)
	u1 := f.user(t, "direct1", "site_member")
	u2 := f.user(t, "direct2", "site_member")
	u3 := f.user(t, "multi", "site_member")
	u4 := f.user(t, "site4", "site_member")
	u5 := f.user(t, "site5", "repair_technician")
	f.direct(t, u1, "site_member", base)
	f.direct(t, u2, "site_admin", base.Add(time.Hour))
	f.siteMember(t, f.siteA, u3, "site_member", []string{"site_member", "site_admin"}, base.Add(2*time.Hour))
	f.siteMember(t, f.siteB, u3, "site_member", []string{"site_member"}, base.Add(3*time.Hour))
	f.siteMember(t, f.siteA, u4, "site_member", []string{"site_member"}, base)
	f.siteMember(t, f.siteA, u5, "repair_technician", []string{"repair_technician"}, base)

	// 跨商户数据（不同 tenant）不得出现
	otherTenant := uuid.New().String()
	testfixtures.SeedTenantFK(t, f.db, otherTenant)
	other := &models.User{ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: otherTenant, OrgID: otherTenant, Username: "other2386", Name: "外商户", Status: "active"}
	require.NoError(t, f.db.Create(other).Error)
	require.NoError(t, f.db.Create(&models.MerchantMember{
		ID: uuid.New().String(), TenantID: otherTenant, MerchantID: uuid.New().String(), UserID: other.ID, Role: "site_member", Status: "active",
	}).Error)

	r := staffRouter2386(f.tenantID, f.tenantID, u1.ID, "ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?scope=all&page_size=50")
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	list := data["list"].([]interface{})
	assert.EqualValues(t, 5, data["total"], "5 deduped users (cross-tenant excluded)")
	require.Len(t, list, 5)
	byID := map[string]map[string]interface{}{}
	for _, it := range list {
		row := it.(map[string]interface{})
		byID[row["user_id"].(string)] = row
	}
	assert.NotContains(t, byID, other.ID, "cross-merchant rows must be invisible")

	multi := byID[u3.ID]
	require.NotNil(t, multi)
	assert.EqualValues(t, 2, len(multi["sites"].([]interface{})), "u3 belongs to A and B")
	assert.Equal(t, false, multi["is_direct"])
	ids := multi["identities"].([]interface{})
	found := map[string]bool{}
	for _, v := range ids {
		found[v.(string)] = true
	}
	assert.True(t, found["site_admin"], "site roles[] must be merged: %v", ids)
	assert.True(t, found["site_member"])
}

// R2：网点管理员 scope=all 降级为本站点；看不到直属成员。
func TestStaffMembersScope_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-24 * time.Hour)
	admin := f.user(t, "siteadmin", "ADMIN")
	u1 := f.user(t, "direct1", "site_member")
	u3 := f.user(t, "multi", "site_member")
	u4 := f.user(t, "site4", "site_member")
	f.siteMember(t, f.siteA, admin, "site_admin", []string{"site_admin"}, base)
	f.direct(t, u1, "site_member", base)
	f.siteMember(t, f.siteA, u3, "site_member", []string{"site_member"}, base)
	f.siteMember(t, f.siteA, u4, "site_member", []string{"site_member"}, base)

	r := staffRouter2386(f.tenantID, f.siteA.OrgID, admin.ID, "ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?scope=all&page_size=50")
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	list := data["list"].([]interface{})
	require.Len(t, list, 3, "site admin must be degraded to their own site")
	for _, it := range list {
		row := it.(map[string]interface{})
		assert.NotEqual(t, u1.ID, row["user_id"], "direct member outside the site must not appear")
	}
}

// R3：系统管理员只读可看；无写路由（POST 404/405）。
func TestStaffMembersPlatform_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-12 * time.Hour)
	u1 := f.user(t, "direct1", "site_member")
	f.direct(t, u1, "site_member", base)

	r := staffRouter2386(f.tenantID, f.tenantID, u1.ID, "NAMESPACE_ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?scope=all&page_size=50")
	require.Equal(t, http.StatusOK, code)
	data := resp["data"].(map[string]interface{})
	assert.GreaterOrEqual(t, data["total"].(float64), float64(1))

	// 生产路由只注册 GET；本测试路由同样只有 GET → POST 必须 404/405（无写入口）。
	req := httptest.NewRequest(http.MethodPost, "/staff/members", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, w.Code)
}

// R5：不传 scope → 默认 direct（Issue 契约 F1）。
func TestStaffMembersDefaultScope_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-24 * time.Hour)
	u1 := f.user(t, "d1", "site_member")
	u2 := f.user(t, "s2", "site_member")
	f.direct(t, u1, "site_member", base)
	f.siteMember(t, f.siteA, u2, "site_member", []string{"site_member"}, base)

	r := staffRouter2386(f.tenantID, f.tenantID, u1.ID, "ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?page_size=50")
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	list := data["list"].([]interface{})
	require.Len(t, list, 1, "default scope must be direct")
	assert.Equal(t, u1.ID, list[0].(map[string]interface{})["user_id"])
}

// R6：identity 过滤必须识别 site_members.roles[] 多角色（F3）。
func TestStaffMembersIdentityRoles_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-24 * time.Hour)
	u := f.user(t, "dual", "site_member")
	f.siteMember(t, f.siteA, u, "site_member", []string{"site_member", "site_admin"}, base)

	r := staffRouter2386(f.tenantID, f.tenantID, u.ID, "ADMIN")
	for _, identity := range []string{"admin", "staff"} {
		code, resp := getStaffMembers2386(t, r, "/staff/members?scope=all&identity="+identity+"&page_size=50")
		require.Equal(t, http.StatusOK, code)
		list := resp["data"].(map[string]interface{})["list"].([]interface{})
		found := false
		for _, it := range list {
			if it.(map[string]interface{})["user_id"] == u.ID {
				found = true
			}
		}
		assert.True(t, found, "identity=%s must match roles[] jsonb", identity)
	}
}

// R7：响应含 merchant_role（直属角色；非直属为空），F2。
func TestStaffMembersMerchantRole_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-24 * time.Hour)
	u1 := f.user(t, "d1", "site_member")
	u2 := f.user(t, "s2", "site_member")
	f.direct(t, u1, "site_admin", base)
	f.siteMember(t, f.siteA, u2, "site_member", []string{"site_member"}, base)

	r := staffRouter2386(f.tenantID, f.tenantID, u1.ID, "ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?scope=all&page_size=50")
	require.Equal(t, http.StatusOK, code)
	byID := map[string]map[string]interface{}{}
	for _, it := range resp["data"].(map[string]interface{})["list"].([]interface{}) {
		row := it.(map[string]interface{})
		byID[row["user_id"].(string)] = row
	}
	assert.Equal(t, "site_admin", byID[u1.ID]["merchant_role"])
	assert.Nil(t, byID[u2.ID]["merchant_role"])
}

// R8：A 网点成员 + B 直属 → A 的 direct 不含该人；all 含且 is_direct=false（F4）。
func TestStaffMembersDirectIsolation_2386(t *testing.T) {
	f := newStaffFixture2386(t)
	base := time.Now().Add(-24 * time.Hour)
	u := f.user(t, "mixed", "site_member")
	f.siteMember(t, f.siteA, u, "site_member", []string{"site_member"}, base)

	tenantB := uuid.New().String()
	testfixtures.SeedTenantFK(t, f.db, tenantB)
	merchantB := &models.Merchant{ID: uuid.New().String(), TenantID: tenantB, OrgID: tenantB, AdminUID: uuid.New().String(), Name: "MB2386", Status: "active"}
	require.NoError(t, f.db.Create(merchantB).Error)
	require.NoError(t, f.db.Create(&models.MerchantMember{
		ID: uuid.New().String(), TenantID: tenantB, MerchantID: merchantB.ID, UserID: u.ID, Role: "merchant_staff", Status: "active",
	}).Error)

	r := staffRouter2386(f.tenantID, f.tenantID, u.ID, "ADMIN")
	code, resp := getStaffMembers2386(t, r, "/staff/members?scope=direct&page_size=50")
	require.Equal(t, http.StatusOK, code)
	list := resp["data"].(map[string]interface{})["list"].([]interface{})
	for _, it := range list {
		assert.NotEqual(t, u.ID, it.(map[string]interface{})["user_id"], "B-direct user must not appear in A direct")
	}

	code, resp = getStaffMembers2386(t, r, "/staff/members?scope=all&page_size=50")
	require.Equal(t, http.StatusOK, code)
	found := false
	for _, it := range resp["data"].(map[string]interface{})["list"].([]interface{}) {
		row := it.(map[string]interface{})
		if row["user_id"] == u.ID {
			found = true
			assert.Equal(t, false, row["is_direct"])
			assert.Nil(t, row["merchant_role"])
		}
	}
	assert.True(t, found, "A site member must appear in all")
}
