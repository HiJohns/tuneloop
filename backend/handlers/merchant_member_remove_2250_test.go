package handlers

// #2250（返工）：RemoveMember 的成员行查询与 IAM 解绑必须统一锚定商户 org；
// 共享 tenant 下 merchant.tenant_id ≠ merchant.org_id，旧代码因此跳过解绑。

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

type iamCallRecord struct {
	Method string
	Path   string
	Action string
}

func TestRemoveMember_SharedTenant_OrgAnchorAndIAMUnbind_2250(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sharedTenant := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	merchantA := models.Merchant{
		ID: uuid.NewString(), TenantID: sharedTenant, OrgID: orgA, Name: "RM-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}
	merchantB := models.Merchant{
		ID: uuid.NewString(), TenantID: sharedTenant, OrgID: orgB, Name: "RM-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}
	require.NoError(t, db.Create(&merchantA).Error)
	require.NoError(t, db.Create(&merchantB).Error)

	iamSub := uuid.NewString()
	user := models.User{
		ID: uuid.NewString(), IAMSub: iamSub, Name: "待移除成员", Phone: "13900008851",
		TenantID: orgA, OrgID: orgA, Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: orgA, MerchantID: merchantA.ID, UserID: user.ID, Role: "site_member", Status: "active",
	}).Error)

	var mu sync.Mutex
	var calls []iamCallRecord
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rec := iamCallRecord{Method: r.Method, Path: r.URL.Path}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if action, ok := body["action"].(string); ok {
			rec.Action = action
		}
		mu.Lock()
		calls = append(calls, rec)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Setenv("IAM_SECRET", testIAMSecret)

	actor := func(orgID string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Request = c.Request.WithContext(testutil.TestActor{
				TenantID: orgID, OrgID: orgID, UserID: uuid.NewString(), Role: "OWNER",
			}.InjectContext(c.Request.Context()))
			c.Next()
		}
	}

	// 商户 B 不得移除商户 A 的成员
	rB := gin.New()
	rB.Use(actor(orgB))
	rB.DELETE("/admin/merchants/:id/members/:uid", (&MerchantMemberHandler{}).RemoveMember)
	reqB := httptest.NewRequest(http.MethodDelete, "/admin/merchants/"+merchantA.ID+"/members/"+user.ID, nil)
	reqB.Header.Set("Authorization", "Bearer test-token")
	wB := httptest.NewRecorder()
	rB.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusForbidden, wB.Code, wB.Body.String())
	var stillThere int64
	db.Model(&models.MerchantMember{}).Where("merchant_id = ? AND user_id = ?", merchantA.ID, user.ID).Count(&stillThere)
	assert.Equal(t, int64(1), stillThere, "B 的越权删除不得生效")

	// 商户 A 移除自己的成员 → 本地删除 + IAM 解绑（orgA）
	rA := gin.New()
	rA.Use(actor(orgA))
	rA.DELETE("/admin/merchants/:id/members/:uid", (&MerchantMemberHandler{}).RemoveMember)
	reqA := httptest.NewRequest(http.MethodDelete, "/admin/merchants/"+merchantA.ID+"/members/"+user.ID, nil)
	reqA.Header.Set("Authorization", "Bearer test-token")
	wA := httptest.NewRecorder()
	rA.ServeHTTP(wA, reqA)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())

	var remaining int64
	db.Model(&models.MerchantMember{}).Where("merchant_id = ? AND user_id = ?", merchantA.ID, user.ID).Count(&remaining)
	assert.Equal(t, int64(0), remaining, "成员行应已删除")

	mu.Lock()
	defer mu.Unlock()
	expectedPath := "/api/v1/organizations/" + orgA + "/users/" + iamSub + "/bind"
	var found bool
	for _, call := range calls {
		if call.Method == http.MethodPut && call.Path == expectedPath && call.Action == "unbind" {
			found = true
		}
		assert.False(t, strings.Contains(call.Path, orgB), "不得以商户 B 的 org 解绑: %s", call.Path)
	}
	assert.True(t, found, "必须调用 IAM 解绑（org=%s, sub=%s），实际调用: %+v", orgA, iamSub, calls)
}
