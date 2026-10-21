package handlers

// #2251：商户级配置按商户 org 隔离（共享 tenant 的两个商户不得互相看见/改写配置）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type configActorApp struct {
	sharedTenant string
	orgA         string
	orgB         string
	merchantA    models.Merchant
	merchantB    models.Merchant
}

func setupConfigActors2251(t *testing.T) (*gin.Engine, configActorApp) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	app := configActorApp{
		sharedTenant: uuid.NewString(),
		orgA:         uuid.NewString(),
		orgB:         uuid.NewString(),
	}
	app.merchantA = models.Merchant{
		ID: uuid.NewString(), TenantID: app.sharedTenant, OrgID: app.orgA, Name: "M2251-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}
	app.merchantB = models.Merchant{
		ID: uuid.NewString(), TenantID: app.sharedTenant, OrgID: app.orgB, Name: "M2251-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		orgID := c.GetHeader("X-Test-Org")
		c.Request = c.Request.WithContext(testutil.TestActor{
			TenantID: app.sharedTenant, OrgID: orgID, UserID: uuid.NewString(), Role: "OWNER",
		}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/merchant/pricing-config", GetMerchantPricingConfig)
	r.PUT("/merchant/pricing-config", UpdateMerchantPricingConfig)
	r.GET("/config/repair", GetRepairAllSettings)
	r.GET("/config/repair/single", GetRepairSetting)
	r.PUT("/config/repair/single", SetRepairSetting)
	r.GET("/config/home-menu", GetHomeMenuConfig)
	r.PUT("/config/home-menu", SetHomeMenuConfig)
	r.GET("/merchant/points-policies", ListPointsPolicies)
	r.POST("/merchant/points-policies", CreatePointsPolicy)
	r.PUT("/merchant/points-policies/:id", UpdatePointsPolicy)
	r.GET("/merchant/rebate", GetMerchantRebateOptIn)
	r.PUT("/merchant/rebate", UpdateMerchantRebateOptIn)
	return r, app
}

func (a configActorApp) seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&a.merchantA).Error)
	require.NoError(t, db.Create(&a.merchantB).Error)
}

