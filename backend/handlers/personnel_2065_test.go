package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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
}

func setupPersonnel2065(t *testing.T) (*gin.Engine, personnel2065Fixture) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	f := personnel2065Fixture{
		tid:             uuid.New().String(),
		oid:             uuid.New().String(),
		otherTid:        uuid.New().String(),
		rootOrg:         uuid.New().String(),
		siteA:           uuid.New().String(),
		siteB:           uuid.New().String(),
		transitSite:     uuid.New().String(),
		merchantAdmin:   uuid.New().String(),
		siteAdminAcct:   uuid.New().String(),
		siteMemberA:     uuid.New().String(),
		siteMemberB:     uuid.New().String(),
		technician:      uuid.New().String(),
		platformStaff:   uuid.New().String(),
		transitMember:   uuid.New().String(),
		customerOnly:    uuid.New().String(),
		otherTenantUser: uuid.New().String(),
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

	// 网点（含一个中转网点，归属根组织）
	require.NoError(t, db.Create(&models.Site{ID: f.siteA, TenantID: f.tid, OrgID: f.tid, Name: "网点A", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.siteB, TenantID: f.tid, OrgID: f.tid, Name: "网点B", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.transitSite, TenantID: f.rootOrg, OrgID: f.rootOrg, Name: "中转网点", Type: "transit"}).Error)

	// 成员关系
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.tid, SiteID: f.siteA, UserID: f.siteMemberA, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: f.tid, SiteID: f.siteB, UserID: f.siteMemberB, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active"}).Error)
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
