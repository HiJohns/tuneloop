package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2117: /users/me 的 tenant_name 在员工上下文以 JWT tid 为准——
// 零租户自注册户（user.TenantID=00000000）切换到员工上下文后，
// 旧实现按本地 tenant_id 查商户恒空 → 身份徽标缺失。
func TestGetCurrentUser_TenantNameByJWTTid_2117(t *testing.T) {
	db, _, userID := setupIdPhotoTestDB(t)
	// 零租户自注册户形态（#2078）
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"tenant_id": "00000000-0000-0000-0000-000000000000"}).Error)

	// 员工上下文对应的商户（JWT tid 指向）
	staffTenant := uuid.New().String()
	_ = db.Migrator().DropTable(&models.Merchant{})
	require.NoError(t, db.Migrator().CreateTable(&models.Merchant{}))
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: staffTenant, OrgID: uuid.NewString(),
		Name: "卡丹萨", AdminUID: uuid.NewString(), Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: staffTenant, OrgID: uuid.NewString(), UserID: userID, Role: "STAFF"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/api/users/me", (&UserStaffHandler{}).GetCurrentUser)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Equal(t, "卡丹萨", resp.Data["tenant_name"], "员工上下文应按 JWT tid 返回商户名")
}

// 顾客上下文（tid 指向无商户的租户）不得泄漏/误配商户名。
func TestGetCurrentUser_TenantNameAbsentForCustomer_2117(t *testing.T) {
	db, tenantID, userID := setupIdPhotoTestDB(t)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"tenant_id": "00000000-0000-0000-0000-000000000000"}).Error)
	staffTenant := uuid.New().String()
	_ = db.Migrator().DropTable(&models.Merchant{})
	require.NoError(t, db.Migrator().CreateTable(&models.Merchant{}))
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: staffTenant, OrgID: uuid.NewString(),
		Name: "卡丹萨", AdminUID: uuid.NewString(), Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// 顾客上下文：tid 与商户无关联
		actor := testutil.TestActor{TenantID: tenantID, OrgID: "", UserID: userID, Role: "USER"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/api/users/me", (&UserStaffHandler{}).GetCurrentUser)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	_, has := resp.Data["tenant_name"]
	assert.False(t, has, "顾客上下文不应返回商户名")
}
