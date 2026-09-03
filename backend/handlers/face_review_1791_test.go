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

	"tuneloop-backend/database"
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

// faceReviewRouterWithTenant 创建测试路由器，中间件同时设置 tenant_id（模拟平台员工带租户 scope）。
func faceReviewRouterWithTenant(t *testing.T, operatorID, tenantID string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, operatorID)
		ctx = database.SetTenantID(ctx, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &FaceReviewHandler{}
	router.GET("/admin/face-review/queue", h.Queue)
	router.POST("/admin/face-review/:batchId", h.Review)
	router.GET("/admin/face-review/user/:userId", h.UserBatches)
	return router
}

// setupReviewUserZeroTenant 创建零租户顾客（tenant_id=00000000-...）+ pending 批次，用于验证 #1812 修复。
func setupReviewUserZeroTenant(t *testing.T) (string, string, *gorm.DB) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: "00000000-0000-0000-0000-000000000000", OrgID: "00000000-0000-0000-0000-000000000000",
		Username: "zero-" + uuid.NewString()[:6], Status: "active",
		Name:         "零租户顾客",
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
// #1807: 员工填写 5 项实名信息（姓名/身份证号/有效期/签发机关/住址）一并落库。
func TestFaceReview_Approve(t *testing.T) {
	userID, batchID, db := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{
		"action": "approve", "real_name": "张三", "id_card_no": "110101199001011234",
		"id_card_expire": "2035-12-31", "id_card_authority": "北京市公安局", "id_card_address": "北京市东城区XX街道1号",
	})
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
	// #1807: 员工填写实名信息落库（5 项）。
	require.NotNil(t, updated.RealName)
	require.Equal(t, "张三", *updated.RealName)
	require.NotNil(t, updated.IdCardNo)
	require.Equal(t, "110101199001011234", *updated.IdCardNo)
	require.NotNil(t, updated.IdCardExpire)
	require.Equal(t, "2035-12-31", *updated.IdCardExpire)
	require.NotNil(t, updated.IdCardAuthority)
	require.Equal(t, "北京市公安局", *updated.IdCardAuthority)
	require.NotNil(t, updated.IdCardAddress)
	require.Equal(t, "北京市东城区XX街道1号", *updated.IdCardAddress)

	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("id = ?", batchID).First(&batch).Error)
	require.Equal(t, "approved", batch.Status)
	require.NotNil(t, batch.ReviewedAt, "reviewed_at stamped")

	// audit_logs 留痕。
	var audit models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "face_review_approve", userID).First(&audit).Error)

	// #1816: 通知顾客（ntype=id_verify）。
	var notif models.Notification
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "id_verify").First(&notif).Error)
	require.Equal(t, "实名认证已通过", notif.Title)
}

// TestFaceReview_Approve_LongExpire (#1807): 有效期支持「长期」。
func TestFaceReview_Approve_LongExpire(t *testing.T) {
	userID, batchID, db := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{
		"action": "approve", "real_name": "李四", "id_card_no": "110101199002021234",
		"id_card_expire": "长期", "id_card_authority": "北京市公安局", "id_card_address": "北京市西城区XX路2号",
	})
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.Where("id = ?", userID).First(&updated).Error)
	require.NotNil(t, updated.IdCardExpire)
	require.Equal(t, "长期", *updated.IdCardExpire)
	require.True(t, updated.FaceVerified)
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

	// #1816: 通知顾客（ntype=id_verify）。
	var notif models.Notification
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "id_verify").First(&notif).Error)
	require.Equal(t, "实名认证未通过，请重新采集", notif.Title)
	require.Contains(t, notif.Content, "照片与证件不符")
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

// TestFaceReview_Queue_NormalizeDoublePrefix (#1814): 预生产历史脏数据
// users.id_photo_front/back 带双前缀 /uploads/media//uploads/media/...
// resolveSelfieURL 归一化为单前缀，修复审核队列证件照 404。
func TestFaceReview_Queue_NormalizeDoublePrefix(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		Username: "dirty-" + uuid.NewString()[:6], Status: "active",
		Name: "脏数据用户",
		// 与预生产确认的脏值同构：/uploads/media//uploads/media/id_photos/pending_...
		IdPhotoFront: strPtr("/uploads/media//uploads/media/id_photos/pending_front.jpg"),
	}
	require.NoError(t, db.Create(&user).Error)
	batch := models.FaceCaptureBatch{
		ID: uuid.New().String(), UserID: user.ID, Status: "pending",
		SubmittedAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&batch).Error)

	router := faceReviewRouter(t, uuid.New().String())
	req := httptest.NewRequest("GET", "/admin/face-review/queue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				IDPhotos []string `json:"id_photos"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1)
	require.Len(t, resp.Data.List[0].IDPhotos, 1, "front photo included")
	require.Equal(t, "/uploads/media/id_photos/pending_front.jpg", resp.Data.List[0].IDPhotos[0],
		"double-prefix dirty value normalized to single prefix")
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
	require.Contains(t, w.Body.String(), "user id verification required")
}

// TestFaceReview_Approve_RequiresRealInfo (#1807): approve 必须携带员工填写的
// 5 项实名信息（real_name/id_card_no/id_card_expire/id_card_authority/id_card_address）。
func TestFaceReview_Approve_RequiresRealInfo(t *testing.T) {
	_, batchID, _ := setupReviewUser(t)
	router := faceReviewRouter(t, uuid.New().String())

	body, _ := json.Marshal(map[string]interface{}{"action": "approve"})
	req := httptest.NewRequest("POST", "/admin/face-review/"+batchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "real_name, id_card_no, id_card_expire, id_card_authority and id_card_address are required")
}

// TestFaceReview_Queue_ZeroTenantCustomer (#1812): 平台员工带 tenant scope 访问队列，
// 应能看到零租户顾客（tenant_id=00000000-...）的 pending 批次。修复前因
// addTenantScope 过滤 users 查询导致 record not found → 队列空。
func TestFaceReview_Queue_ZeroTenantCustomer(t *testing.T) {
	_, batchID, _ := setupReviewUserZeroTenant(t)
	staffTenant := uuid.New().String()
	router := faceReviewRouterWithTenant(t, uuid.New().String(), staffTenant)

	req := httptest.NewRequest("GET", "/admin/face-review/queue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Total, "zero-tenant customer batch should appear in queue")
	require.Equal(t, batchID, resp.Data.List[0]["batch_id"])
}

// TestFaceReview_UserBatches_ZeroTenantCustomer (#1812): 平台员工带 tenant scope
// 访问用户批次列表，应能看到零租户顾客的全部批次。修复前因 addTenantScope 过滤
// users 查询导致 404。
func TestFaceReview_UserBatches_ZeroTenantCustomer(t *testing.T) {
	userID, batchID, _ := setupReviewUserZeroTenant(t)
	staffTenant := uuid.New().String()
	router := faceReviewRouterWithTenant(t, uuid.New().String(), staffTenant)

	req := httptest.NewRequest("GET", "/admin/face-review/user/"+userID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Total, "zero-tenant customer batch should appear")
	require.Equal(t, batchID, resp.Data.List[0]["batch_id"])
}
