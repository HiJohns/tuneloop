package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2065 人员管理三视图：商户（全员含师傅）/ 网点（本网点）/ 系统（平台员工+中转网点成员）

type personnel2065Fixture struct {
	tid, oid, otherTid string
	rootOrg            string
	siteA, siteB       string
	transitSite        string
	merchantAdmin      string
	siteAdminAcct      string
	siteMemberA        string
	siteMemberB        string
	technician         string
	platformStaff      string
	transitMember      string
	customerOnly       string
	otherTenantUser    string
	zeroTenantMember   string // #2067: users.tenant_id 全零但 membership 在商户内（林维训形态）
	directMember       string // #2067: 商户直属员工（merchant_members）
}

func setupPersonnel2065(t *testing.T) (*gin.Engine, personnel2065Fixture) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	f := personnel2065Fixture{
		tid:              uuid.New().String(),
		oid:              uuid.New().String(),
		otherTid:         uuid.New().String(),
		rootOrg:          uuid.New().String(),
		siteA:            uuid.New().String(),
		siteB:            uuid.New().String(),
		transitSite:      uuid.New().String(),
		merchantAdmin:    uuid.New().String(),
		siteAdminAcct:    uuid.New().String(),
		siteMemberA:      uuid.New().String(),
		siteMemberB:      uuid.New().String(),
		technician:       uuid.New().String(),
		platformStaff:    uuid.New().String(),
		transitMember:    uuid.New().String(),
		customerOnly:     uuid.New().String(),
		otherTenantUser:  uuid.New().String(),
		zeroTenantMember: uuid.New().String(),
		directMember:     uuid.New().String(),
	}

	mkUser := func(id, tid, oid, name, role string) {
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: id, TenantID: tid, OrgID: oid, Username: "u-" + id[:8], Name: name, Role: role, Status: "active",
		}).Error)
	}
	mkUser(f.merchantAdmin, f.tid, f.tid, "商户管理员", "admin")
	mkUser(f.siteAdminAcct, f.tid, f.oid, "网点管理员", "admin")
	mkUser(f.siteMemberA, f.tid, f.oid, "网点A成员", "member")
	mkUser(f.siteMemberB, f.tid, f.siteB, "网点B成员", "member")
	mkUser(f.technician, f.tid, f.tid, "维修张师傅", "member")
	mkUser(f.platformStaff, f.rootOrg, f.rootOrg, "平台员工", "staff")
	mkUser(f.transitMember, f.tid, f.transitSite, "中转成员", "member")
	mkUser(f.customerOnly, f.tid, f.tid, "纯顾客", "USER")
	mkUser(f.otherTenantUser, f.otherTid, f.otherTid, "他商户成员", "member")
	// #2067: 本地 users 行租户号不可靠（全零）但 site_members 归属本商户
	mkUser(f.zeroTenantMember, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000", "零租户成员", "member")
	mkUser(f.directMember, f.tid, f.tid, "直属员工", "member")

	// 网点（含一个中转网点，归属根组织）
	require.NoError(t, db.Create(&models.Site{ID: f.siteA, TenantID: f.tid, OrgID: f.tid, Name: "网点A", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.siteB, TenantID: f.tid, OrgID: f.tid, Name: "网点B", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.transitSite, TenantID: f.rootOrg, OrgID: f.rootOrg, Name: "中转网点", Type: "transit"}).Error)

	// 成员关系
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.tid, SiteID: f.siteA, UserID: f.siteMemberA, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.tid, SiteID: f.siteB, UserID: f.siteMemberB, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
	// #2067: 零租户用户的网点成员身份（归属由 sites.tenant_id 决定）
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.tid, SiteID: f.siteA, UserID: f.zeroTenantMember, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
	// #2067: 商户直属员工
	require.NoError(t, db.Create(&models.MerchantMember{TenantID: f.tid, MerchantID: uuid.New().String(), UserID: f.directMember, Role: "site_member", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.rootOrg, SiteID: f.transitSite, UserID: f.transitMember, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.otherTid, SiteID: uuid.New().String(), UserID: f.otherTenantUser, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)

	// 师傅档案（直属商户，无网点）
	require.NoError(t, db.Create(&models.TechnicianProfile{
		ID: uuid.New().String(), UserID: f.technician, TenantID: f.tid, Status: "active",
	}).Error)

	h := NewPersonnelHandler()
	r := gin.New()
	r.GET("/admin/personnel", func(c *gin.Context) {
		c.Request = c.Request.WithContext(actorOf(f, c).InjectContext(c.Request.Context()))
		c.Next()
	}, h.List)
	return r, f
}

// actorOf 依据查询参数 actor=merchant|site|system|customer 注入对应角色上下文
func actorOf(f personnel2065Fixture, c *gin.Context) testutil.TestActor {
	switch c.Query("actor") {
	case "merchant":
		return testutil.TestActor{TenantID: f.tid, OrgID: f.tid, UserID: f.merchantAdmin, Role: "ADMIN"}
	case "site":
		// 生产约定：site 级账号 JWT oid == site.id（MakeSiteMember 同口径）
		return testutil.TestActor{TenantID: f.tid, OrgID: f.siteA, UserID: f.siteAdminAcct, Role: "ADMIN"}
	case "system":
		return testutil.TestActor{TenantID: "", OrgID: f.rootOrg, UserID: f.platformStaff, Role: "ADMIN"}
	default:
		return testutil.TestActor{TenantID: f.tid, OrgID: f.tid, UserID: f.customerOnly, Role: "USER"}
	}
}

func personnelList2065(t *testing.T, r *gin.Engine, actor string) (int, []map[string]interface{}) {
	return personnelList2065Q(t, r, "actor="+actor)
}

func personnelList2065Q(t *testing.T, r *gin.Engine, query string) (int, []map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/personnel?"+query, nil))
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	list, _ := resp.Data["list"].([]interface{})
	out := make([]map[string]interface{}, 0, len(list))
	for _, it := range list {
		if m, ok := it.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return w.Code, out
}

func names2065(rows []map[string]interface{}) map[string]map[string]interface{} {
	m := map[string]map[string]interface{}{}
	for _, r := range rows {
		m[r["name"].(string)] = r
	}
	return m
}

func TestPersonnel2065_MerchantView(t *testing.T) {
	r, f := setupPersonnel2065(t)
	code, rows := personnelList2065(t, r, "merchant")
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)

	assert.Contains(t, m, "网点A成员")
	assert.Contains(t, m, "网点B成员")
	assert.Contains(t, m, "维修张师傅", "商户视图须含师傅")
	assert.NotContains(t, m, "纯顾客", "顾客不进人员管理")
	assert.NotContains(t, m, "他商户成员", "跨商户不可见")
	assert.NotContains(t, m, "平台员工", "平台员工不在商户视图")
	// 师傅行：所属网点=直属商户、职位=维修师傅
	tech := m["维修张师傅"]
	assert.Equal(t, "直属商户", tech["site_name"])
	assert.Equal(t, "维修师傅", tech["position"])
	assert.Equal(t, true, tech["is_technician"])
	_ = f
}

func TestPersonnel2065_SiteView(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	code, rows := personnelList2065(t, r, "site")
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)

	assert.Contains(t, m, "网点A成员")
	assert.NotContains(t, m, "网点B成员", "跨网点不可见")
	assert.NotContains(t, m, "维修张师傅", "网点视图不含师傅（师傅直属商户）")
	assert.NotContains(t, m, "平台员工")
}

func TestPersonnel2065_SystemView(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	code, rows := personnelList2065(t, r, "system")
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)

	assert.Contains(t, m, "平台员工")
	assert.Contains(t, m, "中转成员")
	assert.NotContains(t, m, "网点A成员", "系统视图不含普通商户网点成员")
	assert.Equal(t, "平台", m["平台员工"]["site_name"])
	assert.Equal(t, "中转网点", m["中转成员"]["site_name"])
}

