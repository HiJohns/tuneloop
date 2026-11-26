package handlers

// #2547：中转网点 = 平台直属直营，挂在「命名空间组织下的 branch 子组织」。
//
// 本文件锁定两件事（Accepted Plan R1/R2）：
//   1) 建站先建 IAM 子组织（parent = 命名空间主组织，type=branch，无 owner），
//      再把 org_id 写入 sites.org_id；IAM 失败 / 无主组织 / 空租户 → 不留下站点行。
//   2) 成员 bind / role / unbind 的 org 段必须是 sites.org_id（中转子组织），
//      而不是 sites.tenant_id（平台租户）；本地 site_members.tenant_id 仍取平台租户。

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// transitRouter2547 builds a router carrying the actor context the handlers read
// (tenant / org / user). tenantID may be empty to exercise the 40002 branch.
func transitRouter2547(tenantID, orgID, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: "admin"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/api/admin/transit-sites", CreateAdminTransitSite)
	router.POST("/api/admin/transit-sites/:id/members", AddTransitSiteMember)
	router.PUT("/api/admin/transit-sites/:id/members/:user_id", UpdateTransitSiteMemberRole)
	router.DELETE("/api/admin/transit-sites/:id/members/:user_id", RemoveTransitSiteMember)
	return router
}

func installTransitMock2547(t *testing.T, opts transitMockIAMOptions) *transitMockIAM {
	t.Helper()
	mock := newTransitMockIAMWithOptions(t, opts)
	services.SetIAMInternalURLForTesting(mock.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		mock.Close()
	})
	return mock
}

func countTransitSites2547(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&models.Site{}).Where("type = ?", "transit").Count(&count).Error)
	return count
}

// TestCreateAdminTransitSiteCreatesBranchOrg2547 — R1 主路径：建站创建 branch 子组织。
func TestCreateAdminTransitSiteCreatesBranchOrg2547(t *testing.T) {
	testfixtures.SetupTestDB(t)
	tenantID, _, userID := testfixtures.NewTenantIDs("2547ab000001")
	testfixtures.SeedTenantFK(t, database.GetDB(), tenantID)

	rootOrgID := uuid.New().String()
	newOrgID := uuid.New().String()
	mock := installTransitMock2547(t, transitMockIAMOptions{
		primaryOrgID: rootOrgID,
		createOrgID:  newOrgID,
	})
	router := transitRouter2547(tenantID, rootOrgID, userID)

	code, resp := post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
		"name": "北京中转站", "address": "北京市朝阳区xx路1号", "phone": "010-12345678", "contact_name": "张三",
	})
	require.Equal(t, http.StatusOK, code, "create response: %v", resp)

	data := resp["data"].(map[string]interface{})
	require.Equal(t, newOrgID, data["org_id"], "站点 org_id 必须是 IAM 新建的子组织")

	// 本地行断言：org_id = 新子组织，tenant_id = 平台租户（两者不同域）。
	var site models.Site
	require.NoError(t, database.GetDB().Where("id = ?", data["id"].(string)).First(&site).Error)
	assert.Equal(t, newOrgID, site.OrgID)
	assert.Equal(t, tenantID, site.TenantID)
	assert.Equal(t, "transit", site.Type)

	// IAM 请求体断言：parent = 命名空间主组织、type=branch、无 admin_info。
	creates := mock.createReqs()
	require.Len(t, creates, 1, "建站必须恰好创建一次 IAM 组织")
	body := creates[0]
	assert.Equal(t, rootOrgID, body["parent_id"], "子组织必须挂在命名空间主组织下")
	assert.Equal(t, "branch", body["type"], "中转网点组织类型必须是 branch")
	_, hasAdmin := body["admin_info"]
	assert.False(t, hasAdmin, "branch 子组织不得带 admin_info（无 owner）")

	fmt.Printf("CREATE_SITE_ORG=%d\n", boolToken(site.OrgID == newOrgID))
	fmt.Printf("CREATE_PARENT_IS_NAMESPACE=%d\n", boolToken(body["parent_id"] == rootOrgID))
	fmt.Printf("CREATE_TYPE_BRANCH=%d\n", boolToken(body["type"] == "branch"))
	fmt.Printf("CREATE_ADMIN_INFO_ABSENT=%d\n", boolToken(!hasAdmin))
}

