package handlers

// #2250：商户归属锚 = org。merchant 的 tenant_id 为共享值时，
// 商户管理员（tid=商户 org）仍应能访问并成功添加成员；成员行落商户 org。

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

func TestAddMember_SharedTenant_MerchantOrgAnchor_2250(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sharedTenant := uuid.New().String()
	merchantOrg := uuid.New().String()
	merchantID := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: sharedTenant, OrgID: merchantOrg, Name: "M2250",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.New().String(),
	}).Error)

	iamSub := uuid.New().String()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, Name: "待加成员", Phone: "13900009950",
		TenantID: merchantOrg, OrgID: merchantOrg, Status: "active",
	}).Error)

	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"access_token": "t2250", "expires_in": 7200, "token_type": "Bearer"})
	})
	for _, p := range []string{"/api/v1/users/", "/api/v1/organizations/", "/api/v1/namespaces/"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			write(w, map[string]interface{}{"data": map[string]interface{}{}})
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &MerchantMemberHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: merchantOrg, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "OWNER"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/admin/merchants/:id/members", h.AddMember)

	body, _ := json.Marshal(map[string]interface{}{"user_id": userID, "role": "merchant_staff"})
	req := httptest.NewRequest(http.MethodPost, "/admin/merchants/"+merchantID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, w.Code, w.Body.String())

	var mm models.MerchantMember
	require.NoError(t, db.Where("merchant_id = ? AND user_id = ?", merchantID, userID).First(&mm).Error)
	assert.Equal(t, merchantOrg, mm.TenantID, "成员归属应锚商户 org（#2250）")
}
