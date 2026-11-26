package handlers

// #2526: 员工 by-id 动作（详情/保存/激活）的可见性口径必须与人员列表三视图同源。
//
// 缺陷：存量行 `users.tenant_id`/`org_id` 为 nil UUID（生产样本 linwx1978@163.com），
// 列表按 membership 可见，但详情按 `users.tenant_id = ?` 过滤 → 「列表可见、点开 404」。
//
// Evidence rows: R1（零租户成员可加载 + 红例确认）、R2（site_admin）、R3（system_admin）、
// R4（越权负例）、R5（列表↔详情逐行一致）、R6（保存真正落库）、R7（激活行操作）、R8（归属缺失标记）。
//
// Token lines are printed with fmt.Printf so scripts/verify-2526.sh can grep them
// regardless of go test -v indentation.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func flag2526(cond bool) int {
	if cond {
		return 1
	}
	return 0
}

func staffRouter2526(f personnel2065Fixture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &UserStaffHandler{}
	withActor := func(c *gin.Context) {
		c.Request = c.Request.WithContext(actorOf(f, c).InjectContext(c.Request.Context()))
		c.Next()
	}
	r := gin.New()
	r.GET("/admin/staff/:id", withActor, h.GetStaffUser)
	r.PUT("/users/:id", withActor, h.UpdateUser)
	r.POST("/users/:id/activate", withActor, h.ActivateUser)
	return r
}

// seedZeroTenantRoleUser2526 追加 `role=USER` + nil 租户/组织 + site_members(siteA) 变体。
// 该形态对齐生产主诉样本（role=USER 的存量行仍以网点成员身份挂在商户下）。
func seedZeroTenantRoleUser2526(t *testing.T, f personnel2065Fixture) string {
	t.Helper()
	db := database.GetDB()
	id := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: id, IAMSub: id, TenantID: nilUUIDLiteral, OrgID: nilUUIDLiteral,
		Username: "u-" + id[:8], Name: "零租户顾客角色", Role: "USER", Status: "active",
	}).Error)
	testfixtures.SeedTenantFK(t, db, f.tid)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: f.tid, SiteID: f.siteA, UserID: id, Role: "STAFF", Roles: []string{"STAFF"}, Status: "active",
	}).Error)
	return id
}

type staffResp2526 struct {
	status int
	code   int
	data   map[string]interface{}
}

func serveStaff2526(t *testing.T, r *gin.Engine, method, path, actor string, payload interface{}) staffResp2526 {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	req := httptest.NewRequest(method, path+sep+"actor="+actor, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return staffResp2526{status: w.Code, code: resp.Code, data: resp.Data}
}

// R1 主诉：零租户成员（含 role=USER 生产形态）必须能加载详情。
func TestStaffDetail2526_ZeroTenantMember(t *testing.T) {
	_, f := setupPersonnel2065(t)
	roleUserID := seedZeroTenantRoleUser2526(t, f)
	r := staffRouter2526(f)

	status := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.zeroTenantMember, "merchant", nil)
	roleStatus := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+roleUserID, "merchant", nil)

	db := database.GetDB()
	var oldRows, membershipRows int64
	require.NoError(t, db.Model(&models.User{}).
		Where("id = ? AND tenant_id = ?", f.zeroTenantMember, f.tid).
		Count(&oldRows).Error)
	require.NoError(t, db.Table("site_members AS sm").
		Joins("JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id = ? AND sm.status = 'active' AND s.tenant_id = ?", f.zeroTenantMember, f.tid).
		Count(&membershipRows).Error)

	siteName, _ := status.data["site_name"].(string)

	fmt.Printf("DETAIL_ZERO_TENANT_STATUS=%d\n", status.status)
	fmt.Printf("DETAIL_ZERO_TENANT_USER_ROLE_STATUS=%d\n", roleStatus.status)
	fmt.Printf("OLD_PREDICATE_ROWS=%d\n", oldRows)
	fmt.Printf("MEMBERSHIP_ROWS=%d\n", membershipRows)
	fmt.Printf("DETAIL_SITE_NAME=%s\n", siteName)
	fmt.Printf("RED_CONFIRMED=%d\n", flag2526(oldRows == 0 && membershipRows == 1))

	require.Equal(t, http.StatusOK, status.status, "零租户成员详情必须可加载")
	require.Equal(t, 20000, status.code)
	require.Equal(t, http.StatusOK, roleStatus.status, "role=USER 的零租户成员（生产形态）同样可编辑")
	require.Equal(t, "网点A", siteName)
	require.Zero(t, oldRows, "夹具必须复刻缺陷：旧谓词按 tenant 过滤取不到该行")
	require.EqualValues(t, 1, membershipRows)
}

