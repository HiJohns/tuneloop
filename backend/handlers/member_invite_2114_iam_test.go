package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2114 P1 门控（IAM 权威判定）：网点级管理员只能直接添加「已是本商户成员」的用户。
// mock IAM 组织树：A(is_primary 平台根) ← B(商户根) ← C(网点)。
const (
	iamOrgA2114 = "aaaaaaaa-0000-4000-8000-00000000000a" // 平台根（primary，所有人级联持有）
	iamOrgB2114 = "bbbbbbbb-0000-4000-8000-00000000000b" // 商户根
	iamOrgC2114 = "cccccccc-0000-4000-8000-00000000000c" // 网点
)

// orgTreeHandler2114 返回 C→B→A(primary) 的组织树；其余组织操作（bind 等）返回通用成功。
func orgTreeHandler2114(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/"):
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	case r.URL.Path == "/api/v1/organizations/"+iamOrgC2114:
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": iamOrgC2114, "parent_id": iamOrgB2114, "is_primary": false})
	case r.URL.Path == "/api/v1/organizations/"+iamOrgB2114:
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": iamOrgB2114, "parent_id": iamOrgA2114, "is_primary": false})
	case r.URL.Path == "/api/v1/organizations/"+iamOrgA2114:
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": iamOrgA2114, "is_primary": true})
	default:
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "code": 20000})
	}
}

// setupP1GateTestDB 只建站点 + 目标用户（**故意不建 site_members 行**——已成员会被
// 去重 continue 跳过而不走门控）；mock IAM 由各用例按需注册。
func setupP1GateTestDB(t *testing.T, tenantID string) (siteID, userID string) {
	t.Helper()
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	siteID = uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, Name: "P1 Site", TenantID: tenantID, OrgID: iamOrgC2114, Status: "active",
	}).Error)
	userID = uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: "sub-" + userID, TenantID: tenantID, OrgID: iamOrgC2114,
		Name: "P1 User", Status: "active",
	}).Error)
	return siteID, userID
}

// mountMockUserOrgs2114 注册用户组织列表 mock（orgs 为空且 status!=200 时模拟 IAM 故障）。
func mountMockUserOrgs2114(t *testing.T, orgs []string, status int) {
	t.Helper()
	mockIAM := newMockIAMServer(orgTreeHandler2114, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		items := make([]map[string]string, 0, len(orgs))
		for _, o := range orgs {
			items = append(items, map[string]string{"id": o})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"organizations": items})
	})
	t.Cleanup(mockIAM.Close)
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
}

// postP1Member2114WithTenant 以网点管理员（site_admin → adminLevel=false）身份提交添加成员。
func postP1Member2114WithTenant(t *testing.T, tenantID, siteID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")      // → BusinessRoleSiteAdmin
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, iamOrgC2114) // tid != oid → 非商户管理员
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/sites/:id/members", NewSiteMemberHandler().AddMember)

	body, _ := json.Marshal(map[string]interface{}{
		"user_ids": []map[string]string{{"user_id": userID, "role": "site_member"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID+"/members", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// 红绿实证：旧实现（本地缓存判定）对该用例返回 40311（本地无行）；
// 新实现以 IAM 关系为准（用户持商户根 B 关系）→ 直接绑定成功。
func TestP1Gate2114_MemberViaMerchantRoot_DirectAdd(t *testing.T) {
	tenantID := uuid.NewString()
	siteID, userID := setupP1GateTestDB(t, tenantID)
	mountMockUserOrgs2114(t, []string{iamOrgB2114}, http.StatusOK)

	w := postP1Member2114WithTenant(t, tenantID, siteID, userID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DirectlyAdded []gin.H `json:"directly_added"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20100, resp.Code)
	assert.Len(t, resp.Data.DirectlyAdded, 1, "P1（已是商户成员）应直接绑定")
}

// 平台根（is_primary）不参与成员判定：仅持有 A:member ≠ 商户成员。
func TestP1Gate2114_PlatformRootOnly_NeedsApply(t *testing.T) {
	tenantID := uuid.NewString()
	siteID, userID := setupP1GateTestDB(t, tenantID)
	mountMockUserOrgs2114(t, []string{iamOrgA2114}, http.StatusOK)

	w := postP1Member2114WithTenant(t, tenantID, siteID, userID)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "40311", "仅平台根关系应判为非本商户成员 → 40311 need_apply")
}

// IAM 关系与本商户无交集 → 40311（P2）。
func TestP1Gate2114_UnrelatedOrg_NeedsApply(t *testing.T) {
	tenantID := uuid.NewString()
	siteID, userID := setupP1GateTestDB(t, tenantID)
	mountMockUserOrgs2114(t, []string{"dddddddd-0000-4000-8000-00000000000d"}, http.StatusOK)

	w := postP1Member2114WithTenant(t, tenantID, siteID, userID)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "40311")
}

// IAM 校验失败 → 显式 500（不静默吞错、不静默回退本地缓存）。
func TestP1Gate2114_IAMFailure_ExplicitError(t *testing.T) {
	tenantID := uuid.NewString()
	siteID, userID := setupP1GateTestDB(t, tenantID)
	mountMockUserOrgs2114(t, nil, http.StatusInternalServerError)

	w := postP1Member2114WithTenant(t, tenantID, siteID, userID)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "50000")
}
