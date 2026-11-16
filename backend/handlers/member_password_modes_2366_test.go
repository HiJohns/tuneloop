package handlers

// #2366 D1/D6（F2）：群组「新增成员」密码模式 + 无邮箱标识。
// R1 无邮箱 + 手动密码；R2 无邮箱 + 自动生成；R3 无通道拒绝（且 password/auto_generate 互斥）；
// R4 老客户端兼容（仅 username/name/email → 邀请分支，不下发初始密码）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

// iamCapture 保存最近一次 IAM 创建用户请求体。
type iamCapture struct{ last map[string]interface{} }

// setupMemberCreate2366 建测试库（网点 + 商户级 actor）与 IAM mock。
// captured.last = 最近一次 IAM 创建用户请求体，用于断言 skip_activation/password/callback_url。
func setupMemberCreate2366(t *testing.T) (tenantID, siteID string, router *gin.Engine, db *gorm.DB, captured *iamCapture) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db = testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID = uuid.New().String()
	operatorID := uuid.New().String()
	// #2344 严格 schema：site_members/users 有 tenant FK，先建 tenants 行。
	testfixtures.SeedTenantFK(t, db, tenantID)
	siteID = uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2366m", Role: "admin", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S2366M", Status: "active",
	}).Error)

	captured = &iamCapture{}

	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2366m", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // CreateOrGetUser 先查列表
			writeJSON(w, map[string]interface{}{"data": []interface{}{}})
			return
		}
		body := map[string]interface{}{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured.last = body
		// CreateOrGetUser 期望顶层 user_id（data 包装只在解析失败时回退）。
		writeJSON(w, map[string]interface{}{"user_id": uuid.New().String(), "status": "active"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if bytes.Contains([]byte(r.URL.Path), []byte("role-templates")) {
			writeJSON(w, []map[string]string{
				{"id": "tmpl-2366-site-member", "code": "site_member"},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"id": "mock-ns-2366"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := NewSiteMemberHandler()
	router = gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/sites/:id/members", h.AddMember)
	return tenantID, siteID, router, db, captured
}

func postNewUsers2366(t *testing.T, r *gin.Engine, siteID string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/sites/"+siteID+"/members", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

func countUsers2366(t *testing.T, db *gorm.DB, tenantID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&n).Error)
	return n
}

func TestAddMemberNewUsersManualPassword_2366(t *testing.T) {
	tenantID, siteID, router, db, captured := setupMemberCreate2366(t)

	code, resp := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{
			{"username": "m1", "name": "成员1", "password": "Abcd1234"},
		},
	})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)
	require.EqualValues(t, 20100, resp["code"])

	data := resp["data"].(map[string]interface{})
	passwords, ok := data["initial_passwords"].([]interface{})
	require.True(t, ok, "应回显 initial_passwords：%v", data)
	require.Len(t, passwords, 1)
	pw := passwords[0].(map[string]interface{})
	assert.Equal(t, "m1", pw["identifier"], "identifier = 实际登录名")
	assert.Equal(t, "Abcd1234", pw["password"])

	added := data["directly_added"].([]interface{})
	require.Len(t, added, 1)
	userID := added[0].(map[string]interface{})["user_id"].(string)

	var u models.User
	require.NoError(t, db.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&u).Error)
	assert.Equal(t, "", u.Email, "无邮箱账号本地 email 为空")
	assert.Equal(t, "", u.Phone)

	// IAM 请求：手动密码直接激活（skip_activation=true），不带 callback_url。
	require.NotNil(t, captured.last, "应调用 IAM 创建用户")
	assert.Equal(t, "Abcd1234", captured.last["password"])
	assert.Equal(t, true, captured.last["skip_activation"])
	assert.Equal(t, "m1", captured.last["username"])
	_, hasCallback := captured.last["callback_url"]
	assert.False(t, hasCallback, "直接激活模式不得下发邀请回调")
}