// R2：site_admin 详情按组织域判定；跨组织仍 404。
func TestStaffDetail2526_SiteAdmin(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := staffRouter2526(f)

	same := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.zeroTenantMember, "site", nil)
	cross := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.siteMemberB, "site", nil)

	fmt.Printf("SITE_DETAIL_ZERO=%d\n", same.status)
	fmt.Printf("SITE_DETAIL_CROSS_ORG=%d\n", cross.status)

	require.Equal(t, http.StatusOK, same.status, "本组织站点成员必须可加载（sites.org_id 口径）")
	require.Equal(t, http.StatusNotFound, cross.status, "跨组织成员不得泄漏")
}

// R3：system_admin 详情 = 平台员工 ∪ 中转网点成员；零租户普通成员不因 tenant 缺失被加宽。
func TestStaffDetail2526_SystemAdmin(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := staffRouter2526(f)

	platform := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.platformStaff, "system", nil)
	transit := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.transitMember, "system", nil)
	zero := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.zeroTenantMember, "system", nil)

	fmt.Printf("SYSTEM_PLATFORM=%d\n", platform.status)
	fmt.Printf("SYSTEM_TRANSIT=%d\n", transit.status)
	fmt.Printf("SYSTEM_ZERO_TENANT=%d\n", zero.status)

	require.Equal(t, http.StatusOK, platform.status)
	require.Equal(t, http.StatusOK, transit.status)
	require.Equal(t, http.StatusNotFound, zero.status, "system 口径不得被误加宽")
}

// R4：越权负例——纯顾客与他商户成员都返回 404/40400（不泄漏存在性）。
func TestStaffDetail2526_IsolationNegative(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := staffRouter2526(f)

	customer := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.customerOnly, "merchant", nil)
	other := serveStaff2526(t, r, http.MethodGet, "/admin/staff/"+f.otherTenantUser, "merchant", nil)

	fmt.Printf("MERCHANT_CUSTOMER=%d\n", customer.status)
	fmt.Printf("MERCHANT_CUSTOMER_BODY_CODE=%d\n", customer.code)
	fmt.Printf("MERCHANT_OTHER_TENANT=%d\n", other.status)
	fmt.Printf("MERCHANT_OTHER_TENANT_BODY_CODE=%d\n", other.code)

	require.Equal(t, http.StatusNotFound, customer.status, "纯顾客不得被当员工打开（B10）")
	require.Equal(t, 40400, customer.code)
	require.Equal(t, http.StatusNotFound, other.status)
	require.Equal(t, 40400, other.code)
}

// R5：列表 ↔ 详情逐行一致性（不变量）——列表可见的每一行都必须能被详情加载，
// 且网点归属不发生分裂。
func TestStaffDetail2526_ListDetailParity(t *testing.T) {
	listEngine, f := setupPersonnel2065(t)
	code, rows := personnelList2065(t, listEngine, "merchant")
	require.Equal(t, http.StatusOK, code)

	detail := staffRouter2526(f)
	detailOK, mismatch := 0, 0
	for _, row := range rows {
		id, _ := row["id"].(string)
		resp := serveStaff2526(t, detail, http.MethodGet, "/admin/staff/"+id, "merchant", nil)
		if resp.status != http.StatusOK {
			continue
		}
		detailOK++
		listSiteID, _ := row["site_id"].(string)
		detailSiteID, _ := resp.data["site_id"].(string)
		if listSiteID != "" && listSiteID != detailSiteID {
			mismatch++
		}
	}

	fmt.Printf("PARITY_LIST_ROWS=%d\n", len(rows))
	fmt.Printf("PARITY_DETAIL_OK=%d\n", detailOK)
	fmt.Printf("PARITY_MISMATCH=%d\n", mismatch)

	require.Equal(t, 5, len(rows), "夹具商户视图应为 5 行")
	require.Equal(t, len(rows), detailOK, "列表可见的每一行都必须能被详情加载")
	require.Zero(t, mismatch, "列表与详情的网点归属必须一致")
}

