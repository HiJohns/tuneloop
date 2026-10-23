package handlers

// #2266：CreateSite 归属锚 = 目标商户 org（含平台代建 merchant_id）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestCreateSite_AnchorIsMerchantOrg_2266(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	platformOrg := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	merchantA := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantA, TenantID: shared, OrgID: orgA, Name: "A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	var mu sync.Mutex
	var lastParentID string
	var createCalls int
	iam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock-client-token", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/organizations"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			lastParentID, _ = body["parent_id"].(string)
			createCalls++
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"org_id": uuid.NewString()})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/organizations/"):
			isPrimary := strings.HasSuffix(r.URL.Path, platformOrg)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.NewString(), "is_primary": isPrimary})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/namespaces/"):
			json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.NewString()})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	defer iam.Close()
	services.SetIAMInternalURLForTesting(iam.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: c.GetHeader("X-Tenant"), OrgID: c.GetHeader("X-Org"),
			UserID: uuid.NewString(), Role: "OWNER",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/merchant/sites", NewSiteHandler().CreateSite)

	call := func(tenant, org, body string) (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodPost, "/api/merchant/sites", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant", tenant)
		req.Header.Set("X-Org", org)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 1) 共享 tenant + 商户管理员：锚 = 商户 org（旧实现写 shared → 红）
	code, resp := call(shared, orgA, `{"name":"S1"}`)
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	mu.Lock()
	assert.Equal(t, orgA, lastParentID, "IAM ParentID 必须是商户 org")
	mu.Unlock()
	var site models.Site
	require.NoError(t, db.Where("name = ?", "S1").First(&site).Error)
	assert.Equal(t, orgA, site.TenantID, "sites.tenant_id 必须是商户 org")

	// 2) 平台代建：指定 merchant_id → 锚 = 该商户 org
	code, resp = call(shared, platformOrg, `{"name":"S2","merchant_id":"`+merchantA+`"}`)
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	mu.Lock()
	assert.Equal(t, orgA, lastParentID)
	mu.Unlock()
	site = models.Site{}
	require.NoError(t, db.Where("name = ?", "S2").First(&site).Error)
	assert.Equal(t, orgA, site.TenantID)

	// 2b) 平台代建不传 merchant_id → 拒绝且不写库
	var before int64
	db.Model(&models.Site{}).Count(&before)
	code, _ = call(shared, platformOrg, `{"name":"S3"}`)
	assert.Equal(t, http.StatusForbidden, code)
	var after int64
	db.Model(&models.Site{}).Count(&after)
	assert.Equal(t, before, after, "拒绝时不得写入 sites")

	// 3) 商户 B 管理员指定商户 A → 403，且不写库
	code, _ = call(shared, orgB, `{"name":"S4","merchant_id":"`+merchantA+`"}`)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, before, after)

	// 4) 平台代建指定不存在的商户 → 40002，且不写库
	code, _ = call(shared, platformOrg, `{"name":"S5","merchant_id":"`+uuid.NewString()+`"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	var final int64
	db.Model(&models.Site{}).Count(&final)
	assert.Equal(t, before, final, "无效商户不得写入 sites")

	mu.Lock()
	assert.GreaterOrEqual(t, createCalls, 2)
	mu.Unlock()
}