func TestAddMemberNewUsersAutoGenerate_2366(t *testing.T) {
	tenantID, siteID, router, db, captured := setupMemberCreate2366(t)

	code, resp := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{
			{"username": "m2", "name": "成员2", "auto_generate": true, "force_password_change": true},
		},
	})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)

	data := resp["data"].(map[string]interface{})
	passwords := data["initial_passwords"].([]interface{})
	require.Len(t, passwords, 1)
	pw := passwords[0].(map[string]interface{})
	assert.Equal(t, "m2", pw["identifier"])
	generated, _ := pw["password"].(string)
	assert.Len(t, generated, 12, "自动生成密码为 12 位")
	assert.NoError(t, validatePassword(generated), "自动生成密码必须通过后端强度校验")
	assert.Equal(t, generated, captured.last["password"], "回显密码与 IAM 请求一致")
	assert.Equal(t, true, captured.last["force_password_change"])

	added := data["directly_added"].([]interface{})
	require.Len(t, added, 1)
	var u models.User
	require.NoError(t, db.Where("id = ?", added[0].(map[string]interface{})["user_id"].(string)).First(&u).Error)
	assert.Equal(t, "", u.Email)
	assert.EqualValues(t, 2, countUsers2366(t, db, tenantID), "操作员 + 新建成员（无邮箱）")
}

func TestAddMemberNewUsersRejectNoChannel_2366(t *testing.T) {
	tenantID, siteID, router, db, _ := setupMemberCreate2366(t)
	before := countUsers2366(t, db, tenantID)

	// 1) 无 username/email/password/auto_generate → 400（登录标识 + 投递通道都缺）
	code, resp := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{{"name": "无通道"}},
	})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.EqualValues(t, 40001, resp["code"])

	// 2) username 有、email 无、无密码模式 → 400（邀请无投递通道）
	code2, resp2 := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{{"username": "m3", "name": "无邮箱"}},
	})
	assert.Equal(t, http.StatusBadRequest, code2, "resp=%v", resp2)
	assert.EqualValues(t, 40001, resp2["code"])

	// 3) password 与 auto_generate 互斥 → 400
	code3, resp3 := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{
			{"username": "m4", "name": "互斥", "password": "Abcd1234", "auto_generate": true},
		},
	})
	assert.Equal(t, http.StatusBadRequest, code3, "resp=%v", resp3)
	assert.EqualValues(t, 40001, resp3["code"])

	assert.Equal(t, before, countUsers2366(t, db, tenantID), "被拒请求不得建号")
}

func TestAddMemberNewUsersLegacyCompat_2366(t *testing.T) {
	tenantID, siteID, router, db, captured := setupMemberCreate2366(t)

	// 老客户端：只传 username/name/email（无密码模式）→ 邀请分支，行为与基线一致。
	code, resp := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{
			{"username": "m5", "name": "老客户端", "email": "legacy2366@example.com"},
		},
	})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)

	data := resp["data"].(map[string]interface{})
	assert.Nil(t, data["initial_passwords"], "邀请分支不得下发初始密码")

	require.NotNil(t, captured.last)
	_, hasPwd := captured.last["password"]
	assert.False(t, hasPwd, "邀请分支不得携带密码")
	assert.Equal(t, false, captured.last["skip_activation"], "邀请分支保持 skip_activation=false")
	assert.NotEmpty(t, captured.last["callback_url"], "邀请分支下发回调 URL")
	assert.Equal(t, "m5", captured.last["username"])

	added := data["directly_added"].([]interface{})
	require.Len(t, added, 1)
	var u models.User
	require.NoError(t, db.Where("id = ? AND tenant_id = ?", added[0].(map[string]interface{})["user_id"].(string), tenantID).First(&u).Error)
	assert.Equal(t, "legacy2366@example.com", u.Email)

	// 老客户端变体：不传 username、只传 email → email 作为登录名（显式化，不再静默）。
	code2, resp2 := postNewUsers2366(t, router, siteID, map[string]interface{}{
		"new_users": []map[string]interface{}{
			{"name": "仅邮箱", "email": "emailonly2366@example.com"},
		},
	})
	require.Equal(t, http.StatusCreated, code2, "resp=%v", resp2)
	assert.Equal(t, "emailonly2366@example.com", captured.last["username"])
	assert.Equal(t, "emailonly2366@example.com", captured.last["email"])
}
