package handlers

import (
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

// RequeueSecondDocBatches (#1921/#2312) creates pending `second_doc` review
// batches for users who uploaded a second document (id_photo_other) but are
// still unverified — those users have no independent second-doc review and
// cannot reach deposit-free eligibility. Idempotent: users with a pending or
// approved `second_doc` batch are skipped, so repeated runs never duplicate.
func RequeueSecondDocBatches(dryRun bool) (int, error) {
	db := database.GetDB()

	var users []models.User
	if err := db.
		Where("id_photo_other IS NOT NULL AND id_photo_other != ''").
		Where("id_photo_other_verified = ?", false).
		// #2435 D3：已清退（inactive）/已删除用户不再入队；历史队列不自动撤销。
		Where("status <> ?", "inactive").
		Where("deleted_at IS NULL").
		Find(&users).Error; err != nil {
		return 0, err
	}

	count := 0
	for _, user := range users {
		// #2435 R4：唯一索引 uq_face_capture_batches_user_pending 只允许
		// 同一用户 1 个 pending 批次（不分 kind）。已有 pending（如 registration）
		// 时先跳过，待其审结后再由 approve 路径/下次 requeue 补建，避免 23505 中断整批。
		var pending int64
		if err := db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND status = ?", user.ID, "pending").
			Count(&pending).Error; err != nil {
			return count, err
		}
		if pending > 0 {
			continue
		}
		// Idempotency: an open/approved second-doc review already exists.
		var open int64
		if err := db.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND kind = ? AND status IN ?", user.ID, "second_doc", []string{"pending", "approved"}).
			Count(&open).Error; err != nil {
			return count, err
		}
		if open > 0 {
			continue
		}

		log.Printf("[RequeueSecondDoc] user %s (%s): second doc unverified and no open second_doc batch → requeue",
			user.ID, user.Username)
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
