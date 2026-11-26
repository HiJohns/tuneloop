package handlers

// #2547 D3：中转成员识别改为**站点维度**（`sites.type='transit'` + `sites.tenant_id = 平台租户`），
// 不再用 `sites.org_id = 平台组织`。
//
// 背景：中转网点建站后会持有自己的 branch 子组织（`sites.org_id = <中转子组织>`），
// 此时 `sites.org_id ≠ 平台组织(oid)`；沿用旧判据会让既有中转成员从系统视图消失。
// 本用例以「org_id ≠ tenant_id」的夹具锁定新口径，并保留两条负例
// （同租户非中转站成员、他租户中转站成员都不得进入平台系统视图）。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

type personnelTransit2547Fixture struct {
	tid, rootOrg, transitOrg, otherTid, otherTransitOrg string
	transitPerson                                       string
	storePerson                                         string
	foreignTransitPerson                                string
	platformStaff                                       string
}

func setupPersonnelTransit2547(t *testing.T) (*gin.Engine, personnelTransit2547Fixture) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	f := personnelTransit2547Fixture{
		tid:                  uuid.New().String(),
		rootOrg:              uuid.New().String(),
		transitOrg:           uuid.New().String(),
		otherTid:             uuid.New().String(),
		otherTransitOrg:      uuid.New().String(),
		storePerson:          uuid.New().String(),
		foreignTransitPerson: uuid.New().String(),
		platformStaff:        uuid.New().String(),
	}
	require.NotEqual(t, f.tid, f.transitOrg, "中转站 org_id 必须 ≠ 平台租户（否则旧判据不可区分）")
	testfixtures.SeedTenantFK(t, db, f.tid, f.rootOrg, f.otherTid)

	mkUser := func(id, tid, oid, name, role string) {
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: id, TenantID: tid, OrgID: oid,
			Username: "u-" + id[:8], Name: name, Role: role, Status: "active",
		}).Error)
	}

	// 平台员工（根组织 users）
	mkUser(f.platformStaff, f.tid, f.rootOrg, "平台员工2547", "staff")

	// 中转网点：tenant=平台租户，org=该站自己的 branch 子组织（新模型）
	transitSite := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: transitSite, TenantID: f.tid, OrgID: f.transitOrg,
		Name: "北京中转站2547", Type: "transit", Status: "active",
	}).Error)
	transitMember := uuid.New().String()
	mkUser(transitMember, f.tid, f.transitOrg, "中转成员2547", "member")
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.tid, SiteID: transitSite, UserID: transitMember, Role: "site_member", Status: "active",
	}).Error)
	f.transitPerson = transitMember

	// 负例 1：同租户普通网点（type=store）成员 —— 不得进入平台系统视图
	storeSite := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: storeSite, TenantID: f.tid, OrgID: uuid.New().String(),
		Name: "加盟网点2547", Type: "store", Status: "active",
	}).Error)
	mkUser(f.storePerson, f.tid, f.tid, "加盟网点成员2547", "member")
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.tid, SiteID: storeSite, UserID: f.storePerson, Role: "site_member", Status: "active",
	}).Error)

	// 负例 2：他租户的中转网点成员 —— 不得进入本平台系统视图
	otherTransitSite := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: otherTransitSite, TenantID: f.otherTid, OrgID: f.otherTransitOrg,
		Name: "他租户中转站2547", Type: "transit", Status: "active",
	}).Error)
	mkUser(f.foreignTransitPerson, f.otherTid, f.otherTransitOrg, "他租户中转成员2547", "member")
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.otherTid, SiteID: otherTransitSite, UserID: f.foreignTransitPerson,
		Role: "site_member", Status: "active",
	}).Error)

	h := NewPersonnelHandler()
	staffHandler := &UserStaffHandler{}
	r := gin.New()
	systemActor := func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: f.tid,     // 平台租户
			OrgID:    f.rootOrg, // 命名空间主组织（JWT oid）
			UserID:   f.platformStaff,
			Role:     "NAMESPACE_ADMIN",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}
	r.GET("/admin/personnel", systemActor, h.List)
	r.GET("/admin/staff/:id", systemActor, staffHandler.GetStaffUser)
	return r, f
}

func TestPersonnelSystemViewTransitBySiteOrg2547(t *testing.T) {
	r, f := setupPersonnelTransit2547(t)

	code, rows := personnelList2065Q(t, r, "actor=system")
	require.Equal(t, http.StatusOK, code)
	byName := names2065(rows)

	_, transitVisible := byName["中转成员2547"]
	_, storeVisible := byName["加盟网点成员2547"]
	_, foreignVisible := byName["他租户中转成员2547"]
	platformStaffVisible := false
	if _, ok := byName["平台员工2547"]; ok {
		platformStaffVisible = true
	}

	assert.True(t, transitVisible, "中转成员必须出现在平台系统视图（org_id ≠ 平台组织仍按站点维度命中）")
	assert.False(t, storeVisible, "同租户非中转站成员不得进入平台系统视图")
	assert.False(t, foreignVisible, "他租户中转站成员不得进入本平台系统视图")
	assert.True(t, platformStaffVisible, "平台员工仍须可见")

	// by-id（#2526 同源判定）：中转成员详情可见
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/staff/"+f.transitPerson, nil))
	assert.Equal(t, http.StatusOK, w.Code, "中转成员详情必须可见: %s", w.Body.String())

	fmt.Printf("SYSTEM_TRANSIT_VISIBLE=%d\n", flag2547(transitVisible))
	fmt.Printf("SYSTEM_NON_TRANSIT_PLATFORM_SITE_HIDDEN=%d\n", flag2547(!storeVisible))
	fmt.Printf("SYSTEM_FOREIGN_TENANT_TRANSIT_HIDDEN=%d\n", flag2547(!foreignVisible))
	fmt.Printf("STAFF_DETAIL_TRANSIT_VISIBLE=%d\n", flag2547(w.Code == http.StatusOK))
}

func flag2547(cond bool) int {
	if cond {
		return 1
	}
	return 0
}
