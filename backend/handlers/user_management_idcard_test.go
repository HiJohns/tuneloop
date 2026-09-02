package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupIdCardTestDB creates users + face_capture_batches tables with one
// customer user (with ID photos) and a pending batch.
func setupIdCardTestDB(t *testing.T) (*gorm.DB, string) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil, ""
	}
	database.SetDB(db)

	for _, m := range []interface{}{
		&models.User{}, &models.FaceCaptureBatch{}, &models.Notification{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	if !db.Migrator().HasColumn(&models.User{}, "iam_sub") {
		require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN iam_sub varchar(255)`).Error)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	front := "id/front.jpg"
	back := "id/back.jpg"
	db.Create(&models.User{
		ID:            userID,
		TenantID:      tenantID,
		OrgID:         tenantID,
		IAMSub:        userID,
		Role:          "USER",
		Status:        "active",
		IdPhotoFront:  &front,
		IdPhotoBack:   &back,
		IdPhotoOther:  nil,
		FaceVerified:  false,
		FaceVerifyMethod: strPtr("manual"),
	})

	batch := models.FaceCaptureBatch{
		ID:     uuid.New().String(),
		UserID: userID,
		Status: "pending",
	}
	require.NoError(t, db.Create(&batch).Error)

	return db, userID
}



// idCardRouter registers the two new handlers + UserBatches on a router.
func idCardRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewUserManagementHandler()
	fh := &FaceReviewHandler{}
	router.PUT("/api/admin/user-management/:id/id-card", h.UpdateIdCard)
	router.POST("/api/admin/user-management/:id/id-photo/reject", h.RejectIdPhotos)
	router.GET("/api/admin/face-review/user/:userId", fh.UserBatches)
	return router
}

func TestUpdateIdCard_HappyPath(t *testing.T) {
	db, userID := setupIdCardTestDB(t)
	router := idCardRouter()

	body := `{"real_name":"张三","id_card_no":"110101199001011234","id_card_expire":"长期","id_card_authority":"北京市公安局","id_card_address":"北京市朝阳区"}`
	req := httptest.NewRequest("PUT", "/api/admin/user-management/"+userID+"/id-card", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.Where("id = ?", userID).First(&u).Error)
	assert.Equal(t, "张三", *u.RealName)
	assert.Equal(t, "110101199001011234", *u.IdCardNo)
	assert.Equal(t, "长期", *u.IdCardExpire)

	var n models.Notification
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "id_verify").First(&n).Error)
	assert.Contains(t, n.Title, "实名信息已采集")
}

func TestUpdateIdCard_InvalidIdCardNo(t *testing.T) {
	setupIdCardTestDB(t)
	router := idCardRouter()

	body := `{"id_card_no":"12345"}`
	req := httptest.NewRequest("PUT", "/api/admin/user-management/xxx/id-card", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateIdCard_UserNotFound(t *testing.T) {
	setupIdCardTestDB(t)
	router := idCardRouter()

	body := `{"real_name":"李四"}`
	req := httptest.NewRequest("PUT", "/api/admin/user-management/00000000-0000-0000-0000-000000000000/id-card", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRejectIdPhotos_ClearsPhotosAndVoidsBatch(t *testing.T) {
	db, userID := setupIdCardTestDB(t)
	router := idCardRouter()

	req := httptest.NewRequest("POST", "/api/admin/user-management/"+userID+"/id-photo/reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.Where("id = ?", userID).First(&u).Error)
	assert.Nil(t, u.IdPhotoFront)
	assert.Nil(t, u.IdPhotoBack)
	assert.False(t, u.FaceVerified)

	var batch models.FaceCaptureBatch
	require.NoError(t, db.Where("user_id = ?", userID).First(&batch).Error)
	assert.Equal(t, "rejected", batch.Status)
	assert.NotNil(t, batch.RejectReason)

	var n models.Notification
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "id_verify").Order("created_at DESC").First(&n).Error)
	assert.Contains(t, n.Title, "证件照未通过")
}

func TestUserBatches_ReturnsHistory(t *testing.T) {
	db, userID := setupIdCardTestDB(t)
	router := idCardRouter()

	// Add a rejected historical batch.
	rejected := models.FaceCaptureBatch{
		ID:           uuid.New().String(),
		UserID:       userID,
		Status:       "rejected",
		RejectReason: strPtr("照片不清晰"),
	}
	require.NoError(t, db.Create(&rejected).Error)

	req := httptest.NewRequest("GET", "/api/admin/face-review/user/"+userID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Len(t, resp.Data.List, 2)

	statuses := map[string]bool{}
	for _, item := range resp.Data.List {
		statuses[item["status"].(string)] = true
	}
	assert.True(t, statuses["pending"])
	assert.True(t, statuses["rejected"])
}

func TestUserBatches_UserNotFound(t *testing.T) {
	setupIdCardTestDB(t)
	router := idCardRouter()

	req := httptest.NewRequest("GET", "/api/admin/face-review/user/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
