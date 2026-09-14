package handlers

import (
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RequeueSecondDocBatches (#1921) creates pending `second_doc` review batches
// for legacy users who uploaded a second document (id_photo_other) but have
// no reviewer-assigned type (id_photo_other_type empty) — those users are
// invisible to the review queue and cannot reach deposit-free eligibility.
// Only users whose latest face-capture batch is `approved` are requeued:
// pending batches are already visible to staff and rejected ones were
// explicitly declined. Idempotent: users with a pending `second_doc` batch
// are skipped, so repeated runs never duplicate batches.
func RequeueSecondDocBatches(dryRun bool) (int, error) {
	db := database.GetDB()

	var users []models.User
	if err := db.
		Where("id_photo_other IS NOT NULL AND id_photo_other != ''").
		Where("id_photo_other_type IS NULL OR id_photo_other_type = ''").
		Find(&users).Error; err != nil {
		return 0, err
	}

	count := 0
	for _, user := range users {
		// Idempotency: an open second-doc review already exists.
		var pending int64
		if err := db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ? AND status = ?", user.ID, "second_doc", "pending").
			Count(&pending).Error; err != nil {
			return count, err
		}
		if pending > 0 {
			continue
		}

		// Only requeue when the latest batch was approved (see doc comment).
		var latest models.FaceCaptureBatch
		if err := db.Where("user_id = ?", user.ID).
			Order("submitted_at DESC, created_at DESC").
			First(&latest).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue // never entered face review — out of scope
			}
			return count, err
		}
		if latest.Status != "approved" {
			continue
		}

		log.Printf("[RequeueSecondDoc] user %s (%s): latest batch %s approved, second doc untyped → requeue",
			user.ID, user.Username, latest.ID)
		if !dryRun {
			if err := db.Create(&models.FaceCaptureBatch{
				ID:          uuid.New().String(),
				UserID:      user.ID,
				Status:      "pending",
				Kind:        "second_doc",
				SubmittedAt: time.Now(),
				CreatedAt:   time.Now(),
			}).Error; err != nil {
				return count, err
			}
		}
		count++
	}

	if dryRun {
		log.Printf("[RequeueSecondDoc] dry run: %d users would get a second_doc review batch", count)
	} else {
		log.Printf("[RequeueSecondDoc] created %d second_doc review batches", count)
	}
	return count, nil
}
