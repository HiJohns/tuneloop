package handlers

// #2552 R2：中转成员识别改为「站点维度」（`sites.type='transit'` 且
// `sites.tenant_id = 平台租户`），不再用 `sites.org_id`。
//
// 缺陷（#2547 审计语义挑战 2）：旧判据是
// `s.type='transit' AND (s.tenant_id = rootOrgID OR s.org_id = rootOrgID)`。
// 中转网点一旦持有自己的 `branch` 子组织（`sites.org_id`，与平台租户不同域），
// 若判据仍按 org 比较，既有成员会从系统视图/详情里**消失**；
// 而放宽判据（只留 `type='transit'`）又会让同租户普通网点成员越权可见。
//
// Token lines are printed with fmt.Printf so scripts/verify-2552.sh can grep them.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

type personnelTransitFixture2552 struct {
	platformTenant string
	otherTenant    string
	rootOrgID      string
	transitSite    models.Site
	storeSite      models.Site
	otherTransit   models.Site

	transitMemberID string
	storeMemberID   string
	otherMemberID   string
}

// setupPersonnelTransit2552 构造「中转成员 M + 同租户 store 成员 S + 他租户 transit 成员 X」。
//
// collapseRootOrg=true 时令「平台租户 == 命名空间根组织」，用于验证
// `platformTenantIDFor` 的 oid 回退分支（系统管理员上下文可能只有 oid）。
// 默认（false）两者**不同**——这才是生产形态，也是旧判据失效的前提：
// 旧代码用 `(s.tenant_id = oid OR s.org_id = oid)`，中转站 tenant 是平台租户（≠ oid），
// 于是站点一旦持有自己的 branch 子组织，成员就从系统视图消失（B6）。
func setupPersonnelTransit2552(t *testing.T, collapseRootOrg bool) personnelTransitFixture2552 {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	platform := uuid.New().String()
	rootOrg := uuid.New().String()
	if collapseRootOrg {
		rootOrg = platform
	}
	f := personnelTransitFixture2552{
		platformTenant:  platform,
		otherTenant:     uuid.New().String(),
		rootOrgID:       rootOrg,
		transitMemberID: uuid.New().String(),
		storeMemberID:   uuid.New().String(),
		otherMemberID:   uuid.New().String(),
	}
	testfixtures.SeedTenantFK(t, db, f.platformTenant, f.otherTenant)

	// 中转站：tenant = 平台租户，org = 自己的 branch 子组织（两者不同域）。
	f.transitSite = models.Site{
		ID:       uuid.New().String(),
		TenantID: f.platformTenant,
		OrgID:    uuid.New().String(),
		Name:     "中转站2552",
		Type:     "transit",
		Status:   "active",
	}
	// 同租户普通网点（判据必须排除）。
	f.storeSite = models.Site{
		ID:       uuid.New().String(),
		TenantID: f.platformTenant,
		OrgID:    uuid.New().String(),
		Name:     "直营店2552",
		Type:     "store",
		Status:   "active",
	}
	// 他租户的中转站（判据必须排除）。
	f.otherTransit = models.Site{
		ID:       uuid.New().String(),
		TenantID: f.otherTenant,
		OrgID:    uuid.New().String(),
		Name:     "他租户中转站2552",
		Type:     "transit",
		Status:   "active",
	}
	for _, site := range []models.Site{f.transitSite, f.storeSite, f.otherTransit} {
		require.NoError(t, db.Create(&site).Error)
	}

	seed := func(userID, tenantID, orgID, name, siteID, role string) {
		require.NoError(t, db.Create(&models.User{
			ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
			Name: name, Email: name + "@example.com", Role: role, Status: "active",
		}).Error)
		require.NoError(t, db.Create(&models.SiteMember{
			ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userID,
			Role: "site_member", Roles: []string{"site_member"}, Status: "active",
		}).Error)
	}
	seed(f.transitMemberID, f.platformTenant, f.transitSite.OrgID, "中转成员2552", f.transitSite.ID, "site_member")
	seed(f.storeMemberID, f.platformTenant, f.storeSite.OrgID, "门店成员2552", f.storeSite.ID, "site_member")
	seed(f.otherMemberID, f.otherTenant, f.otherTransit.OrgID, "他租户成员2552", f.otherTransit.ID, "site_member")

	return f
}

