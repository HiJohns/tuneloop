package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// newMerchantMemberMockIAM serves the IAM endpoints AddMerchantMember needs:
// client-credentials token, user creation, org binding, role templates,
// role template assignment.
func newMerchantMemberMockIAM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user_id": uuid.New().String(),
				"status":  "active",
			})
			return
		}
		if r.Method == "GET" {
			// ListUsers — empty list so CreateOrGetUser proceeds to create
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id":   uuid.New().String(),
				"admin_id": uuid.New().String(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    20000,
			"message": "success",
		})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/role-templates"):
			// ListRoleTemplates
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": uuid.New().String(), "code": "site_member"},
				{"id": uuid.New().String(), "code": "merchant_admin"},
			})
		case r.Method == "GET":
			// getNamespaceID → GET /api/v1/namespaces/{ns}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": uuid.New().String(),
			})
		default:
			// AssignRoleTemplateToUser → POST to role-templates,
			// CreateOrganization → POST to /namespaces/{ns}/organizations
			if strings.HasSuffix(r.URL.Path, "/organizations") {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"org_id":   uuid.New().String(),
					"admin_id": uuid.New().String(),
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    20000,
				"message": "success",
			})
		}
	})
	return httptest.NewServer(mux)
}

func setupMerchantMemberRouter(t *testing.T, tenantID string) (*gin.Engine, *httptest.Server) {
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Merchant{}, &models.MerchantMember{}))

	mockIAM := newMerchantMemberMockIAM(t)
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "operator-1")
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, "ns-1")
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", tenantID)
		c.Set("user_id", "operator-1")
		c.Next()
	})

	handler := NewMerchantMemberHandler()
	router.GET("/admin/merchants/:id/members", handler.ListMembers)
	router.POST("/admin/merchants/:id/members", handler.AddMember)
	router.PUT("/admin/merchants/:id/members/:uid", handler.UpdateMemberRole)
	router.DELETE("/admin/merchants/:id/members/:uid", handler.RemoveMember)
	return router, mockIAM
}

func TestAddMerchantMember_FullFlow(t *testing.T) {
	tenantID := uuid.New().String()
	router, mockIAM := setupMerchantMemberRouter(t, tenantID)
	defer mockIAM.Close()
	db := database.GetDB()

	merchant := models.Merchant{
		ID:       uuid.New().String(),
		Name:     "测试商户",
		TenantID: tenantID,
		OrgID:    tenantID,
		AdminUID: uuid.New().String(),
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	body := map[string]interface{}{
		"new_users": []map[string]interface{}{
			{
				"name":     "张三",
				"email":    "test_staff@tuneloop.com",
				"phone":    "12345678",
				"role":     "site_member",
				"username": "test_staff",
			},
		},
		"skip_activation": true,
	}
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/admin/merchants/"+merchant.ID+"/members", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DirectlyAdded []struct {
				UserID string `json:"user_id"`
				Role   string `json:"role"`
			} `json:"directly_added"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20100, resp.Code)
	require.Len(t, resp.Data.DirectlyAdded, 1)
	require.Equal(t, "site_member", resp.Data.DirectlyAdded[0].Role)

	// Verify local merchant_members record
	var member models.MerchantMember
	require.NoError(t, db.Where("merchant_id = ? AND tenant_id = ?", merchant.ID, tenantID).First(&member).Error)
	require.Equal(t, "site_member", member.Role)

	// Verify local users record created
	var localUser models.User
	require.NoError(t, db.Where("id = ?", member.UserID).First(&localUser).Error)
	require.Equal(t, "test_staff@tuneloop.com", localUser.Email)
}

func TestListMerchantMembers(t *testing.T) {
	tenantID := uuid.New().String()
	router, mockIAM := setupMerchantMemberRouter(t, tenantID)
	defer mockIAM.Close()
	db := database.GetDB()

	merchant := models.Merchant{ID: uuid.New().String(), Name: "商户A", TenantID: tenantID, OrgID: tenantID, AdminUID: uuid.New().String(), Status: "active"}
	require.NoError(t, db.Create(&merchant).Error)

	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantID,
		OrgID: tenantID, Name: "李四", Email: "lisi@test.com", Role: "merchant_admin", Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: merchant.ID, UserID: user.ID, Role: "merchant_admin",
	}).Error)

	req := httptest.NewRequest("GET", "/admin/merchants/"+merchant.ID+"/members", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				UserID   string `json:"user_id"`
				UserName string `json:"user_name"`
				Role     string `json:"role"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1)
	require.Equal(t, "李四", resp.Data.List[0].UserName)
	require.Equal(t, "merchant_admin", resp.Data.List[0].Role)
}

