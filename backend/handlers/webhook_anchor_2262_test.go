package handlers

// #2262 B5：商户 webhook 配置键 = 商户 org；共享 tenant 双商户互不覆盖，旧 tenant 键只读回退。

import (
	"encoding/json"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerchantWebhook_SharedTenantIsolation_2262(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB, orgC := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, m := range []struct{ org, name string }{{orgA, "A"}, {orgB, "B"}, {orgC, "C"}} {
		require.NoError(t, db.Create(&models.Merchant{
			ID: uuid.NewString(), TenantID: shared, OrgID: m.org, Name: m.name,
			Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
		}).Error)
	}

	urlA := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=aaa"
	urlB := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=bbb"
	require.NoError(t, services.SaveMerchantWebhook(db, orgA, services.WebhookConfig{Enabled: true, URL: urlA}))
	require.NoError(t, services.SaveMerchantWebhook(db, orgB, services.WebhookConfig{Enabled: true, URL: urlB}))

	cfgA, err := services.LoadMerchantWebhook(db, orgA)
	require.NoError(t, err)
	assert.Equal(t, urlA, cfgA.URL, "A 配置不得被 B 覆盖")
	cfgB, err := services.LoadMerchantWebhook(db, orgB)
	require.NoError(t, err)
	assert.Equal(t, urlB, cfgB.URL)

	var n int64
	require.NoError(t, db.Model(&models.SystemSetting{}).Where("tenant_id IN ?", []string{orgA, orgB}).Count(&n).Error)
	assert.EqualValues(t, 2, n, "新写入必须落在商户 org 键")

	// 旧 tenant 键只读回退（C 无新键）
	enc, err := services.EncryptSecret("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=ccc")
	require.NoError(t, err)
	raw, _ := json.Marshal(services.WebhookConfig{Enabled: true, URL: enc})
	require.NoError(t, db.Create(&models.SystemSetting{
		TenantID: shared, SettingKey: services.MerchantWebhookKey, SettingValue: string(raw),
	}).Error)
	cfgC, err := services.LoadMerchantWebhook(db, orgC)
	require.NoError(t, err)
	assert.Contains(t, cfgC.URL, "key=ccc", "旧 tenant 键应可读（兼容）")
}