// systemAdminRouter2552 以系统管理员（NAMESPACE_ADMIN）身份注入上下文。
// tenantClaim 为空时走 `platformTenantIDFor` 的 oid 回退分支（#2552 D2）。
func systemAdminRouter2552(f personnelTransitFixture2552, tenantClaim string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantClaim,
			OrgID:    f.rootOrgID,
			UserID:   uuid.New().String(),
			Role:     "NAMESPACE_ADMIN",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/api/admin/personnel", (&PersonnelHandler{}).List)
	r.GET("/api/admin/staff/:id", (&UserStaffHandler{}).GetStaffUser)
	return r
}

func listPersonnelIDs2552(t *testing.T, r *gin.Engine) []string {
	t.Helper()
	repr := httptest.NewRequest(http.MethodGet, "/api/admin/personnel?page_size=100", nil)
	wrec := httptest.NewRecorder()
	r.ServeHTTP(wrec, repr)
	require.Equal(t, http.StatusOK, wrec.Code, "personnel list: %s", wrec.Body.String())

	var resp struct {
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wrec.Body.Bytes(), &resp))
	ids := make([]string, 0, len(resp.Data.List))
	for _, row := range resp.Data.List {
		ids = append(ids, row.ID)
	}
	return ids
}

func getStaff2552(t *testing.T, r *gin.Engine, userID string) int {
	t.Helper()
	greq := httptest.NewRequest(http.MethodGet, "/api/admin/staff/"+userID, bytes.NewReader(nil))
	gw := httptest.NewRecorder()
	r.ServeHTTP(gw, greq)
	return gw.Code
}

// R2：系统视图/by-id 能看到中转成员；同租户 store 成员与他租户 transit 成员都不可见。
func TestPersonnelTransitPredicate2552(t *testing.T) {
	f := setupPersonnelTransit2552(t, false)
	// 生产形态：平台租户(tid) 与命名空间根组织(oid) 不同值。
	// 这是旧判据失效的前提（旧代码用 oid 比较 sites.tenant_id/org_id）。
	r := systemAdminRouter2552(f, f.platformTenant)

	ids := listPersonnelIDs2552(t, r)
	contains := func(id string) int {
		for _, got := range ids {
			if got == id {
				return 1
			}
		}
		return 0
	}
	systemViewTransit := contains(f.transitMemberID)
	negStore := contains(f.storeMemberID)
	negOtherTenant := contains(f.otherMemberID)

	byIDTransit := getStaff2552(t, r, f.transitMemberID)
	byIDStore := getStaff2552(t, r, f.storeMemberID)
	byIDOther := getStaff2552(t, r, f.otherMemberID)

	fmt.Printf("SYSTEM_VIEW_TRANSIT=%d\n", systemViewTransit)
	fmt.Printf("BYID_TRANSIT=%d\n", byIDTransit)
	fmt.Printf("NEG_STORE_SAME_TENANT=%d\n", negStore)
	fmt.Printf("NEG_OTHER_TENANT_TRANSIT=%d\n", negOtherTenant)
	fmt.Printf("BYID_STORE_SAME_TENANT=%d\n", byIDStore)
	fmt.Printf("BYID_OTHER_TENANT_TRANSIT=%d\n", byIDOther)

	require.Equal(t, 1, systemViewTransit, "中转网点成员必须出现在系统视图（判据换维度后不得消失）")
	require.Equal(t, http.StatusOK, byIDTransit, "中转成员详情必须可见")
	require.Zero(t, negStore, "同租户普通网点成员不得进入系统视图")
	require.Zero(t, negOtherTenant, "他租户中转成员不得进入系统视图")
	// 注：带 tid claim 的系统管理员下，同租户 store 成员的 **by-id** 会经
	// #2526 的「带租户 claim 旧口径兜底」（`u.tenant_id == tid`）放行——那是
	// #2526 的既有决策，不在 #2552 范围，故此处不断言；by-id 负例在
	// TestPersonnelTransitPredicate2552_WithTenantClaim（tid 为空）中断言。
	require.Equal(t, http.StatusNotFound, byIDOther, "他租户中转成员详情不得打开")
}

