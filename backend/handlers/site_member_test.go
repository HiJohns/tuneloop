package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2035: UpdateMemberRole 多角色保存/回读与归一化。
// site.OrgID 置空以隔离 IAM（仅验证本地多角色逻辑）。
func setupUpdateMemberRoleTest(t *testing.T) (tenantID string, siteID string, userID string) {
	t.Helper()
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup) // 允许调用方 defer cleanup 已被 t.Cleanup 接管
	_ = cleanup

	db := database.GetDB()
	tenantID = uuid.New().String()
	// OrgID 用合法 UUID（UUID 列不可写空串）；user.IAMSub 留空 → 跳过 IAM 分支，隔离本地逻辑
	site := models.Site{ID: uuid.New().String(), Name: "Role Site", TenantID: tenantID, OrgID: uuid.New().String(), Status: "active"}
	require.NoError(t, db.Create(&site).Error)
	user := models.User{ID: uuid.New().String(), TenantID: tenantID, OrgID: site.OrgID, Name: "U", Status: "active"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: site.ID, UserID: user.ID, Role: "site_member",
	}).Error)
	return tenantID, site.ID, user.ID
}

func newUpdateMemberRoleRouter(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewSiteMemberHandler()
	router.PUT("/api/sites/:id/members/:uid", h.UpdateMemberRole)
	return router
}

func putMemberRole(t *testing.T, router *gin.Engine, siteID, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PUT", "/api/sites/"+siteID+"/members/"+userID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestUpdateMemberRole_MultiRoleSaveReadBack(t *testing.T) {
	tenantID, siteID, userID := setupUpdateMemberRoleTest(t)
	db := database.GetDB()
	router := newUpdateMemberRoleRouter(t, tenantID)

	w := putMemberRole(t, router, siteID, userID,
		`{"role":"site_member","roles":["site_member","repair_technician"]}`)
	require.Equal(t, http.StatusOK, w.Code)

	var m models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteID, userID).First(&m).Error)
	assert.Equal(t, "site_member", m.Role, "主角色")
	assert.ElementsMatch(t, []string{"site_member", "repair_technician"}, m.Roles, "角色全集回读一致")
}

func TestUpdateMemberRole_NormalizesAndDedupes(t *testing.T) {
	tenantID, siteID, userID := setupUpdateMemberRoleTest(t)
	db := database.GetDB()
	router := newUpdateMemberRoleRouter(t, tenantID)

	// 乱序 + 重复：主角色置首，集合去重
	w := putMemberRole(t, router, siteID, userID,
		`{"role":"repair_technician","roles":["site_member","repair_technician","site_member"]}`)
	require.Equal(t, http.StatusOK, w.Code)

	var m models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteID, userID).First(&m).Error)
	assert.Equal(t, "repair_technician", m.Role, "role 为主角色")
	assert.Len(t, m.Roles, 2, "去重后 2 个角色")
	assert.ElementsMatch(t, []string{"site_member", "repair_technician"}, m.Roles)
}

func TestUpdateMemberRole_LegacyRoleOnly(t *testing.T) {
	tenantID, siteID, userID := setupUpdateMemberRoleTest(t)
	db := database.GetDB()
	router := newUpdateMemberRoleRouter(t, tenantID)

	w := putMemberRole(t, router, siteID, userID, `{"role":"site_admin"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var m models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteID, userID).First(&m).Error)
	assert.Equal(t, "site_admin", m.Role)
	assert.Equal(t, []string{"site_admin"}, m.Roles, "旧调用等价 roles=[role]")
}

func TestUpdateMemberRole_EmptyRejected(t *testing.T) {
	tenantID, siteID, userID := setupUpdateMemberRoleTest(t)
	router := newUpdateMemberRoleRouter(t, tenantID)

	for _, body := range []string{`{}`, `{"roles":[]}`, `{"role":"","roles":[]}`} {
		w := putMemberRole(t, router, siteID, userID, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s 应 400", body)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Contains(t, resp["message"], "role or roles is required")
	}
}
