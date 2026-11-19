package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #2429：编辑时**只改其它字段**（站点未变）不得触发 IAM 解绑/绑定。
// 旧实现 siteIDChanged 硬编码 true → 每次保存都会尝试 IAM 调用（测试环境无 IAM → 500），
// 且生产上会把直属员工误挂到所选网点。
func TestUpdateUser_SameSiteSkipsIAM_2429(t *testing.T) {
	_, handler, tenantID, _ := setupUserStaffTest(t)
	db := database.GetDB()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2429"}).Error)
	userID := createTestUser(t, db, tenantID)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: uuid.New().String(), Name: "测试网点",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userID,
		Role: "Staff", Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	// 携带**相同** site_id（真实场景：编辑页回填后直接保存）
	body := map[string]interface{}{"name": "改名", "position": "调律", "site_id": siteID}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/users/"+userID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.First(&updated, "id = ?", userID).Error)
	assert.Equal(t, "改名", updated.Name)
	assert.Equal(t, "调律", updated.Position)
	assert.Equal(t, tenantID, updated.OrgID, "站点未变更时 org 不得被改写")
}

// #2429：GET /admin/staff/:id 必须返回归属网点与「是否直属员工」标识。
func TestGetStaffUser_ReturnsSiteAndDirectFlag_2429(t *testing.T) {
	_, handler, tenantID, _ := setupUserStaffTest(t)
	db := database.GetDB()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2429b"}).Error)
	userID := createTestUser(t, db, tenantID)
	directUserID := createTestUser(t, db, tenantID)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: uuid.New().String(), Name: "西铁营",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userID,
		Role: "Staff", Status: "active",
	}).Error)
	// 直属员工：有 merchant_members，无 site_members
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.New().String(), TenantID: tenantID, MerchantID: uuid.New().String(),
		UserID: directUserID, Role: "merchant_admin", Status: "active", CreatedAt: time.Now(),
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/admin/staff/:id", handler.GetStaffUser)

	get := func(id string) map[string]interface{} {
		req := httptest.NewRequest("GET", "/api/admin/staff/"+id, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}

	d1 := get(userID)
	assert.Equal(t, siteID, d1["site_id"])
	assert.Equal(t, "西铁营", d1["site_name"])
	assert.Equal(t, false, d1["is_direct_staff"])

	d2 := get(directUserID)
	assert.Equal(t, "", d2["site_id"])
	assert.Equal(t, true, d2["is_direct_staff"])
}
