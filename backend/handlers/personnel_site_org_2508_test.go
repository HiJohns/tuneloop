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

// #2508 网点管理员人员列表恒空：JWT `oid`（组织 ID = sites.org_id）被当成
// `site_members.site_id`（= sites.id）使用。
//
// 夹具（setupPersonnel2065）已保证 sites.id ≠ sites.org_id，因此：
//   - 旧谓词 `sm.site_id = oid` 恒 0 行（红）；
//   - 新谓词 `s.org_id = oid` 命中本组织全部站点成员（绿）。
//
// 断言行标记（verify-2508.sh 逐行 grep）：
//   R1 BUGGY_ROWS / FIXED_ROWS / RED_CONFIRMED
//   R2 SITE_VIEW_ROWS / HAS_* / LEAK_*
//   R3 A_ONLY / B_ONLY / CROSS_LEAK
//   R4 OLD_KEYING_ROWS / STATUS
//   R5 MULTI_ROWS / FILTER_A_ROWS / FILTER_B_ROWS / INVALID_SITE_CODE

// personnel2508Router 与 setupPersonnel2065 的路由同构，但允许用 `oid` 查询参数
// 指定 site_admin 的组织 ID（用于跨组织 / 旧口径负例）。
func personnel2508Router(f personnel2065Fixture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewPersonnelHandler()
	r := gin.New()
	r.GET("/admin/personnel", func(c *gin.Context) {
		oid := f.oid
		if q := c.Query("oid"); q != "" {
			oid = q
		}
		actor := testutil.TestActor{TenantID: f.tid, OrgID: oid, UserID: f.siteAdminAcct, Role: "ADMIN"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, h.List)
	return r
}

// R1：同一份数据下，旧谓词（site_id = oid）0 行、新谓词（s.org_id = oid）2 行。
func TestPersonnel2508_IDDomainRootCause(t *testing.T) {
	_, f := setupPersonnel2065(t)
	db := testfixtures.GetTestDB(t)

	var buggy int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM site_members sm WHERE sm.site_id = ? AND sm.status = 'active'", f.oid,
	).Scan(&buggy).Error)
	var fixedRows int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM site_members sm JOIN sites s ON s.id = sm.site_id
		 WHERE s.org_id = ? AND sm.status = 'active'`, f.oid,
	).Scan(&fixedRows).Error)

	require.NotEqual(t, f.oid, f.siteA, "夹具必须让 sites.id ≠ sites.org_id，否则缺陷不可见")
	assert.EqualValues(t, 0, buggy, "旧谓词 sm.site_id = oid 必须恒 0 行（缺陷证据）")
	assert.EqualValues(t, 2, fixedRows, "新谓词 s.org_id = oid 必须命中本组织 2 名成员")

	redConfirmed := 0
	if buggy == 0 && fixedRows == 2 {
		redConfirmed = 1
	}
	t.Logf("R1 BUGGY_ROWS=%d FIXED_ROWS=%d RED_CONFIRMED=%d", buggy, fixedRows, redConfirmed)
}

// R2：site_admin（oid = sites.org_id）列表返回本组织成员，且不含跨组织/师傅/平台/他租户。
func TestPersonnel2508_SiteView(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := personnel2508Router(f)

	code, rows := personnelList2065Q(t, r, "oid="+f.oid)
	require.Equal(t, http.StatusOK, code)
	m := names2065(rows)

	hasSiteAMember := boolInt(m, "网点A成员")
	hasZeroTenant := boolInt(m, "零租户成员")
	leakSiteB := boolInt(m, "网点B成员")
	leakTech := boolInt(m, "维修张师傅")
	leakPlatform := boolInt(m, "平台员工")
	leakOtherTenant := boolInt(m, "他商户成员")

	assert.Equal(t, 2, len(rows), "本组织（网点A）应恰好 2 名成员")
	assert.Equal(t, 1, hasSiteAMember)
	assert.Equal(t, 1, hasZeroTenant, "零租户成员（membership 权威）不得被自动租户范围滤掉")
	assert.Equal(t, 0, leakSiteB, "跨组织成员不可见")
	assert.Equal(t, 0, leakTech, "网点视图不含师傅")
	assert.Equal(t, 0, leakPlatform)
	assert.Equal(t, 0, leakOtherTenant)

	t.Logf("R2 SITE_VIEW_ROWS=%d HAS_SITE_A_MEMBER=%d HAS_ZERO_TENANT=%d LEAK_SITE_B=%d LEAK_TECH=%d LEAK_PLATFORM=%d LEAK_OTHER_TENANT=%d",
		len(rows), hasSiteAMember, hasZeroTenant, leakSiteB, leakTech, leakPlatform, leakOtherTenant)
}

// R3：跨组织隔离（A 只见 A，B 只见 B，无交叉泄漏）。
func TestPersonnel2508_CrossOrgIsolation(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := personnel2508Router(f)

	codeA, rowsA := personnelList2065Q(t, r, "oid="+f.oid)
	require.Equal(t, http.StatusOK, codeA)
	mA := names2065(rowsA)
	codeB, rowsB := personnelList2065Q(t, r, "oid="+f.siteBOrg)
	require.Equal(t, http.StatusOK, codeB)
	mB := names2065(rowsB)

	crossLeak := 0
	if _, ok := mA["网点B成员"]; ok {
		crossLeak = 1
	}
	if _, ok := mB["网点A成员"]; ok {
		crossLeak = 1
	}
	if _, ok := mB["零租户成员"]; ok {
		crossLeak = 1
	}
	aOnly := 0
	if len(rowsA) == 2 && crossLeak == 0 {
		aOnly = 1
	}
	bOnly := 0
	if len(rowsB) == 1 && crossLeak == 0 {
		if _, ok := mB["网点B成员"]; ok {
			bOnly = 1
		}
	}

	assert.Equal(t, 0, crossLeak, "两个组织不得互相可见")
	assert.Equal(t, 1, aOnly)
	assert.Equal(t, 1, bOnly)

	t.Logf("R3 A_ONLY=%d B_ONLY=%d CROSS_LEAK=%d", aOnly, bOnly, crossLeak)
}

// R4：旧错误口径（oid = sites.id）不再命中任何成员——旧行为已消失（负例）。
func TestPersonnel2508_OldKeyingGone(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := personnel2508Router(f)

	code, rows := personnelList2065Q(t, r, "oid="+f.siteA)
	require.Equal(t, http.StatusOK, code)

	assert.Equal(t, 0, len(rows), "按 sm.site_id = oid（站点 PK 当组织 ID）取值不得再命中")
	t.Logf("R4 OLD_KEYING_ROWS=%d STATUS=%d", len(rows), code)
}

// R5：一个组织多站点 → 返回全部站点成员；site_id 与本组织求交（跨组织 → 空集）；
// 非法 site_id 仍 40002。
func TestPersonnel2508_MultiSiteFilter(t *testing.T) {
	_, f := setupPersonnel2065(t)
	db := testfixtures.GetTestDB(t)
	r := personnel2508Router(f)

	// org A 的第二个站点 + 1 名成员（#2508 D1：不递归，只按 s.org_id 平铺）
	siteA2 := uuid.New().String()
	memberA2 := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA2, TenantID: f.tid, OrgID: f.oid, Name: "网点A2", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: memberA2, IAMSub: memberA2, TenantID: f.tid, OrgID: f.oid,
		Username: "u-" + memberA2[:8], Name: "网点A2成员", Role: "member", Status: "active",
	}).Error)
	testfixtures.SeedTenantFK(t, db, f.tid)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.tid, SiteID: siteA2, UserID: memberA2, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active",
	}).Error)

	codeAll, rowsAll := personnelList2065Q(t, r, "oid="+f.oid)
	require.Equal(t, http.StatusOK, codeAll)

	codeA, rowsA := personnelList2065Q(t, r, "oid="+f.oid+"&site_id="+f.siteA)
	require.Equal(t, http.StatusOK, codeA)

	codeB, rowsB := personnelList2065Q(t, r, "oid="+f.oid+"&site_id="+f.siteB)
	require.Equal(t, http.StatusOK, codeB)

	codeInvalid, codeInvalidBody := personnel2508BodyCode(t, r, "oid="+f.oid+"&site_id="+url.QueryEscape("null"))

	assert.Equal(t, 3, len(rowsAll), "组织 A 两个站点成员合计 3（2+1）")
	assert.Equal(t, 2, len(rowsA), "site_id 过滤命中网点A 的 2 名成员")
	assert.Equal(t, 0, len(rowsB), "跨组织站点 ID 与本组织求交 → 空集（不泄漏）")
	assert.Equal(t, http.StatusBadRequest, codeInvalid, "非法 site_id 仍须 400")
	assert.Equal(t, 40002, codeInvalidBody, "非法 site_id 的业务码仍须 40002")

	t.Logf("R5 MULTI_ROWS=%d FILTER_A_ROWS=%d FILTER_B_ROWS=%d INVALID_SITE_CODE=%d",
		len(rowsAll), len(rowsA), len(rowsB), codeInvalidBody)
}

// personnel2508BodyCode 返回 HTTP 状态码与响应体业务码（code 字段）。
func personnel2508BodyCode(t *testing.T, r *gin.Engine, query string) (int, int) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/personnel?"+query, nil))
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Code
}

func boolInt(m map[string]map[string]interface{}, name string) int {
	if _, ok := m[name]; ok {
		return 1
	}
	return 0
}
