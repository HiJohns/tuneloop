package handlers

// #2228 P2：邀请模式创建员工——不传密码/auto_generate 时走 IAM pending（邀请邮件），
// 响应不得下发初始密码；邮箱必填（邀请邮件投递通道，40002）。

import (
	"encoding/json"
	"io"
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
)

func TestStaffCreateUser_InviteMode_2228(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2228", Role: "admin", Status: "active",
	}).Error)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S2228"}).Error)

	var capturedCreate map[string]interface{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2228", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedCreate)
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": uuid.New().String(), "status": "pending"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	// 1) 邀请模式（无 auto_generate / password）→ 成功，无初始密码，IAM 请求不含密码/skip
	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "受邀员工", "phone": "13900002228", "email": "invitee2228@example.com",
		"site_id": siteID,
	})
	require.Equal(t, http.StatusOK, code, resp)
	data, _ := resp["data"].(map[string]interface{})
	assert.Empty(t, data["initial_password"], "邀请模式不得下发初始密码")
	require.NotNil(t, capturedCreate, "应调用 IAM 创建用户")
	_, hasPwd := capturedCreate["password"]
	assert.False(t, hasPwd, "IAM 请求不得携带密码（邀请模式）")
	assert.NotEqual(t, true, capturedCreate["skip_activation"], "邀请模式不得 skip_activation")

	var u models.User
	require.NoError(t, db.Where("id = ?", data["id"]).First(&u).Error)
	assert.Equal(t, "pending", u.Status, "预建账户在接受/激活前为 pending")

	// 2) 邀请模式缺邮箱 → 40002（邀请邮件通道必填）
	code2, resp2 := doCreate2068(t, r, map[string]interface{}{
		"name": "无邮箱", "phone": "13900002229", "site_id": siteID,
	})
	assert.Equal(t, http.StatusBadRequest, code2)
	assert.Contains(t, resp2["message"], "邮箱")

	// 3) 回归：auto_generate 模式仍可用（初始密码下发不变）
	code3, resp3 := doCreate2068(t, r, map[string]interface{}{
		"name": "直建员工", "phone": "13900002230", "email": "direct2228@example.com",
		"site_id": siteID, "auto_generate": true, "force_password_change": true,
	})
	require.Equal(t, http.StatusOK, code3, resp3)
	d3, _ := resp3["data"].(map[string]interface{})
	assert.NotEmpty(t, d3["initial_password"], "auto_generate 模式应保留初始密码契约")
}
