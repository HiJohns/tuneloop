package handlers

// #2402 D4：站点邀请路径 id 非 UUID → 400 40001（禁止拼进 uuid SQL 产生 22P02/404）。

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendStaffInviteInvalidSiteID_2402(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/sites/:id/invites/send", SendStaffInvite)

	req := httptest.NewRequest(http.MethodPost, "/sites/undefined/invites/send", bytes.NewBufferString(`{"identifier":"13900000003"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "40001")
}
