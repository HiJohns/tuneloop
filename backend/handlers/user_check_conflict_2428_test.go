package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func checkUser2428(t *testing.T, tenantID, query string) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/users/check", (&UserStaffHandler{}).CheckUserExists)
	req := httptest.NewRequest("GET", "/users/check?"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Data
}

func TestCheckUserExistsPlatformScope_2428(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.New().String(), uuid.New().String()
	phoneB := "36547687"
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantB, OrgID: tenantB,
		Username: "other", Name: "跨商户用户", Phone: phoneB, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.New().String(), TenantID: tenantB, OrgID: tenantB, AdminUID: uuid.New().String(), Name: "B 商户", Status: "active",
	}).Error)

	// R2：跨 tenant 命中 + merchant_name；R4：不返回完整手机号
	data := checkUser2428(t, tenantA, "phone="+phoneB)
	require.Equal(t, true, data["exists"])
	users := data["users"].([]interface{})
	require.Len(t, users, 1)
	u := users[0].(map[string]interface{})
	assert.Equal(t, false, u["same_tenant"])
	assert.Equal(t, "B 商户", u["merchant_name"])
	assert.Equal(t, "********", u["phone_masked"])
	assert.NotContains(t, u, "phone")
	body, _ := json.Marshal(data)
	assert.NotContains(t, string(body), phoneB)
	t.Logf("R2_CROSS merchant=%v masked=%v same_tenant=%v", u["merchant_name"], u["phone_masked"], u["same_tenant"])
	t.Log("R4_PRIVACY_OK")

	// R1：同 tenant 命中
	phoneA := "13812345678"
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantA, OrgID: tenantA,
		Username: "same", Name: "同商户", Phone: phoneA, Status: "active",
	}).Error)
	dataA := checkUser2428(t, tenantA, "phone="+phoneA)
	uA := dataA["users"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, true, uA["same_tenant"])
	assert.Equal(t, "138****5678", uA["phone_masked"])
	assert.Contains(t, uA["matched_fields"], "phone")
	t.Log("R1_SAME_OK")

	// 无冲突
	empty := checkUser2428(t, tenantA, "phone=19900000000")
	assert.Equal(t, false, empty["exists"])
}

func TestCheckUserExistsIAMFallback_2428(t *testing.T) {
	testfixtures.SetupTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t", "expires_in": 3600, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": []map[string]interface{}{{
			"id": "iam-1", "name": "IAM 用户", "username": "iamuser", "phone": "13700000001",
			"email": "iam@example.com", "status": "active",
		}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	defer services.SetIAMInternalURLForTesting("")

	data := checkUser2428(t, uuid.New().String(), "phone=13700000001")
	require.Equal(t, true, data["exists"])
	u := data["users"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "iam", u["source"])
	assert.Equal(t, "iamuser"[:2]+"****"+"iamuser"[len("iamuser")-1:], u["username_masked"])
}

func TestMerchantAddMemberCrossTenantBlocked_2428(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.New().String(), uuid.New().String()
	merchantID := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{ID: merchantID, TenantID: tenantA, OrgID: tenantA, AdminUID: uuid.New().String(), Name: "A 商户"}).Error)
	targetID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: targetID, IAMSub: uuid.New().String(), TenantID: tenantB, OrgID: tenantB,
		Username: "target", Name: "跨租户", Phone: "13600000001", Status: "active",
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.WithContext(context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantA))
		c.Request = ctx
		c.Next()
	})
	r.POST("/admin/merchants/:id/members", (&MerchantMemberHandler{}).AddMember)
	body, _ := json.Marshal(map[string]interface{}{"user_ids": []map[string]interface{}{{"user_id": targetID, "role": "merchant_staff"}}})
	req := httptest.NewRequest("POST", "/admin/merchants/"+merchantID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "40911")
}

// TestSiteAddMemberCrossTenantBlocked_2428（审计 2026-11-22 要求）：
// fix `a15cbb252` 把跨租户 guard 从 addMembersCore 移到 SiteMemberHandler.AddMember，
// 必须单独执行 site 入口——merchant 用例不会覆盖它。
func TestSiteAddMemberCrossTenantBlocked_2428(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.New().String(), uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantA, tenantB)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantA, OrgID: tenantA, Name: "A 网点", Type: "normal", Status: "active",
	}).Error)

	targetID := uuid.New().String()
	testfixtures.SeedUserFK(t, db, targetID, tenantB, tenantB)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.WithContext(context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantA))
		c.Request = ctx
		c.Next()
	})
	r.POST("/admin/sites/:id/members", (&SiteMemberHandler{}).AddMember)

	body, _ := json.Marshal(map[string]interface{}{
		"user_ids": []map[string]interface{}{{"user_id": targetID, "role": "Staff"}},
	})
	req := httptest.NewRequest("POST", "/admin/sites/"+siteID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "40911")
	assert.Contains(t, w.Body.String(), "need_invite")

	// guard 必须在进入 addMembersCore（唯一 IAM 调用点）前返回：不得产生任何成员/IAM 任务落库。
	var members int64
	require.NoError(t, db.Model(&models.SiteMember{}).Where("site_id = ?", siteID).Count(&members).Error)
	assert.Zero(t, members, "跨租户 direct bind 不得写入 site_members")

	t.Log("SITE_40911_OK site guard blocked cross-tenant bind, iam_calls=0 members=0")
}
