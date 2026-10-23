package handlers

// #2274：负责人列表读取锚 = 商户 org（与写入锚一致）。

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

func TestMerchantMembers_ListAnchoredToMerchantOrg_2274(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB, nsOrg := uuid.NewString(), uuid.NewString(), uuid.NewString()

	// 平台管理员判定走 IAM GetOrganization(is_primary)
	iam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t", "expires_in": 3600})
		case strings.HasPrefix(r.URL.Path, "/api/v1/organizations/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.NewString(), "is_primary": strings.HasSuffix(r.URL.Path, nsOrg)})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	defer iam.Close()
	services.SetIAMInternalURLForTesting(iam.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	merchantA := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantA, TenantID: shared, OrgID: orgA, Name: "MA-2274",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "MB-2274",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	adminUser := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: adminUser, IAMSub: adminUser, TenantID: orgA, OrgID: orgA,
		Name: "负责人A", Email: "a@example.test", Status: "active",
	}).Error)
	// 关键：写入侧锚 = 商户 org（#2250），不是共享 tenant
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: orgA, MerchantID: merchantA, UserID: adminUser, Role: "merchant_admin",
	}).Error)

	call := func(t *testing.T, tid, oid, role string) (int, map[string]interface{}) {
		t.Helper()
		actor := testutil.TestActor{TenantID: tid, OrgID: oid, UserID: uuid.NewString(), Role: role, SysPerm: -1}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		r.GET("/admin/merchants/:id/members", NewMerchantMemberHandler().ListMembers)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/merchants/"+merchantA+"/members", nil))
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 1) 平台管理员（tid=共享 tenant，oid=命名空间主组织）：父提交 total=0 → 修复 total=1
	code, resp := call(t, shared, nsOrg, "NAMESPACE_ADMIN")
	require.Equal(t, http.StatusOK, code, "%v", resp)
	data, _ := resp["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["total"], "平台管理员必须能看到负责人（锚=商户 org）")

	// 2) 商户 A 管理员（tid=orgA）
	code, resp = call(t, orgA, orgA, "OWNER")
	require.Equal(t, http.StatusOK, code)
	data, _ = resp["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["total"])

	// 3) 商户 B 管理员不得访问 A
	code, _ = call(t, orgB, orgB, "OWNER")
	assert.Equal(t, http.StatusForbidden, code)
}

// #2274 §3：AddMember 写入锚必须是商户 org（共享 tenant 下不得落共享值）。
func TestMerchantMembers_AddAnchoredToMerchantOrg_2274(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	iam := newMerchantMemberMockIAM(t)
	defer iam.Close()
	services.SetIAMInternalURLForTesting(iam.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	shared := uuid.NewString()
	orgA := uuid.NewString()
	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: shared, OrgID: orgA, Name: "MA-ADD",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER", SysPerm: -1}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/admin/merchants/:id/members", NewMerchantMemberHandler().AddMember)

	body := `{"new_users":[{"username":"newsadmin","name":"新负责人","email":"add@example.test","phone":"13800000001","role":"merchant_admin"}]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/merchants/"+merchantID+"/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, w.Code, w.Body.String())

	var m models.MerchantMember
	require.NoError(t, db.Where("merchant_id = ?", merchantID).Order("created_at desc").First(&m).Error)
	assert.Equal(t, orgA, m.TenantID, "merchant_members 锚必须是商户 org")
	var u models.User
	require.NoError(t, db.Where("email = ?", "add@example.test").First(&u).Error)
	assert.Equal(t, orgA, u.TenantID, "本地用户缓存锚必须是商户 org")
}
