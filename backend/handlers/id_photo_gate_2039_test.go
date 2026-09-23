package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2039: 第二证件（other）以身份证正反面已上传为前提（后端权威）。
func TestUploadIDPhoto_OtherRequiresFrontBack(t *testing.T) {
	_, tenantID, userID := setupIdPhotoTestDB(t)
	actor := testutil.MakeCustomer(tenantID, userID)
	router := idPhotoRouter(actor)

	post := func(side string) *httptest.ResponseRecorder {
		body, ctype := uploadForm(side)
		req := httptest.NewRequest("POST", "/api/user/id-photo", body)
		req.Header.Set("Content-Type", ctype)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	// 无 front/back → other 被拒 40902（含 reasons.no_id_photo）
	r := post("other")
	require.Equal(t, http.StatusForbidden, r.Code)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &out))
	assert.Equal(t, float64(40902), out["code"])

	// 传 front + back 后 → other 允许
	require.Equal(t, http.StatusOK, post("front").Code)
	require.Equal(t, http.StatusOK, post("back").Code)
	require.Equal(t, http.StatusOK, post("other").Code)
}
