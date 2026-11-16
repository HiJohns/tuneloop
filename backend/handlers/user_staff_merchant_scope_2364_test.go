package handlers

// #2367（父任务 #2364，子任务 A）：CreateUser 的商户归属解析改用 merchantConfigDB。
//
// 缺陷形态（预生产 2026-11-16 15:14 实测）：商户行 merchants.org_id=商户 org、
// merchants.tenant_id=命名空间 org；管理员 JWT tid=商户 org。resolveOperatorMerchant2247
// 收到带自动 tenant scope 的句柄时，org 分支被追加 `AND tenant_id = <JWT tid>` → 恒 0 行；
// CreateUser 的 merchant_direct 事务守卫随后返回 500
// `failed to create user: merchant not found for operator scope`。
//
// R1：修复后同一 fixture 返回 200，users + merchant_members 各 1 行，IAM bind 收到商户 org。
// R2：pre-fix 根因守卫（带 scope 句柄解析失败）+ 真实 handler 在不可解析时的 500 文案。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	merchantScope2364Username = "merchant_direct_2364"
	unresolved2364Username    = "merchant_direct_2364_unresolved"
)

// iamBindCall2367 记录 IAM bind 请求，用于断言「bind 到商户 org 而不是命名空间 tenant」。
type iamBindCall2367 struct {
	Action string
	Role   string
	OrgID  string
	UserID string
}

type iamRecorder2367 struct {
	mu    sync.Mutex
	binds []iamBindCall2367
}

func (r *iamRecorder2367) record(call iamBindCall2367) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.binds = append(r.binds, call)
}

func (r *iamRecorder2367) snapshot() []iamBindCall2367 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]iamBindCall2367, len(r.binds))
	copy(out, r.binds)
	return out
}