func configRequest(t *testing.T, r *gin.Engine, method, path, orgID string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Org", orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMerchantPricingConfig_SharedTenantIsolation_2251(t *testing.T) {
	r, app := setupConfigActors2251(t)
	db := testfixtures.SetupTestDB(t)
	app.seed(t, db)

	tmplA := models.PricingTemplate{ID: uuid.NewString(), Code: "t2251a", Name: "A", ConfigSchema: `{}`, IsActive: true}
	tmplB := models.PricingTemplate{ID: uuid.NewString(), Code: "t2251b", Name: "B", ConfigSchema: `{}`, IsActive: true}
	require.NoError(t, db.Create(&tmplA).Error)
	require.NoError(t, db.Create(&tmplB).Error)

	cfgA := models.MerchantPricingConfig{ID: uuid.NewString(), TenantID: app.orgA, TemplateID: tmplA.ID, Config: `{"deposit_ratio":0.11}`}
	cfgB := models.MerchantPricingConfig{ID: uuid.NewString(), TenantID: app.orgB, TemplateID: tmplB.ID, Config: `{"deposit_ratio":0.22}`}
	require.NoError(t, db.Create(&cfgA).Error)
	require.NoError(t, db.Create(&cfgB).Error)

	readConfig := func(orgID string) (string, string) {
		w := configRequest(t, r, http.MethodGet, "/merchant/pricing-config", orgID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				ID         string `json:"id"`
				TemplateID string `json:"template_id"`
				Config     struct {
					DepositRatio float64 `json:"deposit_ratio"`
				} `json:"config"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data.ID, resp.Data.TemplateID
	}

	idA, tmplSeenByA := readConfig(app.orgA)
	idB, tmplSeenByB := readConfig(app.orgB)
	assert.Equal(t, cfgA.ID, idA, "商户 A 必须读到 A 自己的配置")
	assert.Equal(t, cfgB.ID, idB, "商户 B 必须读到 B 自己的配置")
	assert.Equal(t, tmplA.ID, tmplSeenByA)
	assert.Equal(t, tmplB.ID, tmplSeenByB)

	// A 更新配置 → 只改 A
	w := configRequest(t, r, http.MethodPut, "/merchant/pricing-config", app.orgA, map[string]interface{}{
		"template_id": tmplA.ID,
		"config":      map[string]interface{}{"deposit_ratio": 0.33},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var storedA, storedB models.MerchantPricingConfig
	require.NoError(t, db.Where("tenant_id = ?", app.orgA).First(&storedA).Error)
	require.NoError(t, db.Where("tenant_id = ?", app.orgB).First(&storedB).Error)
	assert.Contains(t, storedA.Config, "0.33")
	assert.NotContains(t, storedB.Config, "0.33", "商户 B 的配置不得被 A 的写入影响")
}

func TestRepairSettings_SharedTenantIsolation_2251(t *testing.T) {
	r, app := setupConfigActors2251(t)
	db := testfixtures.SetupTestDB(t)
	app.seed(t, db)

	w := configRequest(t, r, http.MethodPut, "/config/repair/single", app.orgA, map[string]interface{}{
		"key": keyRepairInspectionFee, "value": "123",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	readAll := func(orgID string) map[string]string {
		w := configRequest(t, r, http.MethodGet, "/config/repair", orgID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data map[string]string `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}

	assert.Equal(t, "123", readAll(app.orgA)[keyRepairInspectionFee])
	_, leaked := readAll(app.orgB)[keyRepairInspectionFee]
	assert.False(t, leaked, "商户 B 不得读到商户 A 的维修设置")

	var row models.SystemSetting
	require.NoError(t, db.Where("setting_key = ?", keyRepairInspectionFee).First(&row).Error)
	assert.Equal(t, app.orgA, row.TenantID, "维修设置必须落商户 org 锚")
}

func TestHomeMenuConfig_PlatformLevel_2251(t *testing.T) {
	r, app := setupConfigActors2251(t)
	db := testfixtures.SetupTestDB(t)
	app.seed(t, db)

	w := configRequest(t, r, http.MethodPut, "/config/home-menu", app.orgA, map[string]interface{}{"config": `{"menu":"platform"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	for _, org := range []string{app.orgA, app.orgB} {
		w := configRequest(t, r, http.MethodGet, "/config/home-menu", org, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				Config string `json:"config"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.JSONEq(t, `{"menu":"platform"}`, resp.Data.Config, "平台级配置对所有商户一致（org=%s）", org)
	}

	var row models.SystemSetting
	require.NoError(t, db.Where("setting_key = ?", "home_menu_config").First(&row).Error)
	assert.Equal(t, systemTenantID, row.TenantID, "平台级配置必须以平台 tenant 存储")
}

func TestPointsPolicies_SharedTenantIsolation_2251(t *testing.T) {
	r, app := setupConfigActors2251(t)
	db := testfixtures.SetupTestDB(t)
	app.seed(t, db)

	// 客户端传入他人 scope_id 也必须被忽略（强制锚定本商户 org）
	w := configRequest(t, r, http.MethodPost, "/merchant/points-policies", app.orgA, map[string]interface{}{
		"scope_id": app.orgB, "max_pay_ratio": 0.5, "valid_days": 30, "is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created struct {
		Data models.PointsPolicy `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotNil(t, created.Data.ScopeID)
	assert.Equal(t, app.orgA, *created.Data.ScopeID, "策略归属必须锚定操作者商户 org")

	listFor := func(orgID string) []models.PointsPolicy {
		w := configRequest(t, r, http.MethodGet, "/merchant/points-policies", orgID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data []models.PointsPolicy `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}
	assert.Len(t, listFor(app.orgA), 1)
	assert.Len(t, listFor(app.orgB), 0, "商户 B 不得看到商户 A 的乐币策略")

	// 跨商户按 id 改写必须被拒绝
	w = configRequest(t, r, http.MethodPut, "/merchant/points-policies/"+created.Data.ID, app.orgB, map[string]interface{}{
		"max_pay_ratio": 0.9,
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestMerchantRebate_SharedTenantIsolation_2251(t *testing.T) {
	r, app := setupConfigActors2251(t)
	db := testfixtures.SetupTestDB(t)
	app.seed(t, db)
	// rebate_opt_in 列默认 true；显式置 false 以便区分两商户
	require.NoError(t, db.Model(&models.Merchant{}).
		Where("id IN ?", []string{app.merchantA.ID, app.merchantB.ID}).
		Update("rebate_opt_in", false).Error)

	readOptIn := func(orgID string) bool {
		w := configRequest(t, r, http.MethodGet, "/merchant/rebate", orgID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				OptIn bool `json:"rebate_opt_in"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data.OptIn
	}

	require.False(t, readOptIn(app.orgA))
	require.False(t, readOptIn(app.orgB))

	w := configRequest(t, r, http.MethodPut, "/merchant/rebate", app.orgA, map[string]interface{}{"opt_in": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.True(t, readOptIn(app.orgA))
	assert.False(t, readOptIn(app.orgB), "商户 B 的返点开关不得被 A 的写入影响")
}
