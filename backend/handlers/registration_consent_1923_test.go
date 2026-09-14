package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1923: registration (both the H5 /auth/register path and the weapp
// registration-session path) must reject callers that did not explicitly
// agree to 《租用服务协议》/《个人信息保护政策》.
func TestRegistrationRequiresAgreedTerms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	t.Run("h5_register", func(t *testing.T) {
		router := gin.New()
		router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)
		body, _ := json.Marshal(map[string]interface{}{
			"name": "未同意用户", "nickname": "未同意", "phone": "13900139999", "password": "secret123",
		})
		req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "租用服务协议")
	})

	t.Run("weapp_session", func(t *testing.T) {
		h := NewRegistrationSessionHandler(db)
		router := gin.New()
		router.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)
		body, _ := json.Marshal(map[string]interface{}{
			"name": "未同意用户", "nickname": "未同意", "phone": "13900139998",
		})
		req := httptest.NewRequest("POST", "/api/auth/registration-sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "租用服务协议")
	})
}