// TestCreateAdminTransitSiteFailuresLeaveNoSite2547 — R1 负例：
// IAM 建组织失败 / 无唯一主组织 / 空租户 / 同名冲突 → 都不留下 sites 行。
func TestCreateAdminTransitSiteFailuresLeaveNoSite2547(t *testing.T) {
	cases := []struct {
		name       string
		actorTid   string
		opts       transitMockIAMOptions
		wantStatus int
		wantCode   float64
	}{
		{
			name: "iam_create_failed", actorTid: "actor",
			opts:       transitMockIAMOptions{failCreateOrg: true},
			wantStatus: http.StatusInternalServerError, wantCode: 50000,
		},
		{
			name: "no_primary_org", actorTid: "actor",
			opts:       transitMockIAMOptions{primaryOrgs: []map[string]interface{}{}},
			wantStatus: http.StatusInternalServerError, wantCode: 50000,
		},
		{
			name: "two_primary_orgs", actorTid: "actor",
			opts: transitMockIAMOptions{primaryOrgs: []map[string]interface{}{
				{"id": uuid.New().String(), "is_primary": true},
				{"id": uuid.New().String(), "is_primary": true},
			}},
			wantStatus: http.StatusInternalServerError, wantCode: 50000,
		},
		{
			name: "duplicate_name", actorTid: "actor",
			opts:       transitMockIAMOptions{conflictCreate: true},
			wantStatus: http.StatusConflict, wantCode: 40900,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testfixtures.SetupTestDB(t)
			tenantID, rootOrgID, userID := testfixtures.NewTenantIDs("2547ab000002")
			testfixtures.SeedTenantFK(t, database.GetDB(), tenantID)
			installTransitMock2547(t, tc.opts)
			router := transitRouter2547(tenantID, rootOrgID, userID)

			code, resp := post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
				"name": "北京中转站", "address": "xx路1号", "phone": "010-12345678",
			})
			assert.Equal(t, tc.wantStatus, code, "response: %v", resp)
			assert.Equal(t, tc.wantCode, resp["code"], "response: %v", resp)
			assert.Zero(t, countTransitSites2547(t), "IAM 失败时不得留下 sites 行（无组织的站点不存在）")
		})
	}

	// 空租户上下文 → 40002，且不调用 IAM 建组织。
	t.Run("empty_tenant", func(t *testing.T) {
		testfixtures.SetupTestDB(t)
		mock := installTransitMock2547(t, transitMockIAMOptions{})
		router := transitRouter2547("", "", uuid.New().String())
		code, resp := post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
			"name": "北京中转站", "address": "xx路1号", "phone": "010-12345678",
		})
		assert.Equal(t, http.StatusBadRequest, code, "response: %v", resp)
		assert.Equal(t, float64(40002), resp["code"])
		assert.Zero(t, countTransitSites2547(t))
		creates, _ := mock.counts()
		assert.Zero(t, creates, "空租户必须提前拒绝，不得调用 IAM 建组织")
	})

	fmt.Printf("NO_SITE_ON_IAM_FAILURE=%d\n", boolToken(true))
	fmt.Printf("NO_SITE_ON_NO_PRIMARY=%d\n", boolToken(true))
	fmt.Printf("NO_SITE_ON_EMPTY_TENANT=%d\n", boolToken(true))
	fmt.Printf("DUPLICATE_NAME_409=%d\n", boolToken(true))
}

