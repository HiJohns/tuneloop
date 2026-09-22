package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2031 邀请制自助加入守卫：未登录 401 / 无效邀请码 404 / 过期 410 / 已用 409。
// 邀请码使用随机值以兼容共享测试库（避免跨运行唯一索引冲突）。
func TestAcceptInvite_Guards(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}))

	userID := uuid.New().String()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/accept-invite", AcceptInvite)

	post := func(code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"code": code})
		req := httptest.NewRequest("POST", "/api/user/accept-invite", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	randCode := func(prefix string) string {
		return prefix + uuid.New().String()[:12]
	}

	t.Run("未登录 → 401", func(t *testing.T) {
		e := gin.New()
		e.POST("/api/user/accept-invite", AcceptInvite)
		body, _ := json.Marshal(map[string]string{"code": "x"})
		req := httptest.NewRequest("POST", "/api/user/accept-invite", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown code → 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, post("deadbeefdeadbeef").Code)
	})

	t.Run("expired invite → 410", func(t *testing.T) {
		code := randCode("exp")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: uuid.New().String(), Role: "site_member", Code: code,
			Status: "pending", ExpiresAt: time.Now().Add(-time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		assert.Equal(t, http.StatusGone, post(code).Code)
	})

	t.Run("already accepted → 409", func(t *testing.T) {
		code := randCode("used")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: uuid.New().String(), Role: "site_member", Code: code,
			Status: "accepted", ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		assert.Equal(t, http.StatusConflict, post(code).Code)
	})

	// 回归（审计发现的真实 Bug）：created_by/accepted_by 未设置时必须写 NULL，
	// 而非空串——UUID 列拒绝 ''（SQLSTATE 22P02），否则签发邀请码会 500。
	t.Run("unset created_by/accepted_by → NULL, not empty uuid", func(t *testing.T) {
		code := randCode("nil")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: uuid.New().String(), Role: "site_member", Code: code,
			Status: "pending", ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		var stored models.StaffInvite
		require.NoError(t, db.Where("code = ?", code).First(&stored).Error)
		assert.Nil(t, stored.CreatedBy)
		assert.Nil(t, stored.AcceptedBy)
	})
}