// R2（续）：带平台租户 claim 的系统管理员下，系统视图口径必须一致
// （中转成员在、同租户 store 与他租户 transit 成员都不在）。
//
// 注：此形态下同租户 store 成员的 **by-id** 会经 #2526 的「带租户 claim 的旧口径
// 兜底」放行（`u.tenant_id == tid`），那是 #2526 的既有决策，不在 #2552 范围；
// 本用例只锁定系统视图的判据维度。
func TestPersonnelTransitPredicate2552_WithTenantClaim(t *testing.T) {
	// 平台租户 == 根组织 + tid 为空 → 走 `platformTenantIDFor` 的 oid 回退分支；
	// 此时没有 #2526 的「带租户 claim 旧口径兜底」，by-id 负例才都是 404。
	f := setupPersonnelTransit2552(t, true)
	r := systemAdminRouter2552(f, "")

	ids := listPersonnelIDs2552(t, r)
	contains := func(id string) int {
		for _, got := range ids {
			if got == id {
				return 1
			}
		}
		return 0
	}
	systemViewTransit := contains(f.transitMemberID)
	negStore := contains(f.storeMemberID)
	negOther := contains(f.otherMemberID)

	fmt.Printf("CLAIM_SYSTEM_VIEW_TRANSIT=%d\n", systemViewTransit)
	fmt.Printf("CLAIM_NEG_STORE_SAME_TENANT=%d\n", negStore)
	fmt.Printf("CLAIM_NEG_OTHER_TENANT_TRANSIT=%d\n", negOther)

	require.Equal(t, 1, systemViewTransit, "带 tid claim 时中转成员仍必须可见")
	require.Zero(t, negStore, "同租户普通网点成员不得进入系统视图")
	require.Zero(t, negOther, "他租户中转成员不得进入系统视图")
	require.Equal(t, http.StatusOK, getStaff2552(t, r, f.transitMemberID))
	require.Equal(t, http.StatusNotFound, getStaff2552(t, r, f.storeMemberID),
		"同租户普通网点成员详情不得打开（tid 为空时无旧口径兜底）")
	require.Equal(t, http.StatusNotFound, getStaff2552(t, r, f.otherMemberID))
}

// R2（防回归）：`hasTransitSiteMembership` 关闭自动租户作用域，
// 只有显式的 `sites.tenant_id = 平台租户` 才拦得住他租户中转成员。
func TestPersonnelTransitPredicate2552_NoOrgRule(t *testing.T) {
	f := setupPersonnelTransit2552(t, false)
	db := database.GetDB()
	ctx := testutil.TestActor{
		TenantID: f.platformTenant, OrgID: f.rootOrgID, Role: "NAMESPACE_ADMIN",
	}.InjectContext(context.Background())

	hit, err := hasTransitSiteMembership(db, ctx, f.otherMemberID, f.platformTenant)
	require.NoError(t, err)
	missOther, err := hasTransitSiteMembership(db, ctx, f.storeMemberID, f.platformTenant)
	require.NoError(t, err)
	hitTransit, err := hasTransitSiteMembership(db, ctx, f.transitMemberID, f.platformTenant)
	require.NoError(t, err)

	fmt.Printf("OTHER_TENANT_TRANSIT_MEMBERSHIP=%d\n", flag2526(hit))
	fmt.Printf("STORE_MEMBER_TRANSIT_MEMBERSHIP=%d\n", flag2526(missOther))
	fmt.Printf("TRANSIT_MEMBER_TRANSIT_MEMBERSHIP=%d\n", flag2526(hitTransit))

	require.False(t, hit, "他租户中转成员不得命中平台租户的中转判据")
	require.False(t, missOther, "同租户普通网点成员不得命中中转判据")
	require.True(t, hitTransit, "中转成员必须命中站点维度判据")
}
