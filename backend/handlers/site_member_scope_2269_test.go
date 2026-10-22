package handlers

// #2269：站点成员访问必须带商户维度（共享 tenant 下不得跨商户）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteMembers_CrossMerchantDenied_2269(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	siteA, siteB := uuid.NewString(), uuid.NewString()

	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "SM-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "SM-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: shared, OrgID: orgA, Name: "site-A", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: shared, OrgID: orgB, Name: "site-B", Status: "active"}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: shared, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewSiteMemberHandler()
	r.GET("/api/sites/:id/members", h.ListMembers)
	r.POST("/api/sites/:id/members", h.AddMember)

	// 本商户：可访问
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteA+"/members", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 跨商户：读取/写入均 404
	req = httptest.NewRequest(http.MethodGet, "/api/sites/"+siteB+"/members", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "跨商户成员列表必须 404")

	req = httptest.NewRequest(http.MethodPost, "/api/sites/"+siteB+"/members",
		strings.NewReader(`{"user_id":"`+uuid.NewString()+`","role":"Manager"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "跨商户添加成员必须 404")
}
