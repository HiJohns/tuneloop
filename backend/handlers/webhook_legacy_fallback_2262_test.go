package handlers

// #2262 返工：handler 真实 scoped ctx 下，旧 tenant 键必须可读（只读回退）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerchantWebhook_LegacyKeyFallbackUnderScopedCtx_2262(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "WH-LEGACY",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	// 仅旧 tenant 键存在
	enc, err := services.EncryptSecret("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=legacy")
	require.NoError(t, err)
	raw, _ := json.Marshal(services.WebhookConfig{Enabled: true, URL: enc})
	require.NoError(t, db.Create(&models.SystemSetting{
		TenantID: shared, SettingKey: services.MerchantWebhookKey, SettingValue: string(raw),
	}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/api/merchant/notification-webhook", GetMerchantWebhook)
	r.PUT("/api/merchant/notification-webhook", UpdateMerchantWebhook)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/merchant/notification-webhook", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Enabled bool `json:"enabled"`
			URLSet  bool `json:"url_set"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Data.Enabled)
	assert.True(t, resp.Data.URLSet, "旧 tenant 键必须可读（scoped ctx 回退）")

	// 保存一次 → 写新 org 键；再次保存 → 更新同一行（不重复/不报错）
	for i := 0; i < 2; i++ {
		body := `{"enabled":true,"url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=new2262"}`
		req := httptest.NewRequest(http.MethodPut, "/api/merchant/notification-webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	var count int64
	require.NoError(t, db.Model(&models.SystemSetting{}).
		Where("tenant_id = ? AND setting_key = ?", orgA, services.MerchantWebhookKey).Count(&count).Error)
	assert.EqualValues(t, 1, count, "新键单写（org 键唯一）")
}
