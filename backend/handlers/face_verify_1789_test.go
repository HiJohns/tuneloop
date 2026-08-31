package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"gorm.io/gorm"
)

// #1789 T1 tests: id_verify_status 五态派生 + 信息变更作废批次（R2 C4）。

// setupVerifyUser 创建用户 + 可选证件照/批次。
func setupVerifyUser(t *testing.T, opts map[string]interface{}) (*models.User, *gorm.DB) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "verify-" + uuid.NewString()[:6], Status: "active",
	}
	if v, ok := opts["id_photo_front"].(string); ok {
		user.IdPhotoFront = &v
	}
	if v, ok := opts["face_verified"].(bool); ok {
		user.FaceVerified = v
	}
	require.NoError(t, db.Create(&user).Error)
	return &user, db
}

// TestDeriveIdVerifyStatus_None (#1789): 三张证件照均空 → none。
func TestDeriveIdVerifyStatus_None(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{})
	require.Equal(t, IdVerifyStatusNone, deriveIdVerifyStatus(db, user))
}

// TestDeriveIdVerifyStatus_Verified (#1789): face_verified=true → verified
// （不查批次——即使存在 rejected 批次也不影响）。
func TestDeriveIdVerifyStatus_Verified(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{
		"id_photo_front": "/uploads/media/front.jpg",
		"face_verified":  true,
	})
	// 存在 rejected 批次也不影响（verified 优先级最高）。
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "rejected",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)
	require.Equal(t, IdVerifyStatusVerified, deriveIdVerifyStatus(db, user))
}

// TestDeriveIdVerifyStatus_PendingReview (#1789): 证件照已上传 + pending 批次 → pending_review。
func TestDeriveIdVerifyStatus_PendingReview(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{
		"id_photo_front": "/uploads/media/front.jpg",
	})
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)
	require.Equal(t, IdVerifyStatusPendingReview, deriveIdVerifyStatus(db, user))
}

// TestDeriveIdVerifyStatus_Rejected (#1789): rejected 批次 → rejected。
func TestDeriveIdVerifyStatus_Rejected(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{
		"id_photo_front": "/uploads/media/front.jpg",
	})
	reason := "照片不清晰"
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "rejected",
		RejectReason: &reason, SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)
	require.Equal(t, IdVerifyStatusRejected, deriveIdVerifyStatus(db, user))
}

// TestDeriveIdVerifyStatus_Uploaded (#1789): 证件照已上传 + 无批次 → uploaded。
func TestDeriveIdVerifyStatus_Uploaded(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{
		"id_photo_front": "/uploads/media/front.jpg",
	})
	require.Equal(t, IdVerifyStatusUploaded, deriveIdVerifyStatus(db, user))
}

// TestDeriveIdVerifyStatus_LatestBatch (#1789): 多批次取最新（pending 优先于旧 rejected）。
func TestDeriveIdVerifyStatus_LatestBatch(t *testing.T) {
	user, db := setupVerifyUser(t, map[string]interface{}{
		"id_photo_front": "/uploads/media/front.jpg",
	})
	now := time.Now()
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "rejected",
		SubmittedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour),
	}).Error)
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: now, CreatedAt: now,
	}).Error)
	require.Equal(t, IdVerifyStatusPendingReview, deriveIdVerifyStatus(db, user), "最新批次 pending 优先")
}

// TestUpdateCurrentUser_IdentityChange_InvalidatesBatches (R2 C4):
// 修改 id_card_no → face_verified 清除 + face_verify_method 清除 +
// pending 批次作废（rejected, reason=身份信息已变更，请重新采集）。
func TestUpdateCurrentUser_IdentityChange_InvalidatesBatches(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "verify-invalidate", Status: "active",
		FaceVerified:     true,
		FaceVerifiedAt:   func() *time.Time { t := time.Now(); return &t }(),
		FaceVerifyMethod: strPtr("tencent"),
		IdPhotoFront:     strPtr("/uploads/media/front.jpg"),
	}
	require.NoError(t, db.Create(&user).Error)

	// 创建 pending 批次。
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	// Mock IAM UpdateUser（UpdateCurrentUser 先调 IAM）。
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    20000,
			"message": "success",
		})
	})
	mockIAM := httptest.NewServer(mux)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	defer services.SetIAMInternalURLForTesting("")

	// 通过 UpdateCurrentUser 修改 id_card_no。
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, user.IAMSub)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/users/me", (&UserStaffHandler{}).UpdateCurrentUser)

	body, _ := json.Marshal(map[string]interface{}{
		"id_card_no": "110101199001011234",
	})
	req := httptest.NewRequest("PUT", "/users/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// face_verified/method 清除。
	var updated models.User
	require.NoError(t, db.Where("id = ?", user.ID).First(&updated).Error)
	require.False(t, updated.FaceVerified, "face_verified cleared")
	require.Nil(t, updated.FaceVerifyMethod, "face_verify_method cleared")
	require.Nil(t, updated.FaceVerifiedAt, "face_verified_at cleared")

	// pending 批次作废。
	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&batch).Error)
	require.Equal(t, "rejected", batch.Status, "pending batch invalidated")
	require.NotNil(t, batch.RejectReason)
	require.Equal(t, "身份信息已变更，请重新采集", *batch.RejectReason)
}

// TestUpdateCurrentUser_SameValue_KeepsBatches (#1807):
// 顾客保存资料时原样提交已上传的 id_photo_front/back（值相同）→
// 不判定身份信息变更 → pending 批次保留、face_verified 不清除。
func TestUpdateCurrentUser_SameValue_KeepsBatches(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "verify-samevalue", Status: "active",
		FaceVerified:     true,
		FaceVerifyMethod: strPtr("manual"),
		IdPhotoFront:     strPtr("/uploads/media/front.jpg"),
		IdPhotoBack:      strPtr("/uploads/media/back.jpg"),
	}
	require.NoError(t, db.Create(&user).Error)

	// 创建 pending 批次。
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}).Error)

	// Mock IAM UpdateUser。
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	mockIAM := httptest.NewServer(mux)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	defer services.SetIAMInternalURLForTesting("")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, user.IAMSub)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/users/me", (&UserStaffHandler{}).UpdateCurrentUser)

	// 原样重提交已上传的证件照（保存资料场景，EditProfile 会带上这些值）。
	body, _ := json.Marshal(map[string]interface{}{
		"nickname":       "测试",
		"id_photo_front": "/uploads/media/front.jpg",
		"id_photo_back":  "/uploads/media/back.jpg",
	})
	req := httptest.NewRequest("PUT", "/users/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// face_verified 保留 + pending 批次保留。
	var updated models.User
	require.NoError(t, db.Where("id = ?", user.ID).First(&updated).Error)
	require.True(t, updated.FaceVerified, "face_verified kept")
	require.NotNil(t, updated.FaceVerifyMethod)
	require.Equal(t, "manual", *updated.FaceVerifyMethod)

	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&batch).Error)
	require.Equal(t, "pending", batch.Status, "pending batch kept (same value is not an identity change)")
}
