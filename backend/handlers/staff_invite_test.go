package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// #2031 邀请制自助加入：端点接线 + 守卫（未登录 401 / 无效邀请码 404）。
func TestAcceptInvite_Guards(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, uuid.New().String())
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

	t.Run("unknown code → 404", func(t *testing.T) {
		w := post("deadbeefdeadbeef")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

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
}