// newIAMMock2367 提供最小 IAM 契约：创建用户 / 绑定组织 / 组织查询 / 角色模板 / perm-version。
func newIAMMock2367(t *testing.T, iamUserID string, rec *iamRecorder2367) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2367", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": iamUserID, "status": "active"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": iamUserID}})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bind") {
			var body struct {
				Action string `json:"action"`
				Role   string `json:"role"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			// /api/v1/organizations/{org}/users/{user}/bind
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/"), "/")
			if len(parts) >= 4 {
				rec.record(iamBindCall2367{Action: body.Action, Role: body.Role, OrgID: parts[0], UserID: parts[2]})
			}
			writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
			return
		}
		// GetOrganization：缺 id → IAMClient 返回 error → isPlatformOrg2247=false（非平台管理员）
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []interface{}{})
	})
	mux.HandleFunc("/api/v1/perm-version/increment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000})
	})

	return httptest.NewServer(mux)
}

// setupMerchantScope2367 构造现场数据形态：merchants.tenant_id=命名空间 org ≠ JWT tid=商户 org。
func setupMerchantScope2367(t *testing.T) (db *gorm.DB, merchantOrg, namespaceOrg, merchantID string, iamRec *iamRecorder2367) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db = testfixtures.SetupTestDB(t)
	merchantOrg = uuid.NewString()
	namespaceOrg = uuid.NewString()
	operatorID := uuid.NewString()

	merchant := models.Merchant{
		ID:           uuid.NewString(),
		TenantID:     namespaceOrg,
		OrgID:        merchantOrg,
		Name:         "M2364A",
		Code:         "M2364A-" + merchantOrg[:8],
		Address:      "x",
		ContactName:  "c",
		ContactPhone: "1",
		Status:       "active",
		AdminUID:     operatorID,
	}
	require.NoError(t, db.Create(&merchant).Error)

	iamRec = &iamRecorder2367{}
	srv := newIAMMock2367(t, uuid.NewString(), iamRec)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	return db, merchantOrg, namespaceOrg, merchant.ID, iamRec
}

func router2367(actor testutil.TestActor) *gin.Engine {
	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)
	return r
}

// TestCreateMerchantDirectMerchantScope_2364 是 R1：修复后创建商户直属员工不再 500。
func TestCreateMerchantDirectMerchantScope_2364(t *testing.T) {
	db, merchantOrg, namespaceOrg, merchantID, iamRec := setupMerchantScope2367(t)

	// JWT 现场形态：tid=商户 org，oid=商户 org
	r := router2367(testutil.TestActor{
		TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "ADMIN",
	})

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name":          "商户直属员工",
		"username":      merchantScope2364Username,
		"auto_generate": true,
		"user_type":     "merchant_direct",
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)

	data, _ := resp["data"].(map[string]interface{})
	require.NotEmpty(t, data["id"], "响应须含新建用户 id")

	var users []models.User
	require.NoError(t, db.Where("username = ?", merchantScope2364Username).Find(&users).Error)
	require.Len(t, users, 1, "users 必须恰好落库 1 行")
	assert.Equal(t, merchantOrg, users[0].TenantID)
	assert.Equal(t, merchantOrg, users[0].OrgID)

	var members []models.MerchantMember
	require.NoError(t, db.Where("user_id = ?", users[0].ID).Find(&members).Error)
	require.Len(t, members, 1, "merchant_members 必须恰好落库 1 行")
	assert.Equal(t, merchantOrg, members[0].TenantID)
	assert.Equal(t, merchantID, members[0].MerchantID)
	assert.NotEqual(t, namespaceOrg, members[0].TenantID, "商户员工归属不得回落到命名空间 tenant")

	binds := iamRec.snapshot()
	require.Len(t, binds, 1, "IAM bind 必须恰好调用 1 次")
	assert.Equal(t, "bind", binds[0].Action)
	assert.Equal(t, "STAFF", binds[0].Role, "merchant_staff → IAM STAFF")
	assert.Equal(t, merchantOrg, binds[0].OrgID, "bind 目标必须是商户 org")
	assert.Equal(t, users[0].IAMSub, binds[0].UserID)
}

// TestResolveMerchantScopePreFixRed_2364 是 R2：
// 1) pre-fix 的带自动 tenant scope 句柄必须解析失败（缺陷根因可复现）；
// 2) 修复口径（merchantConfigDB）可解析；
// 3) 真实 handler 在「确实无法解析」时返回 500 且文案含 merchant not found for operator scope。
func TestResolveMerchantScopePreFixRed_2364(t *testing.T) {
	db, merchantOrg, namespaceOrg, _, _ := setupMerchantScope2367(t)

	// 现场数据形态核对
	var m models.Merchant
	require.NoError(t, db.Where("org_id = ?", merchantOrg).First(&m).Error)
	require.Equal(t, namespaceOrg, m.TenantID)
	require.NotEqual(t, merchantOrg, m.TenantID)

	ctx := testutil.TestActor{
		TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "ADMIN",
	}.InjectContext(context.Background())

	// 1) pre-fix：自动 tenant scope 追加 tenant_id=JWT tid → org 分支恒 0 行
	_, ok := resolveOperatorMerchant2247(database.GetDB().WithContext(ctx), merchantOrg, merchantOrg)
	assert.False(t, ok, "pre-fix 带自动 tenant scope 的句柄必须解析失败（缺陷根因）")

	// 2) 修复口径：IdentityCtx 跳过自动 scope → 命中商户 org
	fixed, ok := resolveOperatorMerchant2247(merchantConfigDB(ctx), merchantOrg, merchantOrg)
	require.True(t, ok, "merchantConfigDB 必须能解析到商户")
	assert.Equal(t, merchantOrg, fixed.OrgID)

	// 3) 共享 tenant 多商户 → 按 D2 不解析；真实 handler 必须 500 + 既有文案（不是 50000 之外的静默）
	require.NoError(t, db.Create(&models.Merchant{
		ID:           uuid.NewString(),
		TenantID:     namespaceOrg,
		OrgID:        uuid.NewString(),
		Name:         "M2364B",
		Code:         "M2364B-" + uuid.NewString()[:8],
		Address:      "x",
		ContactName:  "c",
		ContactPhone: "1",
		Status:       "active",
		AdminUID:     uuid.NewString(),
	}).Error)

	r := router2367(testutil.TestActor{
		TenantID: namespaceOrg, OrgID: uuid.NewString(), UserID: uuid.NewString(), Role: "ADMIN",
	})
	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name":          "无法解析商户",
		"username":      unresolved2364Username,
		"auto_generate": true,
		"user_type":     "merchant_direct",
	})
	require.Equal(t, http.StatusInternalServerError, code, "resp=%v", resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "merchant not found for operator scope")

	var n int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", unresolved2364Username).Count(&n).Error)
	assert.EqualValues(t, 0, n, "事务守卫失败必须回滚本地用户行")
}
