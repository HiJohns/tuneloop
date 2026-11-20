package handlers

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/models"
)

// #2435：实名审核通过时，若用户已上传第二证件但类型未定，
// 必须补建独立的 second_doc 复审批次（幂等）；类型不得由实名审核直接写入
// （#1924/#2311 裁定语义不变）。
func TestFaceReview_Registration_AutoEnqueuesSecondDoc_2435(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "registration", false)
	router := faceReviewRouter(t, uuid.New().String())

	body := map[string]interface{}{
		"action": "approve", "real_name": "张三", "id_card_no": "110101199001011234",
		"id_card_expire": "2035-12-31", "id_card_authority": "北京市公安局", "id_card_address": "北京市",
	}
	w := postReview(t, router, batchID, body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, db.First(&user, "id = ?", userID).Error)
	// 语义不变：实名审核不写第二证件类型/认证态
	assert.Nil(t, user.IdPhotoOtherType, "registration approve must not assign second-doc type")
	assert.False(t, user.IdPhotoOtherVerified)

	// 新增：补建 pending second_doc 批次
	var n int64
	require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
		Where("user_id = ? AND kind = ? AND status = ?", userID, "second_doc", "pending").
		Count(&n).Error)
	assert.Equal(t, int64(1), n, "registration approve must enqueue one pending second_doc batch")

	// 幂等：再次调用 helper 不新增
	ensureSecondDocBatch(db, userID)
	require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
		Where("user_id = ? AND kind = ? AND status = ?", userID, "second_doc", "pending").
		Count(&n).Error)
	assert.Equal(t, int64(1), n, "second_doc batch creation must be idempotent")
}

// 负例：第二证件已核定（verified）→ 不再补建批次。
func TestFaceReview_Registration_NoRequeueWhenSecondDocVerified_2435(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "registration", false)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"id_photo_other_verified": true}).Error)
	router := faceReviewRouter(t, uuid.New().String())

	w := postReview(t, router, batchID, map[string]interface{}{
		"action": "approve", "real_name": "张三", "id_card_no": "110101199001011234",
		"id_card_expire": "2035-12-31", "id_card_authority": "北京市公安局", "id_card_address": "北京市",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var n int64
	require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
		Where("user_id = ? AND kind = ?", userID, "second_doc").Count(&n).Error)
	assert.Equal(t, int64(0), n, "verified second doc must not be re-queued")
}

// #2435 R5：已有 rejected second_doc 批次时，实名通过可补建 1 个新 pending；
// open（pending/approved）最多 1 个，重复调用不新增。
func TestFaceReview_Registration_RejectedBatchAllowsOneNewOpen_2435(t *testing.T) {
	userID, batchID, db := setupSecondDocFixture(t, "registration", false)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: userID, Status: "rejected", Kind: "second_doc",
	}).Error) // GORM requires timestamps? FaceCaptureBatch CreatedAt may be zero; acceptable for test.

	router := faceReviewRouter(t, uuid.New().String())
	w := postReview(t, router, batchID, map[string]interface{}{
		"action": "approve", "real_name": "张三", "id_card_no": "110101199001011234",
		"id_card_expire": "2035-12-31", "id_card_authority": "北京市公安局", "id_card_address": "北京市",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	countSecondDoc := func() (total, open int64) {
		require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ?", userID, "second_doc").Count(&total).Error)
		require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ? AND status IN ?", userID, "second_doc", []string{"pending", "approved"}).
			Count(&open).Error)
		return total, open
	}
	total, open := countSecondDoc()
	assert.EqualValues(t, 2, total, "rejected + new pending")
	assert.EqualValues(t, 1, open, "at most one open second_doc batch")

	ensureSecondDocBatch(db, userID)
	total, open = countSecondDoc()
	assert.EqualValues(t, 2, total, "helper must be idempotent")
	assert.EqualValues(t, 1, open)
}
