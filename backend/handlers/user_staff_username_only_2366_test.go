package handlers

// #2366 D3（F5）：后台创建只有 username 必填；phone/email 可选；
// username 缺失/全空白 → 400；空 phone/email 不触发唯一性冲突。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func setupCreate2366(t *testing.T) (tenantID, operatorID string, r *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID = uuid.New().String()
	operatorID = uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2366", Role: "admin", Status: "active",
	}).Error)

	h := &UserStaffHandler{}
	r = gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)
	return tenantID, operatorID, r
}

func withIAMMock2366(t *testing.T) {
	t.Helper()
	// 每次 IAM 创建返回不同的 user_id（R3 需要连续创建两个用户）。
	srv := newIAMMock2366Server(t)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
}

func TestCreateUserUsernameOnly_2366(t *testing.T) {
	tenantID, _, r := setupCreate2366(t)
	withIAMMock2366(t)
	db := testfixtures.GetTestDB(t)

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "师傅A", "username": "tech_a",
		"auto_generate": true, "user_type": "repair_technician",
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)

	var user models.User
	require.NoError(t, db.Where("tenant_id = ? AND username = ?", tenantID, "tech_a").First(&user).Error)
	assert.Equal(t, "师傅A", user.Name)
	assert.Equal(t, "", user.Phone)
	assert.Equal(t, "", user.Email)

	var profiles int64
	require.NoError(t, db.Model(&models.TechnicianProfile{}).Where("user_id = ?", user.ID).Count(&profiles).Error)
	assert.Equal(t, int64(1), profiles, "technician profile must be created in the same flow")
}

func TestCreateUserMissingUsername_2366(t *testing.T) {
	tenantID, _, r := setupCreate2366(t)
	withIAMMock2366(t)
	db := testfixtures.GetTestDB(t)

	var before int64
	require.NoError(t, db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&before).Error)

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "无用户名", "auto_generate": true, "user_type": "repair_technician",
	})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.EqualValues(t, 40001, resp["code"])

	var after int64
	require.NoError(t, db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&after).Error)
	assert.Equal(t, before, after, "no user row must be created when username is missing")
}

func TestCreateUserBlankUsername_2366(t *testing.T) {
	_, _, r := setupCreate2366(t)
	withIAMMock2366(t)

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "空白用户名", "username": "   ", "auto_generate": true,
	})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestCreateUserEmptyContactNoConflict_2366(t *testing.T) {
	tenantID, _, r := setupCreate2366(t)
	withIAMMock2366(t)
	db := testfixtures.GetTestDB(t)

	code1, resp1 := doCreate2068(t, r, map[string]interface{}{
		"name": "无联系方式A", "username": "u_a", "auto_generate": true, "user_type": "repair_technician",
	})
	require.Equal(t, http.StatusOK, code1, "resp1=%v", resp1)

	code2, resp2 := doCreate2068(t, r, map[string]interface{}{
		"name": "无联系方式B", "username": "u_b", "auto_generate": true, "user_type": "repair_technician",
	})
	require.Equal(t, http.StatusOK, code2, "resp2=%v", resp2)

	var users []models.User
	require.NoError(t, db.Where("tenant_id = ? AND username IN ?", tenantID, []string{"u_a", "u_b"}).Find(&users).Error)
	require.Len(t, users, 2)
	for _, u := range users {
		assert.Equal(t, "", u.Phone)
		assert.Equal(t, "", u.Email)
	}
}

// newIAMMock2366Server 每次 /users 调用返回递增的 user_id。
func newIAMMock2366Server(t *testing.T) *httptest.Server {
	t.Helper()
	var seq int64
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2366", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		id := uuid.MustParse("00000000-0000-4000-8000-0000000000" + twoDigits(atomic.AddInt64(&seq, 1)))
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"user_id": id.String(), "status": "active"}})
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
	return httptest.NewServer(mux)
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
