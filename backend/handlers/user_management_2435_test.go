package handlers

// #2435: PC user-management "has second-doc photo but type unset" filter.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

func TestUserManagement_SecondDocUnsetFilter_2435(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	tenantID := uuid.NewString()
	photoNoType := uuid.NewString()
	photoWithType := uuid.NewString()
	noPhoto := uuid.NewString()
	for _, u := range []models.User{
		{ID: photoNoType, IAMSub: photoNoType, TenantID: tenantID, OrgID: tenantID, Username: "sd1", IdPhotoOther: strPtr2435("k1"), Status: "active"},
		{ID: photoWithType, IAMSub: photoWithType, TenantID: tenantID, OrgID: tenantID, Username: "sd2", IdPhotoOther: strPtr2435("k2"), IdPhotoOtherType: strPtr2435("student"), Status: "active"},
		{ID: noPhoto, IAMSub: noPhoto, TenantID: tenantID, OrgID: tenantID, Username: "sd3", Status: "active"},
	} {
		require.NoError(t, db.Create(&u).Error)
	}

	h := &UserManagementHandler{}
	r := gin.New()
	r.GET("/admin/user-management", h.List)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/user-management?second_doc_type=unset&pageSize=50", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	ids := []string{}
	for _, row := range resp.Data.List {
		if id, ok := row["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	assert.Contains(t, ids, photoNoType)
	assert.NotContains(t, ids, photoWithType)
	assert.NotContains(t, ids, noPhoto)
}

func strPtr2435(s string) *string { return &s }