// R6：保存零租户行必须真正落库（RowsAffected 校验）；跨租户不可写。
func TestStaffUpdate2526_ZeroTenantPersists(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := staffRouter2526(f)
	db := database.GetDB()

	newName := "林维训-修正"
	upd := serveStaff2526(t, r, http.MethodPut, "/users/"+f.zeroTenantMember, "merchant", map[string]interface{}{
		"name": newName, "position": "维修师傅",
	})
	var reloaded models.User
	require.NoError(t, db.Where("id = ?", f.zeroTenantMember).First(&reloaded).Error)

	cross := serveStaff2526(t, r, http.MethodPut, "/users/"+f.otherTenantUser, "merchant", map[string]interface{}{
		"name": "不应写入",
	})
	var other models.User
	require.NoError(t, db.Where("id = ?", f.otherTenantUser).First(&other).Error)

	fmt.Printf("UPDATE_ZERO_STATUS=%d\n", upd.status)
	fmt.Printf("UPDATE_ZERO_NAME_PERSISTED=%d\n", flag2526(reloaded.Name == newName))
	fmt.Printf("UPDATE_CROSS_TENANT_STATUS=%d\n", cross.status)
	fmt.Printf("UPDATE_CROSS_TENANT_UNCHANGED=%d\n", flag2526(other.Name == "他商户成员"))

	require.Equal(t, http.StatusOK, upd.status)
	require.Equal(t, newName, reloaded.Name, "零租户行必须真正落库（禁止静默 0 行更新）")
	require.Equal(t, http.StatusNotFound, cross.status)
	require.Equal(t, "他商户成员", other.Name)
}

// R7：激活行操作对零租户行不再 404（已有 iam_sub → 40001）；跨租户仍 404。
func TestStaffActivate2526_RowAction(t *testing.T) {
	_, f := setupPersonnel2065(t)
	r := staffRouter2526(f)

	zero := serveStaff2526(t, r, http.MethodPost, "/users/"+f.zeroTenantMember+"/activate", "merchant", nil)
	cross := serveStaff2526(t, r, http.MethodPost, "/users/"+f.otherTenantUser+"/activate", "merchant", nil)

	fmt.Printf("ACTIVATE_ZERO_STATUS=%d\n", zero.code)
	fmt.Printf("ACTIVATE_ZERO_HTTP=%d\n", zero.status)
	fmt.Printf("ACTIVATE_CROSS_TENANT_STATUS=%d\n", cross.status)

	require.Equal(t, http.StatusBadRequest, zero.status, "零租户行应走到既有 40001（已激活），而不是 404")
	require.Equal(t, 40001, zero.code)
	require.Equal(t, http.StatusNotFound, cross.status)
}

// R8：归属缺失标记（三视图一致）——nil UUID 归属行标 1，正常行标 0。
func TestPersonnelAttributionFlag2526(t *testing.T) {
	engine, _ := setupPersonnel2065(t)

	code, merchantRows := personnelList2065(t, engine, "merchant")
	require.Equal(t, http.StatusOK, code)
	merchant := names2065(merchantRows)

	code, siteRows := personnelList2065(t, engine, "site")
	require.Equal(t, http.StatusOK, code)
	site := names2065(siteRows)

	flagOf := func(rows map[string]map[string]interface{}, name string) int {
		row, ok := rows[name]
		if !ok {
			return -1
		}
		v, _ := row["attribution_missing"].(bool)
		return flag2526(v)
	}

	zeroFlag := flagOf(merchant, "零租户成员")
	normalFlag := flagOf(merchant, "网点A成员")
	siteZeroFlag := flagOf(site, "零租户成员")

	fmt.Printf("FLAG_ZERO_TENANT=%d\n", zeroFlag)
	fmt.Printf("FLAG_NORMAL_MEMBER=%d\n", normalFlag)
	fmt.Printf("FLAG_SITE_VIEW_ZERO=%d\n", siteZeroFlag)

	require.Equal(t, 1, zeroFlag, "nil 租户/组织行必须标记归属缺失")
	require.Equal(t, 0, normalFlag, "正常行不得被标脏")
	require.Equal(t, 1, siteZeroFlag, "网点视图同源标记")
}
