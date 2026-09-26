package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2076：PublicList 档案照为空时回退 users.avatar_url（新建师傅照片落头像 → 列表可显示）
func TestPublicList2076_AvatarFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()

	userNoProfilePhoto := uuid.New().String() // 档案照空 + 头像有（令狐七形态）
	userWithProfilePhoto := uuid.New().String()

	require.NoError(t, db.Create(&models.User{
		ID: userNoProfilePhoto, IAMSub: userNoProfilePhoto, TenantID: tenantID, OrgID: tenantID,
		Username: "ta", Name: "只有头像", AvatarURL: "/uploads/media/avatar_a.webp", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: userWithProfilePhoto, IAMSub: userWithProfilePhoto, TenantID: tenantID, OrgID: tenantID,
		Username: "tb", Name: "有档案照", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{
		ID: uuid.New().String(), UserID: userNoProfilePhoto, TenantID: tenantID, Photo: "", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{
		ID: uuid.New().String(), UserID: userWithProfilePhoto, TenantID: tenantID,
		Photo: "/uploads/media/technician_x.webp", Status: "active",
	}).Error)

	h := NewTechnicianProfileHandler()
	r := gin.New()
	r.GET("/common/repair-technicians", h.PublicList)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/common/repair-technicians", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 2)

	byID := map[string]map[string]interface{}{}
	for _, row := range resp.Data.List {
		byID[row["technician_id"].(string)] = row
	}
	// 令狐七形态：档案照空 → avatar/avatar_thumb 均回退头像
	rowA := byID[userNoProfilePhoto]
	require.NotNil(t, rowA)
	assert.Equal(t, "/uploads/media/avatar_a.webp", rowA["avatar"], "档案照空须回退头像")
	assert.Equal(t, "/uploads/media/avatar_a.webp", rowA["avatar_thumb"], "头像无缩略图变体 → 用原图")
	// 档案照有 → 用档案照
	rowB := byID[userWithProfilePhoto]
	require.NotNil(t, rowB)
	assert.Equal(t, "/uploads/media/technician_x.webp", rowB["avatar"])
}
