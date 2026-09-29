package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// TestFaceVerify_Token_Result_Shape_2109：#2109 自动核身两端点契约补强——
// token：18 位证号校验（非法 40002）+ biz_token 回显 + 落库 real_name/id_card_no；
// result：passed=true → face_verified=true/method=tencent；passed=false → 不落库。
// （复用既有 fakeFaceProvider / faceRouter / setupFaceVerifyTestUser）
func TestFaceVerify_Token_Result_Shape_2109(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000301"
	userID := "00000000-0000-0000-0000-000000000302"
	orgID := "00000000-0000-0000-0000-000000000303"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	provider := &fakeFaceProvider{token: "11111111-2222-3333-4444-555555555555", passed: true, sim: 88.8}
	router := faceRouter(provider, userID, tenantID, orgID)

	post := func(path string, body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
		t.Helper()
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	// 非法证号 → 40002
	w, _ := post("/api/user/face-verify/token", map[string]interface{}{"name": "王纯羽", "id_card_no": "123"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// token 正常 → biz_token 回显 + 姓名/证号落库
	w, resp := post("/api/user/face-verify/token", map[string]interface{}{"name": "王纯羽", "id_card_no": "110101199001011234"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(20000), resp["code"])
	data, _ := resp["data"].(map[string]interface{})
	assert.Equal(t, provider.token, data["biz_token"])

	db := database.GetDB()
	var uu models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&uu).Error)
	require.NotNil(t, uu.RealName)
	assert.Equal(t, "王纯羽", *uu.RealName)
	require.NotNil(t, uu.IdCardNo)
	assert.Equal(t, "110101199001011234", *uu.IdCardNo)

	// result passed=true → face_verified=true + method=tencent
	w, resp = post("/api/user/face-verify/result", map[string]interface{}{"biz_token": provider.token})
	require.Equal(t, http.StatusOK, w.Code)
	rdata, _ := resp["data"].(map[string]interface{})
	assert.Equal(t, true, rdata["passed"])
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&uu).Error)
	assert.True(t, uu.FaceVerified)
	require.NotNil(t, uu.FaceVerifyMethod)
	assert.Equal(t, "tencent", *uu.FaceVerifyMethod)

	// passed=false → 不置 verified
	provider.passed = false
	require.NoError(t, db.Model(&models.User{}).Where("iam_sub = ?", userID).
		Updates(map[string]interface{}{"face_verified": false, "face_verify_method": nil}).Error)
	w, _ = post("/api/user/face-verify/result", map[string]interface{}{"biz_token": provider.token})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&uu).Error)
	assert.False(t, uu.FaceVerified, "passed=false 不得置 verified")

}
