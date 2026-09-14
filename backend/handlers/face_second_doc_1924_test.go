package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1924: second-document review batches — reviewer assigns the type; the
// registration review must also assign it when the customer submitted a
// second document.

func setupSecondDocFixture(t *testing.T, kind string, faceVerified bool) (string, string, *gorm.DB) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "second-" + uuid.NewString()[:6], Status: "active", Name: "第二证件用户",
		IdPhotoFront:         strPtr("/uploads/media/front.jpg"),
		IdPhotoBack:          strPtr("/uploads/media/back.jpg"),
		IdPhotoOther:         strPtr("/uploads/media/other.jpg"),
		FaceVerified:         faceVerified,
		IdPhotoOtherVerified: false,
	}
	require.NoError(t, db.Create(&user).Error)
	batch := models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending", Kind: kind,
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&batch).Error)
	return user.ID, batch.ID, db
}

func postReview(t *testing.T, router *gin.Engine, batchID string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestFaceReview_SecondDoc_ApproveAssignsType(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "second_doc", true)
	router := faceReviewRouter(t, uuid.New().String())

	w := postReview(t, router, batchID, map[string]interface{}{
		"action": "approve", "second_doc_type": "student",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, db.First(&user, "id = ?", userID).Error)
	assert.True(t, user.IdPhotoOtherVerified, "second document must be marked verified")
	require.NotNil(t, user.IdPhotoOtherType)
	assert.Equal(t, "student", *user.IdPhotoOtherType)
	assert.True(t, user.FaceVerified, "second-doc review must not touch face_verified")

	var batch models.FaceCaptureBatch
	require.NoError(t, db.First(&batch, "id = ?", batchID).Error)
	assert.Equal(t, "approved", batch.Status)
}

func TestFaceReview_SecondDoc_RequiresType(t *testing.T) {
	_, batchID, _ := setupSecondDocFixture(t, "second_doc", true)
	router := faceReviewRouter(t, uuid.New().String())

	w := postReview(t, router, batchID, map[string]interface{}{"action": "approve"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "second_doc_type")
}

func TestFaceReview_Registration_RequiresTypeWhenSecondDocPresent(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "registration", false)
	router := faceReviewRouter(t, uuid.New().String())

	idFields := map[string]interface{}{
		"action": "approve", "real_name": "张三", "id_card_no": "110101199001011234",
		"id_card_expire": "2035-12-31", "id_card_authority": "北京市公安局", "id_card_address": "北京市",
	}
	w := postReview(t, router, batchID, idFields)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "second_doc_type")

	idFields["second_doc_type"] = "teacher"
	w = postReview(t, router, batchID, idFields)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, db.First(&user, "id = ?", userID).Error)
	assert.True(t, user.FaceVerified)
	assert.True(t, user.IdPhotoOtherVerified)
	require.NotNil(t, user.IdPhotoOtherType)
	assert.Equal(t, "teacher", *user.IdPhotoOtherType)
}
