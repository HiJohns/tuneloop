package handlers

// #2276 审计修复：申诉写跨租户 404；平台管理员商户列表不受 operator org 限制。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepairAppeal_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	appeal := models.Appeal{ID: uuid.NewString(), TenantID: tenantA, OrgID: tenantA, SiteID: uuid.NewString(),
		Category: "damage", ObjectType: "order", ObjectID: uuid.NewString(), AppellantID: uuid.NewString(),
		Status: models.AppealStatusPending}
	require.NoError(t, db.Create(&appeal).Error)

	router := func(tid string, explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.POST("/repair-appeals/:id/close", CloseAppeal)
		return r
	}
	call := func(tid string, explicit bool) int {
		w := httptest.NewRecorder()
		router(tid, explicit).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/repair-appeals/"+appeal.ID+"/close", nil))
		return w.Code
	}

	// 跨租户：explicit on/off 均 404，状态不变
	assert.Equal(t, http.StatusNotFound, call(tenantB, true), "跨租户 explicit 必须 404")
	assert.Equal(t, http.StatusNotFound, call(tenantB, false), "on/off 等价：handler 显式过滤")
	var after models.Appeal
	require.NoError(t, db.Where("id = ?", appeal.ID).First(&after).Error)
	assert.Equal(t, models.AppealStatusPending, after.Status)

	// 本租户：200
	assert.Equal(t, http.StatusOK, call(tenantA, true))
	require.NoError(t, db.Where("id = ?", appeal.ID).First(&after).Error)
	assert.Equal(t, models.AppealStatusClosed, after.Status)
}

func TestMerchantList_PlatformAdmin_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	sharedTenant := uuid.NewString()
	m1 := models.Merchant{ID: uuid.NewString(), TenantID: sharedTenant, OrgID: uuid.NewString(), AdminUID: uuid.NewString(), Name: "M1", Status: "active"}
	m2 := models.Merchant{ID: uuid.NewString(), TenantID: sharedTenant, OrgID: uuid.NewString(), AdminUID: uuid.NewString(), Name: "M2", Status: "active"}
	require.NoError(t, db.Create(&m1).Error)
	require.NoError(t, db.Create(&m2).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: sharedTenant, OrgID: m1.OrgID, UserID: uuid.NewString(), Role: "NAMESPACE_ADMIN", SysPerm: -1}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, middleware.ExplicitTenantScope())
	r.GET("/merchants", NewMerchantHandler().ListMerchants)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants?page=1&pageSize=10", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int64                    `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data.List, 2, "平台管理员不得被 operator org 过滤为空")
}

func TestSiteInvite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	site := models.Site{ID: uuid.NewString(), TenantID: tenantA, OrgID: tenantA, Name: "S"}
	require.NoError(t, db.Create(&site).Error)

	router := func(tid string, explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.POST("/sites/:id/invites/send", SendStaffInvite)
		return r
	}
	call := func(tid string, explicit bool) int {
		body, _ := json.Marshal(map[string]interface{}{"identifier": "x@example.com", "role": "site_member"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/sites/"+site.ID+"/invites/send", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router(tid, explicit).ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusNotFound, call(tenantB, true))
	assert.Equal(t, http.StatusNotFound, call(tenantB, false), "on/off 等价")
}
