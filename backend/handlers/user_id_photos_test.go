package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// smallValidJPG returns a tiny fake JPEG payload (the storage layer only
// checks content-type, not the actual image bytes).
func smallValidJPG() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01}
}

func setupIdPhotoTestDB(t *testing.T) (*gorm.DB, string, string) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil, "", ""
	}
	database.SetDB(db)

	_ = db.Migrator().DropTable(&models.User{})
	require.NoError(t, db.Migrator().CreateTable(&models.User{}))
	// iam_sub excluded from GORM tags; add manually
	if !db.Migrator().HasColumn(&models.User{}, "iam_sub") {
		require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN iam_sub varchar(255)`).Error)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	db.Create(&models.User{
		ID:       userID,
		TenantID: tenantID,
		OrgID:    tenantID,
		IAMSub:   userID,
		Role:     "USER",
		Status:   "active",
	})

	return db, tenantID, userID
}

// idPhotoRouter builds a router with the given actor injected as the JWT
// identity, mirroring how integration tests exercise handlers.
func idPhotoRouter(actor testutil.TestActor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewUserOnboardingHandler()
	router.POST("/api/user/id-photo", h.UploadIDPhoto)
	router.GET("/api/user/id-photos", h.GetIdPhotos)
	router.DELETE("/api/user/id-photo", h.DeleteIdPhoto)
	router.GET("/api/user/:userId/id-photos", h.GetUserIdPhotos)
	router.POST("/api/admin/user-management/:id/id-photo", h.AdminUploadIDPhoto)
	router.DELETE("/api/admin/user-management/:id/id-photo", h.AdminDeleteIdPhoto)
	return router
}

func uploadForm(side string) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("side", side)
	// Build the file part with an explicit image/jpeg Content-Type header
	// (multipart.Writer would otherwise use application/octet-stream and the
	// handler's MIME allowlist would reject it).
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="id_`+side+`.jpg"`)
	hdr.Set("Content-Type", "image/jpeg")
	fw, err := w.CreatePart(hdr)
	if err != nil {
		panic(err)
	}
	_, _ = fw.Write(smallValidJPG())
	w.Close()
	return body, w.FormDataContentType()
}

// TestIdPhotoUploadPersistsAndDeletes covers the customer self-service
// flow: POST /user/id-photo (front) → DB column set; GET /user/id-photos
// returns URL; upload back → both present; DELETE → column cleared. (#1598)
func TestIdPhotoUploadPersistsAndDeletes(t *testing.T) {
	db, tenantID, userID := setupIdPhotoTestDB(t)
	actor := testutil.MakeCustomer(tenantID, userID)
	router := idPhotoRouter(actor)

	// Upload front
	body, ctype := uploadForm("front")
	req := httptest.NewRequest("POST", "/api/user/id-photo", body)
	req.Header.Set("Content-Type", ctype)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Logf("upload front response: %s", resp.Body.String())
	}
	require.Equal(t, http.StatusOK, resp.Code)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, float64(20000), out["code"])
	assert.Contains(t, out["data"].(map[string]interface{})["url"].(string), "/uploads/media/id_photos/")

	// DB column is set
	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.NotNil(t, user.IdPhotoFront)
	assert.Contains(t, *user.IdPhotoFront, "id_photos/")

	// GET id-photos returns front URL
	req = httptest.NewRequest("GET", "/api/user/id-photos", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	data := out["data"].(map[string]interface{})
	assert.Contains(t, data["front"].(string), "/uploads/media/id_photos/")
	assert.Equal(t, "", data["back"])

	// Upload back
	body, ctype = uploadForm("back")
	req = httptest.NewRequest("POST", "/api/user/id-photo", body)
	req.Header.Set("Content-Type", ctype)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	// Invalid side → 400
	body, ctype = uploadForm("middle")
	req = httptest.NewRequest("POST", "/api/user/id-photo", body)
	req.Header.Set("Content-Type", ctype)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)

	// DELETE front → column cleared
	req = httptest.NewRequest("DELETE", "/api/user/id-photo?side=front", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.NotNil(t, user.IdPhotoFront)
	assert.Equal(t, "", *user.IdPhotoFront)
}

// TestIdPhotoStaffViewAndIsolation covers staff viewing a user's ID photos
// and the customer-forbidden path. (#1598)
func TestIdPhotoStaffViewAndIsolation(t *testing.T) {
	_, tenantID, userID := setupIdPhotoTestDB(t)

	// Seed target user with a front photo key directly
	key := "id_photos/" + userID + "_1691234567890.jpg"
	db := database.GetDB()
	db.Model(&models.User{}).Where("id = ?", userID).Update("id_photo_front", key)

	// Staff can view
	staffActor := testutil.MakeSiteMember(tenantID, tenantID, uuid.New().String())
	router := idPhotoRouter(staffActor)
	req := httptest.NewRequest("GET", "/api/user/"+userID+"/id-photos", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	data := out["data"].(map[string]interface{})
	assert.Contains(t, data["front"].(string), "/uploads/media/")

	// Customer viewing another user → 403
	custActor := testutil.MakeCustomer(tenantID, uuid.New().String())
	router2 := idPhotoRouter(custActor)
	req = httptest.NewRequest("GET", "/api/user/"+userID+"/id-photos", nil)
	resp = httptest.NewRecorder()
	router2.ServeHTTP(resp, req)
	require.Equal(t, http.StatusForbidden, resp.Code)
}

// TestIdPhotoAdminUploadDelete covers the PC admin endpoints:
// POST /admin/user-management/:id/id-photo and DELETE. (#1598)
func TestIdPhotoAdminUploadDelete(t *testing.T) {
	_, tenantID, userID := setupIdPhotoTestDB(t)
	adminActor := testutil.MakeSiteAdmin(tenantID, tenantID, uuid.New().String())
	router := idPhotoRouter(adminActor)

	// Admin uploads back photo for the user
	body, ctype := uploadForm("back")
	req := httptest.NewRequest("POST", "/api/admin/user-management/"+userID+"/id-photo", body)
	req.Header.Set("Content-Type", ctype)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	db := database.GetDB()
	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.NotNil(t, user.IdPhotoBack)
	assert.Contains(t, *user.IdPhotoBack, "id_photos/")

	// Admin deletes it
	req = httptest.NewRequest("DELETE", "/api/admin/user-management/"+userID+"/id-photo?side=back", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.NotNil(t, user.IdPhotoBack)
	assert.Equal(t, "", *user.IdPhotoBack)
}
