package handlers

// #2247：平台管理员不得创建商户员工/维修师；维修师档案锚定商户 org；
// 关联既有用户时必须 active 且属于本商户。

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

func mockIAM2247(t *testing.T, orgID string, primary bool) *httptest.Server {
	t.Helper()
	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"access_token": "t2247", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"id": orgID, "is_primary": primary, "status": "active"})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"data": map[string]interface{}{"user_id": uuid.New().String(), "status": "pending"}})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"data": map[string]interface{}{}})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"data": []interface{}{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	return srv
}

func TestCreateUser_PlatformAdminRejected_2247(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	opOrg := uuid.New().String()
	mockIAM2247(t, opOrg, true)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: opOrg, OrgID: opOrg, UserID: uuid.NewString(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "平台越权", "phone": "13900009941", "email": "platform2247@example.com",
		"user_type": "repair_technician", "site_id": uuid.New().String(),
	})
	assert.Equal(t, http.StatusForbidden, code, "resp=%v", resp)
	assert.Equal(t, float64(40310), resp["code"], "resp=%v", resp)
	_ = db
}

func TestCreateUser_MerchantAdminProfileAnchored_2247(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	merchantOrg := uuid.New().String()
	merchantTenant := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		TenantID: merchantTenant, OrgID: merchantOrg, Name: "M2247",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.New().String(),
	}).Error)
	mockIAM2247(t, merchantOrg, false)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "OWNER"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "商户技师", "phone": "13900009942", "email": "merchant2247@example.com",
		"user_type": "repair_technician",
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data, _ := resp["data"].(map[string]interface{})
	localID, _ := data["id"].(string)
	require.NotEmpty(t, localID)

	var tp models.TechnicianProfile
	require.NoError(t, db.Where("user_id = ?", localID).First(&tp).Error)
	assert.Equal(t, merchantOrg, tp.TenantID, "档案应锚定商户 org")
}

func TestTechnicianProfileCreate_InactiveUserRejected_2247(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	merchantOrg := uuid.New().String()
	merchantTenant := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		TenantID: merchantTenant, OrgID: merchantOrg, Name: "M2247b",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.New().String(),
	}).Error)
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.New().String(), Name: "已删", Phone: "13900009943",
		TenantID: merchantOrg, OrgID: merchantOrg, Status: "deleted",
	}).Error)
	mockIAM2247(t, merchantOrg, false)

	h := &TechnicianProfileHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "OWNER"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/technician-profiles", h.Create)

	body, _ := json.Marshal(map[string]interface{}{"user_id": userID})
	req := httptest.NewRequest(http.MethodPost, "/technician-profiles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "不可用")
}
