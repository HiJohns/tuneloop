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

	// Case C: second doc uploaded, registration batch pending, no second_doc
	// batch → still requeue (independent review)
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

	// Dry run: cases A/C/E are candidates; nothing persisted
	count, err := RequeueSecondDocBatches(true)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	var total int64
	db.Model(&models.FaceCaptureBatch{}).Where("kind = ? AND status = ?", "second_doc", "pending").Count(&total)
	assert.Equal(t, int64(1), total, "dry run must not create batches")

	// Real run: batches created for A/C/E
	count, err = RequeueSecondDocBatches(false)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	for _, uid := range []string{approvedUser.ID, inReviewUser.ID, rejectedUser.ID} {
		var n int64
		require.NoError(t, db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ? AND status = ?", uid, "second_doc", "pending").Count(&n).Error)
		assert.EqualValues(t, 1, n)
	}

	// Idempotency: second run is a no-op
	count, err = RequeueSecondDocBatches(false)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	db.Model(&models.FaceCaptureBatch{}).Where("user_id = ? AND kind = ?", approvedUser.ID, "second_doc").Count(&total)
	assert.Equal(t, int64(1), total)
}
