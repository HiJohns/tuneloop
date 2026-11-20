package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1921: legacy users with an uploaded but untyped second document must be
// requeued for staff review (type assignment + verified flag).

func TestRequeueSecondDocBatches(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// Case A: second doc uploaded, type unset, latest batch approved → requeue
	approvedUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-a", Status: "active",
		IdPhotoOther: strPtr("/uploads/media/other-a.jpg"),
	}
	require.NoError(t, db.Create(&approvedUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: approvedUser.ID, Status: "approved",
		Kind: "registration", SubmittedAt: time.Now().Add(-time.Hour), CreatedAt: time.Now().Add(-time.Hour),
	}).Error)

	// Case B: second doc uploaded but a pending second_doc batch already
	// exists → skip (idempotency)
	pendingUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-b", Status: "active",
		IdPhotoOther: strPtr("/uploads/media/other-b.jpg"),
	}
	require.NoError(t, db.Create(&pendingUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: pendingUser.ID, Status: "approved",
		Kind: "registration", SubmittedAt: time.Now().Add(-time.Hour), CreatedAt: time.Now().Add(-time.Hour),
	}).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: pendingUser.ID, Status: "pending",
		Kind: "second_doc", SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	// Case C: second doc uploaded, registration batch pending → skip.
	// uq_face_capture_batches_user_pending 只允许 1 个 pending（不分 kind）；
	// 待 registration 审结后由 approve 路径补建 second_doc（#2435 R4）。
	inReviewUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-c", Status: "active",
		IdPhotoOther: strPtr("/uploads/media/other-c.jpg"),
	}
	require.NoError(t, db.Create(&inReviewUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: inReviewUser.ID, Status: "pending",
		Kind: "registration", SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	// Case D: second doc typed already (reviewer assigned) → skip
	typedUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-d", Status: "active",
		IdPhotoOther:         strPtr("/uploads/media/other-d.jpg"),
		IdPhotoOtherType:     strPtr("student"),
		IdPhotoOtherVerified: true,
	}
	require.NoError(t, db.Create(&typedUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: typedUser.ID, Status: "approved",
		Kind: "registration", SubmittedAt: time.Now().Add(-time.Hour), CreatedAt: time.Now().Add(-time.Hour),
	}).Error)

	// Case E: latest registration batch rejected, no second_doc batch → requeue
	rejectedUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-e", Status: "active",
		IdPhotoOther: strPtr("/uploads/media/other-e.jpg"),
	}
	require.NoError(t, db.Create(&rejectedUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: rejectedUser.ID, Status: "rejected",
		Kind: "registration", SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	// Case G: type set but still unverified → requeue (verified=false is the gate)
	typedUnverifiedUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-g", Status: "active",
		IdPhotoOther:     strPtr("/uploads/media/other-g.jpg"),
		IdPhotoOtherType: strPtr("student"),
	}
	require.NoError(t, db.Create(&typedUnverifiedUser).Error)

	// Case F: no second doc at all → skip
	plainUser := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "requeue-f", Status: "active",
	}
	require.NoError(t, db.Create(&plainUser).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: plainUser.ID, Status: "approved",
		Kind: "registration", SubmittedAt: time.Now().Add(-time.Hour), CreatedAt: time.Now().Add(-time.Hour),
	}).Error)

	// Dry run: cases A/E/G are candidates; C is blocked by its pending registration batch
	count, err := RequeueSecondDocBatches(true)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	var total int64
	db.Model(&models.FaceCaptureBatch{}).Where("kind = ? AND status = ?", "second_doc", "pending").Count(&total)
	assert.Equal(t, int64(1), total, "dry run must not create batches")

	// Real run: batches created for A/E/G only
	count, err = RequeueSecondDocBatches(false)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	for _, uid := range []string{approvedUser.ID, rejectedUser.ID, typedUnverifiedUser.ID} {
		var n int64
		require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ? AND status = ?", uid, "second_doc", "pending").Count(&n).Error)
		assert.EqualValues(t, 1, n)
	}
	var blockedPending int64
	require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
		Where("user_id = ? AND kind = ?", inReviewUser.ID, "second_doc").Count(&blockedPending).Error)
	assert.Zero(t, blockedPending, "pending registration batch must not trigger a second pending batch")

	// Idempotency: second run is a no-op
	count, err = RequeueSecondDocBatches(false)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	db.Model(&models.FaceCaptureBatch{}).Where("user_id = ? AND kind = ?", approvedUser.ID, "second_doc").Count(&total)
	assert.Equal(t, int64(1), total)
}

// #2435 R4：已清退（inactive）与软删除用户不得进入 requeue；active 用户不受影响。
func TestRequeueSecondDocBatches_SkipsInactiveAndDeleted_2435(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	mkUser := func(name, status string, deleted bool) models.User {
		u := models.User{
			ID: uuid.New().String(), IAMSub: uuid.New().String(),
			TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			Username:     name,
			Status:       status,
			IdPhotoOther: strPtr("/uploads/media/other-" + name + ".jpg"),
		}
		if deleted {
			now := time.Now()
			u.DeletedAt = &now
		}
		require.NoError(t, db.Create(&u).Error)
		return u
	}

	active := mkUser("requeue-active-2435", "active", false)
	inactive := mkUser("requeue-inactive-2435", "inactive", false)
	deleted := mkUser("requeue-deleted-2435", "active", true)
	pendingReg := mkUser("requeue-pending-reg-2435", "active", false)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: pendingReg.ID, Status: "pending", Kind: "registration",
	}).Error)

	count, err := RequeueSecondDocBatches(true)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "dry-run only sees the active user")

	count, err = RequeueSecondDocBatches(false)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	for _, tc := range []struct {
		uid  string
		want int64
	}{
		{active.ID, 1},
		{inactive.ID, 0},
		{deleted.ID, 0},
		{pendingReg.ID, 0},
	} {
		var n int64
		require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ?", tc.uid, "second_doc").Count(&n).Error)
		assert.EqualValues(t, tc.want, n, "user %s", tc.uid)
	}
}
