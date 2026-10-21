package handlers

// #2249（S1）：UpdateIAMUser 更新本地缓存不得按 users.tenant_id 过滤。
// 自注册/顾客行 tenant_id=零 UUID，在员工上下文（tid=商户 org）下更新会静默 0 行。

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

func TestIAMProxy_UpdateIAMUser_ZeroTenant_2249(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sub := uuid.New().String()
	localID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: localID, IAMSub: sub, Name: "旧名字",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Status:   "active",
	}).Error)

	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2249", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Setenv("IAM_SECRET", testIAMSecret)

	merchantOrg := uuid.New().String()
	h := &IAMProxyHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/iam/users/:id", h.UpdateIAMUser)

	body, _ := json.Marshal(map[string]interface{}{"name": "新名字"})
	req := httptest.NewRequest(http.MethodPost, "/api/iam/users/"+sub, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got models.User
	require.NoError(t, db.Where("id = ?", localID).First(&got).Error)
	assert.Equal(t, "新名字", got.Name, "零租户本地行在员工上下文下应被更新（#2249）")
}
