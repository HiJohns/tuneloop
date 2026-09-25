package handlers

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
)

// #2068 创建用户：user_type=repair_technician 合并建户（users + technician_profiles 单事务）
// + 唯一性冲突 candidates + 缺省类型回归 + 非法类型。

func newIAMMock2068(t *testing.T, newUserID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2068", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID, "status": "active"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": newUserID}})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
	})
	return httptest.NewServer(mux)
}

func doCreate2068(t *testing.T, r *gin.Engine, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestCreateUser2068_TechnicianMergedAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2068", Role: "admin", Status: "active",
	}).Error)

	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S2068", Type: "normal"}).Error)

	newUserID := uuid.New().String()
	srv := newIAMMock2068(t, newUserID)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	// 1) 师傅类型 → 建用户 + technician_profiles（bio 落库）+ 返回 profile id
	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "新师傅", "phone": "13900000001", "user_type": "repair_technician", "bio": "<p>钢琴 10 年</p>",
	})
	require.Equal(t, http.StatusOK, code, resp)
	data, _ := resp["data"].(map[string]interface{})
	require.NotEmpty(t, data["technician_profile_id"], "响应须含 technician_profile_id")
	var tp models.TechnicianProfile
	require.NoError(t, db.First(&tp, "user_id = ?", data["id"]).Error)
	assert.Equal(t, "<p>钢琴 10 年</p>", tp.Bio)
	assert.Equal(t, tenantID, tp.TenantID)
	assert.Equal(t, "active", tp.Status)

	// 2) 唯一性冲突 → 409 + candidates
	code2, resp2 := doCreate2068(t, r, map[string]interface{}{
		"name": "重复", "phone": "13900000001", "user_type": "site_staff",
	})
	require.Equal(t, http.StatusConflict, code2)
	cands, _ := resp2["data"].([]interface{})
	assert.NotEmpty(t, cands, "409 须返回冲突候选（candidates）")

	// 3) 缺省类型（site_staff）不建师傅档案（回归；网点员工须带 site_id）
	code3, resp3 := doCreate2068(t, r, map[string]interface{}{"name": "普通员工", "phone": "13900000002", "site_id": siteID})
	require.Equal(t, http.StatusOK, code3, resp3)
	d3, _ := resp3["data"].(map[string]interface{})
	assert.Empty(t, d3["technician_profile_id"])
	var n int64
	require.NoError(t, db.Model(&models.TechnicianProfile{}).Where("user_id = ?", d3["id"]).Count(&n).Error)
	assert.EqualValues(t, 0, n)

	// 4) 非法 user_type → 40001
	code4, _ := doCreate2068(t, r, map[string]interface{}{"name": "x", "phone": "13900000003", "user_type": "bogus"})
	assert.Equal(t, http.StatusBadRequest, code4)
}
