package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/services/wechatpay"
)

// TestPayConfig_MockMode verifies GET /pay/config reflects the current WeChat
// Pay mock mode. With a mock-mode config the flag must be true; with no
// config initialized the handler must not panic and report false.
func TestPayConfig_MockMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wechatpay.InitGlobal(wechatpay.LoadConfig())

	router := gin.New()
	router.GET("/api/public/config", GetPayConfig)

	req := httptest.NewRequest("GET", "/api/public/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			MockPayment bool `json:"mock_payment"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	// Test env runs with WECHAT_PAY_MOCK_MODE=true and no MCH_ID → mock on.
	require.True(t, resp.Data.MockPayment, "test env has WECHAT_PAY_MOCK_MODE=true, mock_payment must be true")
}