func TestUpdateMerchantMemberRole(t *testing.T) {
	tenantID := uuid.New().String()
	router, mockIAM := setupMerchantMemberRouter(t, tenantID)
	defer mockIAM.Close()
	db := database.GetDB()

	merchant := models.Merchant{ID: uuid.New().String(), Name: "商户B", TenantID: tenantID, OrgID: tenantID, AdminUID: uuid.New().String(), Status: "active"}
	require.NoError(t, db.Create(&merchant).Error)

	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantID,
		OrgID: tenantID, Name: "王五", Email: "wangwu@test.com", Role: "site_member", Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: merchant.ID, UserID: user.ID, Role: "site_member",
	}).Error)

	reqBody, _ := json.Marshal(map[string]string{"role": "merchant_admin"})
	req := httptest.NewRequest("PUT", "/admin/merchants/"+merchant.ID+"/members/"+user.ID, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var member models.MerchantMember
	require.NoError(t, db.Where("merchant_id = ? AND user_id = ?", merchant.ID, user.ID).First(&member).Error)
	require.Equal(t, "merchant_admin", member.Role)
}

func TestRemoveMerchantMember(t *testing.T) {
	tenantID := uuid.New().String()
	router, mockIAM := setupMerchantMemberRouter(t, tenantID)
	defer mockIAM.Close()
	db := database.GetDB()

	merchant := models.Merchant{ID: uuid.New().String(), Name: "商户C", TenantID: tenantID, OrgID: tenantID, AdminUID: uuid.New().String(), Status: "active"}
	require.NoError(t, db.Create(&merchant).Error)

	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantID,
		OrgID: tenantID, Name: "赵六", Email: "zhaoliu@test.com", Role: "site_member", Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: merchant.ID, UserID: user.ID, Role: "site_member",
	}).Error)

	req := httptest.NewRequest("DELETE", "/admin/merchants/"+merchant.ID+"/members/"+user.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var count int64
	db.Model(&models.MerchantMember{}).Where("merchant_id = ? AND user_id = ?", merchant.ID, user.ID).Count(&count)
	require.Equal(t, int64(0), count)
}

func TestCreateMerchant_WithoutAdmin(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Merchant{}, &models.Site{}))

	tenantID := uuid.New().String()

	mockIAM := newMerchantMemberMockIAM(t)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "operator-1")
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", tenantID)
		c.Set("user_id", "operator-1")
		c.Next()
	})
	router.POST("/merchants", NewMerchantHandler().CreateMerchant)

	body := map[string]interface{}{
		"name":          "无管理员商户",
		"merchant_type": "full",
		"rebate_opt_in": true,
	}
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/merchants", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "merchant without admin should be created, body: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20100, resp.Code)
}

// TestRemoveLastMerchantAdmin verifies #1718: removing the last merchant_admin
// (负责人) is rejected server-side; removing a non-last admin succeeds.
func TestRemoveLastMerchantAdmin(t *testing.T) {
	tenantID := uuid.New().String()
	router, mockIAM := setupMerchantMemberRouter(t, tenantID)
	defer mockIAM.Close()
	db := database.GetDB()

	merchant := models.Merchant{ID: uuid.New().String(), Name: "商户D", TenantID: tenantID, OrgID: tenantID, AdminUID: uuid.New().String(), Status: "active"}
	require.NoError(t, db.Create(&merchant).Error)

	admin := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantID,
		OrgID: tenantID, Name: "唯一负责人", Role: "merchant_admin", Status: "active",
	}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: merchant.ID, UserID: admin.ID, Role: "merchant_admin",
	}).Error)

	// Removing the last admin must be rejected.
	req := httptest.NewRequest("DELETE", "/admin/merchants/"+merchant.ID+"/members/"+admin.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "不能删除最后一个负责人", resp.Message)

	var count int64
	db.Model(&models.MerchantMember{}).Where("merchant_id = ? AND user_id = ?", merchant.ID, admin.ID).Count(&count)
	require.Equal(t, int64(1), count, "last admin must not be removed")

	// Adding a second admin allows removing the first.
	admin2 := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tenantID,
		OrgID: tenantID, Name: "第二负责人", Role: "merchant_admin", Status: "active",
	}
	require.NoError(t, db.Create(&admin2).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: merchant.ID, UserID: admin2.ID, Role: "merchant_admin",
	}).Error)

	req2 := httptest.NewRequest("DELETE", "/admin/merchants/"+merchant.ID+"/members/"+admin.ID, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, "non-last admin may be removed")
}
