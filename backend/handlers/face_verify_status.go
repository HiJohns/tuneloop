package handlers

import (
	"log"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// 实名核身五态（#1787/#1789 T1）
const (
	IdVerifyStatusNone          = "none"           // 未上传证件照
	IdVerifyStatusUploaded      = "uploaded"       // 已上传证件照，未提交自拍/未比对
	IdVerifyStatusPendingReview = "pending_review" // 自拍已提交，待人工审核
	IdVerifyStatusVerified      = "verified"       // 已核身（tencent 自动比对或 manual 人工审核）
	IdVerifyStatusRejected      = "rejected"       // 审核驳回（可重新采集）
)

// deriveIdVerifyStatus (#1789 T1): 派生用户核身五态。
// 判定优先级（R2 C6）：
//  1. 三张证件照（front/back/other）均空 → none（优先于批次判定）
//  2. face_verified=true → verified（直接返回，不查批次——tencent 通道同步通过）
//  3. face_verified=false → 查最新批次（submitted_at DESC LIMIT 1）：
//     pending → pending_review / rejected → rejected / 无批次 → uploaded
//
// 容错（R2 H4）：批次表不存在/为空 → 按 uploaded/none 处理（不 panic、不报错）。
func deriveIdVerifyStatus(db *gorm.DB, user *models.User) string {
	if user == nil {
		return IdVerifyStatusNone
	}
	// 1. 证件照三张均空 → none。
	if (user.IdPhotoFront == nil || *user.IdPhotoFront == "") &&
		(user.IdPhotoBack == nil || *user.IdPhotoBack == "") &&
		(user.IdPhotoOther == nil || *user.IdPhotoOther == "") {
		return IdVerifyStatusNone
	}
	// 2. face_verified → verified。
	if user.FaceVerified {
		return IdVerifyStatusVerified
	}
	// 3. 查最新批次。
	var batch models.FaceCaptureBatch
	if err := db.Where("user_id = ?", user.ID).
		Order("submitted_at DESC").First(&batch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 无批次 → uploaded（已上传证件照未提交自拍）。
			return IdVerifyStatusUploaded
		}
		// 表不存在/查询失败 → 容错按 uploaded（不 panic、不报错，R2 H4）。
		log.Printf("[deriveIdVerifyStatus] batch query failed for user %s: %v", user.ID, err)
		return IdVerifyStatusUploaded
	}
	switch batch.Status {
	case "pending":
		return IdVerifyStatusPendingReview
	case "rejected":
		return IdVerifyStatusRejected
	case "approved":
		// approved 但 face_verified=false（异常态）→ 按 uploaded 处理。
		return IdVerifyStatusUploaded
	default:
		return IdVerifyStatusUploaded
	}
}

// deriveIdVerifyStatusBulk (#1791 T3 R2 H6): 批量场景的派生版本——
// 最新批次状态由调用方预加载（一次 IN 查询），避免逐行查批次表（N+1）。
// 判定优先级与 deriveIdVerifyStatus 一致（证件照全空 → none 优先；
// face_verified → verified；否则按 latestBatchStatus）。
func deriveIdVerifyStatusBulk(user *models.User, latestBatchStatus string) string {
	if user == nil {
		return IdVerifyStatusNone
	}
	if (user.IdPhotoFront == nil || *user.IdPhotoFront == "") &&
		(user.IdPhotoBack == nil || *user.IdPhotoBack == "") &&
		(user.IdPhotoOther == nil || *user.IdPhotoOther == "") {
		return IdVerifyStatusNone
	}
	if user.FaceVerified {
		return IdVerifyStatusVerified
	}
	switch latestBatchStatus {
	case "pending":
		return IdVerifyStatusPendingReview
	case "rejected":
		return IdVerifyStatusRejected
	default:
		return IdVerifyStatusUploaded
	}
}