func TestPersonnel2065_CustomerDenied(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/personnel", nil))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "40303")
}

// #2065 审计 H1 回归：页面既有搜索表单参数（name/site_id）必须生效
func TestPersonnel2065_MerchantSearchAndSiteFilter(t *testing.T) {
	r, f := setupPersonnel2065(t)

	// name 过滤：仅「网点A成员」命中
	code, rows := personnelList2065Q(t, r, "actor=merchant&name="+url.QueryEscape("网点A成员"))
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)
	assert.Contains(t, m, "网点A成员")
	assert.NotContains(t, m, "网点B成员")
	assert.NotContains(t, m, "维修张师傅")

	// site_id 过滤：仅该网点成员（师傅不属网点 → 排除）
	code, rows = personnelList2065Q(t, r, "actor=merchant&site_id="+f.siteA)
	require.Equal(t, http.StatusOK, code)
	m = names2065(rows)
	assert.Contains(t, m, "网点A成员")
	assert.NotContains(t, m, "网点B成员", "其他网点成员排除")
	assert.NotContains(t, m, "维修张师傅", "指定网点时师傅（直属商户）排除")
	assert.NotContains(t, m, "纯顾客")
}

// #2067 回归：membership 锚定（users.tenant 不可靠不得漏人）+ 只看直属筛选
func TestPersonnel2067_MembershipAnchorAndDirect(t *testing.T) {
	r, _ := setupPersonnel2065(t)

	// 商户视图：零租户用户（membership 在商户内）必须可见——修复前因 users.tenant 过滤而漏
	code, rows := personnelList2065Q(t, r, "actor=merchant")
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)
	assert.Contains(t, m, "零租户成员", "membership 锚定：users.tenant_id 不可靠")
	assert.Contains(t, m, "直属员工", "商户直属员工须可见")
	assert.Contains(t, m, "维修张师傅")

	// direct=true → 仅直属员工 + 师傅（排除网点成员）
	code2, rows2 := personnelList2065Q(t, r, "actor=merchant&direct=true")
	require.Equal(t, http.StatusOK, code2)
	m2 := names2065(rows2)
	assert.Contains(t, m2, "直属员工")
	assert.Contains(t, m2, "维修张师傅")
	assert.NotContains(t, m2, "网点A成员", "直属筛选排除网点成员")
	assert.NotContains(t, m2, "零租户成员")

	// direct 优先：同时带 site_id 仍按直属（不返回空集）
	code3, rows3 := personnelList2065Q(t, r, "actor=merchant&direct=true&site_id=00000000-0000-0000-0000-000000000001")
	require.Equal(t, http.StatusOK, code3)
	m3 := names2065(rows3)
	assert.Contains(t, m3, "直属员工", "direct 优先：同时带 site_id 不得返回空集")
	assert.Contains(t, m3, "维修张师傅")
	assert.NotContains(t, m3, "网点A成员")
}

// #2072 回归：列表按 created_at DESC（新建用户置顶）
func TestPersonnel2072_NewestFirst(t *testing.T) {
	r, f := setupPersonnel2065(t)
	db := database.GetDB()
	newID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: newID, IAMSub: newID, TenantID: f.tid, OrgID: f.tid,
		Username: "new-" + newID[:8], Name: "最新成员", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.tid, SiteID: f.siteA, UserID: newID, Role: "STAFF", Status: "active",
	}).Error)

	code, rows := personnelList2065Q(t, r, "actor=merchant")
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, rows)
	assert.Equal(t, "最新成员", rows[0]["name"], "新建用户须排首位")
}
