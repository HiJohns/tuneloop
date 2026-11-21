package handlers

// #2435: PUT /users/me second-doc type — self-report before verification,
// locked after verification, enum validation.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/models"
)

func TestUpdateCurrentUser_SecondDocType_2435(t *testing.T) {
	db, router, _, userID := setupUpdateCurrentUserTest(t, http.StatusOK)

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// self-report before verification
	w := put(`{"id_photo_other_type":"student"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	require.NotNil(t, user.IdPhotoOtherType)
	assert.Equal(t, "student", *user.IdPhotoOtherType)

	// invalid enum is rejected and does not mutate
	w = put(`{"id_photo_other_type":"bad"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "student", *user.IdPhotoOtherType)

	// verified type is locked for the user
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"id_photo_other_verified": true}).Error)
	w = put(`{"id_photo_other_type":"teacher"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "student", *user.IdPhotoOtherType)

	// same value is allowed
	w = put(`{"id_photo_other_type":"student"}`)
	assert.Equal(t, http.StatusOK, w.Code)
}
