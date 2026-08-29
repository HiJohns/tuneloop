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

	"gorm.io/gorm"
)

// #1791 T3 tests: 人工审核 API（approve/reject/队列）+ 发货强制校验。

func faceReviewRouter(t *testing.T, operatorID string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, operatorID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &FaceReviewHandler{}
	router.GET("/admin/face-review/queue", h.Queue)
	router.POST("/admin/face-review/:batchId", h.Review)
	return router
}

// setupReviewUser 创建待审核用户（证件照 + pending 批次）+ 返回 userID/batchID。
func setupReviewUser(t *testing.T) (string, string, *gorm.DB) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "review-" + uuid.NewString()[:6], Status: "active",
		Name:         "张三",
		IdPhotoFront: strPtr("/uploads/media/front.jpg"),
	}
	require.NoError(t, db.Create(&user).Error)
	batch := models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&batch).Error)
	return user.ID, batch.ID, db
}

// TestFaceReview_Approve (#1791): approve → face_verified=true +
// face_verify_method=manual + 批次 approved + audit_logs 留痕。
func TestFaceReview_Approve(t *testing.T) {
	userID, batchID, db := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{"action": "approve"})
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.Where("id = ?", userID).First(&updated).Error)
	require.True(t, updated.FaceVerified, "face_verified=true")
	require.NotNil(t, updated.FaceVerifyMethod)
	require.Equal(t, "manual", *updated.FaceVerifyMethod)

	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", batchID).First(&batch).Error)
	require.Equal(t, "approved", batch.Status)
	require.NotNil(t, batch.ReviewedAt, "reviewed_at stamped")

	// audit_logs 留痕。
	var audit models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "face_review_approve", userID).First(&audit).Error)
}

// TestFaceReview_Reject (#1791): reject → 批次 rejected + reason + audit_logs。
func TestFaceReview_Reject(t *testing.T) {
	userID, batchID, db := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{"action": "reject", "reason": "照片与证件不符"})
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", batchID).First(&batch).Error)
	require.Equal(t, "rejected", batch.Status)
	require.NotNil(t, batch.RejectReason)
	require.Equal(t, "照片与证件不符", *batch.RejectReason)

	var audit models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "face_review_reject", userID).First(&audit).Error)

	// 用户 face_verified 仍为 false。
	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.False(t, user.FaceVerified)
}

// TestFaceReview_Queue (#1791): 队列返回 pending 批次 + 证件照 + 自拍素材。
func TestFaceReview_Queue(t *testing.T) {
	_, _, db := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	req := httptest.NewRequest("GET", "/admin/face-review/queue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				BatchID  string   `json:"batch_id"`
				IDPhotos []string `json:"id_photos"`
			} `json:"list"`
			Total int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Total)
	require.Len(t, resp.Data.List, 1)
	require.Len(t, resp.Data.List[0].IDPhotos, 1, "front photo included")
	_ = db
}

// TestFaceReview_Reject_RequiresReason (#1791): reject 无 reason → 40002。
func TestFaceReview_Reject_RequiresReason(t *testing.T) {
	_, batchID, _ := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{"action": "reject"})
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestUpdateShipping_VerifyRequired (#1791 R1): 未核身买家订单发货 → 40002。
func TestUpdateShipping_VerifyRequired(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID

	// 未核身用户（有证件照但 face_verified=false，无批次 → uploaded）。
	buyer := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: tenantID, OrgID: orgID,
		Username: "unverified", Status: "active",
		IdPhotoFront: strPtr("/uploads/media/front.jpg"),
	}
	require.NoError(t, db.Create(&buyer).Error)

	instrument := models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-VERIFY", StockStatus: "available",
	}
	require.NoError(t, db.Create(&instrument).Error)

	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: buyer.ID, InstrumentID: instrument.ID,
		Status: models.OrderStatusPaid,
	}
	require.NoError(t, db.Create(&order).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/warehouse/orders/:id/shipping", (&WarehouseHandler{}).UpdateShipping)

	body, _ := json.Marshal(map[string]interface{}{
		"tracking_number": "SF123", "company": "顺丰", "shipped_at": time.Now(),
	})
	req := httptest.NewRequest("PUT", "/warehouse/orders/"+order.ID+"/shipping", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "用户未完成实名核身")
}
