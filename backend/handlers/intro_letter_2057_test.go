package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2057 裁定3：介绍信仅在「学生证作为第二证件」时必需。

// uploadFormExtra 构造 multipart 表单，在 side 之外附加 extra 字段。
func uploadFormExtra(side string, extra map[string]string) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("side", side)
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="id_`+side+`.jpg"`)
	hdr.Set("Content-Type", "image/jpeg")
	fw, err := w.CreatePart(hdr)
	if err != nil {
		panic(err)
	}
	_, _ = fw.Write(smallValidJPG())
	w.Close()
	return body, w.FormDataContentType()
}

func postSecondDoc(t *testing.T, router *gin.Engine, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, ctype := uploadFormExtra("other", extra)
	req := httptest.NewRequest("POST", "/api/user/id-photo", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestIntroLetter_2057_StudentRequiresLetter(t *testing.T) {
	db, tenantID, userID := setupIdPhotoTestDB(t)
	// 第二证件以身份证正反面为前提（#2039）
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"id_photo_front": "front.jpg", "id_photo_back": "back.jpg"}).Error)
	router := idPhotoRouter(testutil.MakeCustomer(tenantID, userID))

	w := postSecondDoc(t, router, map[string]string{"second_doc_type": "student"})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"no_intro_letter"`)

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.Nil(t, u.IntroLetterURL, "缺介绍信时不得落库")
}

func TestIntroLetter_2057_StudentWithLetterPersists(t *testing.T) {
	db, tenantID, userID := setupIdPhotoTestDB(t)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"id_photo_front": "front.jpg", "id_photo_back": "back.jpg"}).Error)
	router := idPhotoRouter(testutil.MakeCustomer(tenantID, userID))

	w := postSecondDoc(t, router, map[string]string{
		"second_doc_type":  "student",
		"intro_letter_url": "id_photos/intro.jpg",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	require.NotNil(t, u.IntroLetterURL)
	assert.Equal(t, "id_photos/intro.jpg", *u.IntroLetterURL)
	require.NotNil(t, u.IdPhotoOtherType)
	assert.Equal(t, "student", *u.IdPhotoOtherType)
}

func TestIntroLetter_2057_NonStudentIgnoresLetter(t *testing.T) {
	db, tenantID, userID := setupIdPhotoTestDB(t)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"id_photo_front": "front.jpg", "id_photo_back": "back.jpg"}).Error)
	router := idPhotoRouter(testutil.MakeCustomer(tenantID, userID))

	// 非 student 无需介绍信；即便传了也应忽略（不落库/清空）。
	w := postSecondDoc(t, router, map[string]string{
		"second_doc_type":  "teacher",
		"intro_letter_url": "id_photos/should_be_ignored.jpg",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	if u.IntroLetterURL != nil {
		assert.Equal(t, "", *u.IntroLetterURL, "非 student 情形介绍信应被忽略/清空")
	}
	require.NotNil(t, u.IdPhotoOtherType)
	assert.Equal(t, "teacher", *u.IdPhotoOtherType)
}

func TestIntroLetter_2057_ReviewerStudentWithoutLetterRejected(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "second_doc", true)
	router := faceReviewRouter(t, uuid.New().String())

	w := postReview(t, router, batchID, map[string]interface{}{
		"action": "approve", "second_doc_type": "student",
	})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), `"no_intro_letter"`)

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.False(t, u.IdPhotoOtherVerified, "缺介绍信不得通过第二证件审核")
}

func TestIntroLetter_2057_ReviewerStudentWithLetterApproved(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "second_doc", true)
	router := faceReviewRouter(t, uuid.New().String())
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Update("intro_letter_url", "id_photos/intro.jpg").Error)

	w := postReview(t, router, batchID, map[string]interface{}{
		"action": "approve", "second_doc_type": "student",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.First(&u, "id = ?", userID).Error)
	assert.True(t, u.IdPhotoOtherVerified)
	require.NotNil(t, u.IdPhotoOtherType)
	assert.Equal(t, "student", *u.IdPhotoOtherType)
}
