package handlers

// #2389：无邮箱账号重设密码必须显式 400，而不是 skipped + 200「假成功」。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetPasswordNoEmail_2389(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		Username: "noemail2389", Name: "无邮箱用户", Email: "", Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/users/reset-password", (&UserStaffHandler{}).ResetPassword)

	body, _ := json.Marshal(map[string]interface{}{"user_ids": []string{userID}})
	req := httptest.NewRequest(http.MethodPost, "/users/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 40002, resp["code"])
	assert.Contains(t, resp["message"], "未配置邮箱")
}
