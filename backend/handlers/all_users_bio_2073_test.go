package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2073 全员照片 + 富文本简介：users.bio / 头像端点 / personnel 字段

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestCreateUser2073_BioAllTypesAndDualWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	operatorID := uuid.New().String()
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: tenantID,
		Username: "op2073", Role: "admin", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S2073", Type: "normal"}).Error)

	srv := newIAMMock2068(t, uuid.New().String())
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Setenv("IAM_SECRET", testIAMSecret)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: operatorID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	// 网点员工 + bio（全类型）
	bio := "<p>客服 3 年</p>"
	code, resp := doCreate2068(t, r, map[string]interface{}{
		"name": "员工A", "phone": "13900001001", "site_id": siteID, "bio": bio,
	})
	require.Equal(t, http.StatusOK, code, resp)
	uidA := resp["data"].(map[string]interface{})["id"].(string)
	var ua models.User
	require.NoError(t, db.First(&ua, "id = ?", uidA).Error)
	assert.Equal(t, bio, ua.Bio, "网点员工 bio 落 users.bio")

	// 维修师傅 + bio → users.bio + technician_profiles.bio 双写
	code2, resp2 := doCreate2068(t, r, map[string]interface{}{
		"name": "师傅B", "phone": "13900001002", "user_type": "repair_technician", "bio": bio,
	})
	require.Equal(t, http.StatusOK, code2, resp2)
	uidB := resp2["data"].(map[string]interface{})["id"].(string)
	var ub models.User
	require.NoError(t, db.First(&ub, "id = ?", uidB).Error)
	assert.Equal(t, bio, ub.Bio, "师傅 bio 落 users.bio")
	var tp models.TechnicianProfile
	require.NoError(t, db.First(&tp, "user_id = ?", uidB).Error)
	assert.Equal(t, bio, tp.Bio, "师傅 bio 双写 profile.bio")
}

func TestAdminUploadUserAvatar2073(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, otherTid := uuid.New().String(), uuid.New().String()
	targetID := uuid.New().String()
	for _, u := range []struct{ id, tid string }{{targetID, tenantID}, {uuid.New().String(), otherTid}} {
		require.NoError(t, db.Create(&models.User{
			ID: u.id, IAMSub: u.id, TenantID: u.tid, OrgID: u.tid, Username: "u-" + u.id[:8], Status: "active",
		}).Error)
	}

	h := &UserStaffHandler{}
	mkRouter := func(actor testutil.TestActor) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		r.POST("/admin/users/:id/avatar", AdminUploadUserAvatar)
		return r
	}
	postAvatar := func(r *gin.Engine, uid string) *httptest.ResponseRecorder {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="a.png"`)
		hdr.Set("Content-Type", "image/png")
		fw, err := w.CreatePart(hdr)
		require.NoError(t, err)
		_, _ = fw.Write(tinyPNG(t))
		w.Close()
		req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid+"/avatar", body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// 本租户管理员 → 200 + avatar_url 落库
	r := mkRouter(testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"})
	rec := postAvatar(r, targetID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var u models.User
	require.NoError(t, db.First(&u, "id = ?", targetID).Error)
	require.NotNil(t, u.AvatarURL)
	assert.Contains(t, u.AvatarURL, "avatar_"+targetID)

	// 跨租户 → 403（#688）
	r2 := mkRouter(testutil.TestActor{TenantID: otherTid, OrgID: otherTid, UserID: uuid.New().String(), Role: "ADMIN"})
	rec2 := postAvatar(r2, targetID)
	assert.Equal(t, http.StatusForbidden, rec2.Code)
	_ = h
}

func TestPersonnel2073_ReturnsBioAndAvatar(t *testing.T) {
	r, _ := setupPersonnel2065(t)
	code, rows := personnelList2065Q(t, r, "actor=merchant")
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, rows)
	// 行结构含 bio/avatar 键（值可为空）
	for _, row := range rows {
		_, hasBio := row["bio"]
		_, hasAvatar := row["avatar"]
		assert.True(t, hasBio && hasAvatar, "personnel 行须含 bio/avatar 字段")
		break
	}
}

// #2075 回归：/upload 移至 userOptionalAuth 后，匿名请求仍须 401（#1681 保护不变）
func TestUpload2075_AnonymousRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/upload", HandleUpload) // 无任何鉴权中间件（模拟 Optional 放行匿名）
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "匿名上传必须 401")
}
