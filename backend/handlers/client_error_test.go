package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetClientErrorLimiterForTest() {
	clientErrorLimiter.mu.Lock()
	clientErrorLimiter.seen = make(map[string]time.Time)
	clientErrorLimiter.mu.Unlock()
}

func postClientError(t *testing.T, router *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/client-error", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func newClientErrorRouter(actor *testutil.TestActor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if actor != nil {
		a := *actor
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(a.InjectContext(c.Request.Context()))
			c.Next()
		})
	}
	r.POST("/client-error", ClientError)
	return r
}

func TestClientError_RecordsAuditLog(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()

	tenantID := "11111111-1111-1111-1111-111111111111"
	userID := "22222222-2222-2222-2222-222222222222"
	r := newClientErrorRouter(&testutil.TestActor{TenantID: tenantID, UserID: userID})

	body := `{"message":"TypeError: t is not a function","stack":"at Checkout (Checkout.jsx:521)","route":"/checkout?id=x","version":"1.0.78","user_agent":"smoke-agent"}`
	rec := postClientError(t, r, body)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String(), "不得回显堆栈")

	var row models.AuditLog
	require.NoError(t, db.Where("action = ?", "client_error").Order("created_at desc").First(&row).Error)
	assert.Equal(t, tenantID, row.TenantID)
	assert.Equal(t, userID, row.UserID)
	assert.Equal(t, "frontend", row.ResourceType)
	assert.Equal(t, "error", row.Status)
	assert.Equal(t, http.StatusInternalServerError, row.StatusCode)
	assert.Equal(t, "smoke-agent", row.UserAgent)
	require.NotNil(t, row.Details)
	var details map[string]string
	require.NoError(t, json.Unmarshal([]byte(*row.Details), &details))
	assert.Equal(t, "/checkout?id=x", details["route"])
	assert.Equal(t, "1.0.78", details["version"])
	assert.Contains(t, details["message"], "t is not a function")
}

func TestClientError_AnonymousUsesZeroUUID(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()

	r := newClientErrorRouter(nil)
	rec := postClientError(t, r, `{"message":"anonymous boom"}`)
	require.Equal(t, http.StatusNoContent, rec.Code)

	var row models.AuditLog
	require.NoError(t, db.Where("action = ?", "client_error").Order("created_at desc").First(&row).Error)
	assert.Equal(t, clientErrorZeroUUID, row.TenantID)
	assert.Equal(t, clientErrorZeroUUID, row.UserID)
}

func TestClientError_RejectsOversizeStack(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()

	r := newClientErrorRouter(nil)
	payload := map[string]string{"message": "big stack", "stack": strings.Repeat("x", clientErrorMaxStack+1)}
	b, _ := json.Marshal(payload)
	rec := postClientError(t, r, string(b))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var count int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ?", "client_error").Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestClientError_RejectsMissingMessage(t *testing.T) {
	testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()
	r := newClientErrorRouter(nil)
	rec := postClientError(t, r, `{"route":"/x"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestClientError_RateLimitsSameIPAndMessage(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()

	r := newClientErrorRouter(nil)
	body := `{"message":"rate limited boom"}`
	first := postClientError(t, r, body)
	second := postClientError(t, r, body)
	assert.Equal(t, http.StatusNoContent, first.Code)
	assert.Equal(t, http.StatusNoContent, second.Code)

	var count int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ? AND resource_id = ?", "client_error", "").Count(&count).Error)
	assert.EqualValues(t, 1, count, "同一 ip+message 一分钟内仅落库 1 条")
}

func TestClientError_RedactsSensitiveValues(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	resetClientErrorLimiterForTest()

	r := newClientErrorRouter(nil)
	body := `{"message":"login failed token=SuperSecret123 phone=13800138000 id_card=110101199003071234","token":"top-secret","phone":"13800138000"}`
	rec := postClientError(t, r, body)
	require.Equal(t, http.StatusNoContent, rec.Code)

	var row models.AuditLog
	require.NoError(t, db.Where("action = ?", "client_error").Order("created_at desc").First(&row).Error)
	require.NotNil(t, row.Details)
	stored := *row.Details
	assert.NotContains(t, stored, "SuperSecret123")
	assert.NotContains(t, stored, "13800138000")
	assert.NotContains(t, stored, "110101199003071234")
	assert.NotContains(t, stored, "top-secret")
	assert.Contains(t, stored, "[redacted]")
}
