package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// #1790 T2 tests: 自拍采集存储（提交/状态/幂等）+ GC 豁免。

func faceCaptureRouter(t *testing.T, iamSub string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, iamSub)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &FaceCaptureHandler{}
	router.POST("/user/face-capture", h.SubmitFaceCapture)
	router.GET("/user/face-capture/status", h.GetFaceCaptureStatus)
	return router
}

// multipartBody 构造带 image（+可选 video）的 multipart body。
func multipartBody(t *testing.T, withVideo bool) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("image", "selfie.jpg")
	require.NoError(t, err)
	fw.Write([]byte("fake-jpeg-bytes"))
	if withVideo {
		vw, err := w.CreateFormFile("video", "selfie.mp4")
		require.NoError(t, err)
		vw.Write([]byte("fake-mp4-bytes"))
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

// setupFaceCaptureUser 创建用户 + 返回 iam_sub 和 db。
func setupFaceCaptureUser(t *testing.T) (string, *gorm.DB) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	iamSub := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: iamSub,
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "facecap-" + uuid.NewString()[:6], Status: "active",
	}).Error)
	return iamSub, db
}

// TestFaceCapture_Submit_StoresBatchAndFile (#1790): 提交 → 批次 pending +
// 文件落盘（face_captures/{userID}/{batchID}/）+ media_assets 注册。
func TestFaceCapture_Submit_StoresBatchAndFile(t *testing.T) {
	iamSub, db := setupFaceCaptureUser(t)
	router := faceCaptureRouter(t, iamSub)

	body, contentType := multipartBody(t, false)
	req := httptest.NewRequest("POST", "/user/face-capture", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			BatchID string `json:"batch_id"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "pending", resp.Data.Status)

	// 批次落库。
	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", resp.Data.BatchID).First(&batch).Error)
	require.Equal(t, "pending", batch.Status)

	// media_assets 注册（source_type=face_capture, source_id=batch_id）。
	var asset models.MediaAsset
	require.NoError(t, db.Where("source_id = ? AND source_type = ?", resp.Data.BatchID, "face_capture").First(&asset).Error)
	require.Contains(t, asset.StorageKey, "face_captures/")

	// 文件落盘。
	fullPath := filepath.Join("./uploads/media", asset.StorageKey)
	_, err := os.Stat(fullPath)
	require.NoError(t, err, "file persisted on disk")
}

// TestFaceCapture_Status (#1790): GET status → 最新批次 + 五态派生。
func TestFaceCapture_Status(t *testing.T) {
	iamSub, db := setupFaceCaptureUser(t)

	// 设置证件照（五态派生需要——证件照全空 → none 优先于批次判定）。
	var localUser models.User
	require.NoError(t, db.Where("iam_sub = ?", iamSub).First(&localUser).Error)
	require.NoError(t, db.Model(&localUser).Update("id_photo_front", "/uploads/media/front.jpg").Error)

	// 创建 pending 批次。
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: localUser.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	router := faceCaptureRouter(t, iamSub)
	req := httptest.NewRequest("GET", "/user/face-capture/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			IdVerifyStatus string `json:"id_verify_status"`
			LatestBatch    struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"latest_batch"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "pending_review", resp.Data.IdVerifyStatus)
	require.Equal(t, "pending", resp.Data.LatestBatch.Status)
}

// TestFaceCapture_Resubmit_InvalidatesOldPending (#1790 R2 M3): 重复提交 →
// 旧 pending 作废（rejected），仅新批次 pending 存活。
func TestFaceCapture_Resubmit_InvalidatesOldPending(t *testing.T) {
	iamSub, db := setupFaceCaptureUser(t)
	router := faceCaptureRouter(t, iamSub)

	submit := func() string {
		body, contentType := multipartBody(t, false)
		req := httptest.NewRequest("POST", "/user/face-capture", body)
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				BatchID string `json:"batch_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data.BatchID
	}

	batch1 := submit()
	batch2 := submit()

	var oldBatch, newBatch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", batch1).First(&oldBatch).Error)
	require.NoError(t, db.Where("id = ?", batch2).First(&newBatch).Error)
	require.Equal(t, "rejected", oldBatch.Status, "old pending invalidated")
	require.NotNil(t, oldBatch.RejectReason)
	require.Equal(t, "已重新提交", *oldBatch.RejectReason)
	require.Equal(t, "pending", newBatch.Status, "new batch pending")

	// 仅一个 pending 存活。
	var pendingCount int64
	db.Model(&models.FaceCaptureBatch{}).Where("user_id = ? AND status = ?", oldBatch.UserID, "pending").Count(&pendingCount)
	require.Equal(t, int64(1), pendingCount)
}
