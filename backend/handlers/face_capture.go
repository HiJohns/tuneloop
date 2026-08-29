package handlers

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FaceCaptureHandler (#1790 T2): 自拍采集存储 + GC 豁免。
// 端点注册在 userOptionalAuth 组（顾客 USER 角色 tid/oid 为空，#833 教训）。
type FaceCaptureHandler struct{}

// faceCaptureResponse 提交成功后的响应。
type faceCaptureResponse struct {
	BatchID string `json:"batch_id"`
	Status  string `json:"status"`
}

// SubmitFaceCapture handles POST /user/face-capture.
// multipart: image（必选，自拍图）+ video（可选，自拍视频）。
// 存储：uploads/media/face_captures/{userID}/{batchID}/（GC 豁免，
// media_assets source_type=face_capture，source_id=batch_id —— #1790 M5 关联键）。
// 建 FaceCaptureBatch{status: pending}；旧 pending 批次事务内作废
// （rejected, reason=已重新提交）——并发幂等由部分唯一索引兜底（R2 M3）。
func (h *FaceCaptureHandler) SubmitFaceCapture(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40104, "message": "not logged in"})
		return
	}

	imageFile, imageHeader, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "image file is required"})
		return
	}
	defer imageFile.Close()

	// 校验图片类型。
	imageExt := strings.ToLower(filepath.Ext(imageHeader.Filename))
	if imageExt != ".jpg" && imageExt != ".jpeg" && imageExt != ".png" && imageExt != ".webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "unsupported image format, use jpg/png/webp"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	// 解析本地用户 ID（iam_sub → 本地 id，face_capture_batches.user_id 引用 users.id）。
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	batchID := uuid.New().String()
	dir := filepath.Join("./uploads/media", "face_captures", localUser.ID, batchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[FaceCapture] mkdir failed for %s: %v", dir, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to store selfie"})
		return
	}

	// 保存图片。
	imageKey := "face_captures/" + localUser.ID + "/" + batchID + "/selfie" + imageExt
	imageDst, err := os.Create(filepath.Join("./uploads/media", imageKey))
	if err != nil {
		log.Printf("[FaceCapture] create image failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to store selfie"})
		return
	}
	if _, err := io.Copy(imageDst, imageFile); err != nil {
		imageDst.Close()
		log.Printf("[FaceCapture] save image failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to store selfie"})
		return
	}
	imageDst.Close()

	// 可选视频。
	videoKey := ""
	if videoFile, videoHeader, err := c.Request.FormFile("video"); err == nil {
		defer videoFile.Close()
		videoExt := strings.ToLower(filepath.Ext(videoHeader.Filename))
		if videoExt == ".mp4" || videoExt == ".mov" || videoExt == ".webm" {
			videoKey = "face_captures/" + localUser.ID + "/" + batchID + "/selfie" + videoExt
			videoDst, err := os.Create(filepath.Join("./uploads/media", videoKey))
			if err == nil {
				if _, err := io.Copy(videoDst, videoFile); err == nil {
					videoDst.Close()
				} else {
					videoDst.Close()
					log.Printf("[FaceCapture] save video failed: %v", err)
					videoKey = ""
				}
			} else {
				log.Printf("[FaceCapture] create video failed: %v", err)
				videoKey = ""
			}
		}
	}

	now := time.Now()
	status := "pending"

	// 事务：建批次 + 作废旧 pending + 注册 media_assets。
	if err := db.Transaction(func(tx *gorm.DB) error {
		// 作废旧 pending 批次（R2 M3 幂等：同一时刻仅一个 pending 存活）。
		rejectReason := "已重新提交"
		if err := tx.Model(&models.FaceCaptureBatch{}).
			Where("user_id = ? AND status = ?", localUser.ID, "pending").
			Updates(map[string]interface{}{"status": "rejected", "reject_reason": rejectReason}).Error; err != nil {
			return err
		}
		batch := models.FaceCaptureBatch{
			ID:          batchID,
			UserID:      localUser.ID,
			Status:      status,
			SubmittedAt: now,
			CreatedAt:   now,
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("[FaceCapture] batch create failed for %s: %v", localUser.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create capture batch"})
		return
	}

	// 注册 media_assets（GC 豁免 source_type=face_capture）。
	if err := services.NewMediaRegistry().RegisterAsset(ctx, imageKey, services.SourceTypeFaceCapture, batchID, imageHeader.Size, "image"); err != nil {
		log.Printf("[FaceCapture] register image asset failed: %v", err)
	}
	if videoKey != "" {
		if err := services.NewMediaRegistry().RegisterAsset(ctx, videoKey, services.SourceTypeFaceCapture, batchID, 0, "video"); err != nil {
			log.Printf("[FaceCapture] register video asset failed: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": faceCaptureResponse{BatchID: batchID, Status: status},
	})
}

// GetFaceCaptureStatus handles GET /user/face-capture/status.
// 返回本人最新批次状态（submitted_at DESC LIMIT 1）。
func (h *FaceCaptureHandler) GetFaceCaptureStatus(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40104, "message": "not logged in"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	// 派生五态（复用 T1 派生函数）。
	idVerifyStatus := deriveIdVerifyStatus(db, &localUser)

	var batch models.FaceCaptureBatch
	batchFound := db.Where("user_id = ?", localUser.ID).
		Order("submitted_at DESC").First(&batch).Error == nil

	data := gin.H{
		"id_verify_status":   idVerifyStatus,
		"face_verify_method": localUser.FaceVerifyMethod,
	}
	if batchFound {
		data["latest_batch"] = gin.H{
			"id":            batch.ID,
			"status":        batch.Status,
			"reject_reason": batch.RejectReason,
			"submitted_at":  batch.SubmittedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": data})
}