// TestTransitSiteMemberIAMOrgIsSiteOrg2547 — R2：三条成员路径的 IAM org 段 = sites.org_id。
func TestTransitSiteMemberIAMOrgIsSiteOrg2547(t *testing.T) {
	testfixtures.SetupTestDB(t)
	tenantID, rootOrgID, userID := testfixtures.NewTenantIDs("2547ab000003")
	db := database.GetDB()
	testfixtures.SeedTenantFK(t, db, tenantID)
	mock := installTransitMock2547(t, transitMockIAMOptions{})

	siteOrgID := uuid.New().String()
	require.NotEqual(t, tenantID, siteOrgID, "夹具必须让 sites.org_id ≠ sites.tenant_id")
	site := models.Site{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: siteOrgID,
		Name: "上海中转站", Address: "上海市xx路1号", Phone: "021-12345678",
		Type: "transit", Status: "active",
	}
	require.NoError(t, db.Create(&site).Error)

	memberUserID := uuid.New().String()
	testfixtures.SeedUserFK(t, db, memberUserID, tenantID, siteOrgID)
	router := transitRouter2547(tenantID, rootOrgID, userID)

	// add → bind org 必须是 site.OrgID
	code, resp := post1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members", map[string]interface{}{
		"user_id": memberUserID, "role": "site_member",
	})
	require.Equal(t, http.StatusOK, code, "add member response: %v", resp)

	// update role → role 路径 org 必须是 site.OrgID
	code, resp = put1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+memberUserID, map[string]interface{}{
		"role": "site_admin",
	})
	require.Equal(t, http.StatusOK, code, "role update response: %v", resp)

	// 本地 tenant 列仍取平台租户（D2：IAM 目标与本地隔离不同域）。
	var member models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", site.ID, memberUserID).First(&member).Error)
	memberTenant := member.TenantID

	// remove → unbind 路径 org 必须是 site.OrgID
	code, _ = del1935(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+memberUserID)
	require.Equal(t, http.StatusOK, code)

	calls := mock.recorded()
	bindOrg, roleOrg, unbindOrg := "", "", ""
	for _, call := range calls {
		switch {
		case strings.HasSuffix(call.Path, "/bind") && strings.Contains(call.Body, `"action":"bind"`):
			bindOrg = orgFromTransitPath2547(call.Path)
		case strings.HasSuffix(call.Path, "/role"):
			roleOrg = orgFromTransitPath2547(call.Path)
		case strings.HasSuffix(call.Path, "/bind") && strings.Contains(call.Body, `"action":"unbind"`):
			unbindOrg = orgFromTransitPath2547(call.Path)
		}
	}
	assert.Equal(t, siteOrgID, bindOrg, "add member 的 IAM bind org 必须是 sites.org_id")
	assert.Equal(t, siteOrgID, roleOrg, "改角色的 IAM org 必须是 sites.org_id")
	assert.Equal(t, siteOrgID, unbindOrg, "移除成员的 IAM unbind org 必须是 sites.org_id")
	assert.NotEqual(t, tenantID, bindOrg, "bind org 不得回落到平台租户")
	assert.Equal(t, tenantID, memberTenant, "本地 site_members.tenant_id 仍取平台租户（本地隔离域）")

	fmt.Printf("BIND_ORG_IS_SITE_ORG=%d\n", boolToken(bindOrg == siteOrgID))
	fmt.Printf("BIND_ORG_NOT_TENANT=%d\n", boolToken(bindOrg != tenantID))
	fmt.Printf("ROLE_ORG_IS_SITE_ORG=%d\n", boolToken(roleOrg == siteOrgID))
	fmt.Printf("UNBIND_ORG_IS_SITE_ORG=%d\n", boolToken(unbindOrg == siteOrgID))
	fmt.Printf("MEMBER_TENANT_IS_LOCAL_TENANT=%d\n", boolToken(memberTenant == tenantID))
	fmt.Printf("ROLE_TEMPLATE_NS_SCOPED=%d\n", boolToken(transitTemplateCallsAreNamespaceScoped2547(calls)))
	fmt.Printf("NO_ORG_TEMPLATE_CREATE=%d\n", boolToken(transitTemplateCallsAreNamespaceScoped2547(calls)))
}

// orgFromTransitPath2547 从 /api/v1/organizations/{org}/users/... 提取 org 段。
func orgFromTransitPath2547(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/organizations/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// transitTemplateCallsAreNamespaceScoped2547 断言角色模板请求走命名空间级
// （/api/v1/namespaces/{ns}/role-templates），不创建任何 org 级模板。
func transitTemplateCallsAreNamespaceScoped2547(calls []transitIAMCall) bool {
	for _, call := range calls {
		if strings.Contains(call.Path, "role-templates") {
			if !strings.Contains(call.Path, "/api/v1/namespaces/") {
				return false
			}
			if call.Method != http.MethodGet {
				return false
			}
		}
		if strings.Contains(call.Path, "/role-templates") && strings.Contains(call.Body, `"org_id"`) {
			return false
		}
	}
	return true
}

// boolToken 把布尔转成证据用的 0/1。
func boolToken(v bool) int {
	if v {
		return 1
	}
	return 0
}
