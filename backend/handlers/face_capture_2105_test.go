package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/models"
)

// TestFaceCapture_VideoSaveFail_NotSilent_2105：#2105 回归——视频临时目录
// 不可写（TMPDIR 只读）时，不再静默 200：响应显式携带 `video_saved:false`
// （红-绿：修复前响应无该字段 → 断言失败），且照片路径不受影响、批次照常创建。
func TestFaceCapture_VideoSaveFail_NotSilent_2105(t *testing.T) {
	iamSub, db := setupFaceCaptureUser(t)
	router := faceCaptureRouter(t, iamSub)

	// 只读 TMPDIR：MkdirTemp("") 必失败 → storeFaceVideo 返回错误
	roDir := t.TempDir()
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
	t.Setenv("TMPDIR", roDir)

	body, contentType := multipartBody(t, true) // image + video
	req := httptest.NewRequest("POST", "/user/face-capture", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// 显式字段断言（非零值兜底）：修复前响应无 video_saved → 此断言失败
	assert.Contains(t, w.Body.String(), `"video_saved":false`, "视频保存失败必须显式可见（不静默）")

	var resp struct {
		Code int `json:"code"`
		Data struct {
			BatchID    string `json:"batch_id"`
			VideoSaved bool   `json:"video_saved"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.False(t, resp.Data.VideoSaved)

	// 照片路径不受影响：批次照常创建（pending），素材仅图片（无 video 资产）
	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", resp.Data.BatchID).First(&batch).Error)
	assert.Equal(t, "pending", batch.Status)

	var videoAssets int64
	require.NoError(t, db.Model(&models.MediaAsset{}).
		Where("source_id = ? AND source_type = ? AND file_type = ?", resp.Data.BatchID, "face_capture", "video").
		Count(&videoAssets).Error)
	assert.Zero(t, videoAssets, "视频失败时不得注册 video 资产")

	// 无残留临时目录（清理路径无泄漏）
	entries, err := os.ReadDir(roDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
