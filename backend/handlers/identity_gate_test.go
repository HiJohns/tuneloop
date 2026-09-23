package handlers

import (
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2040: 下单身份门槛——缺证件照 / 未实名 → 40310 + 结构化 reasons。
func TestCheckIdentityForOrder(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	str := func(s string) *string { return &s }
	seed := func(faceVerified bool, front, back *string) string {
		u := models.User{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			Email: uuid.New().String() + "@example.com", Name: "U", Status: "active",
			FaceVerified: faceVerified, IdPhotoFront: front, IdPhotoBack: back,
		}
		require.NoError(t, db.Create(&u).Error)
		return u.ID
	}

	t.Run("缺照+未实名 → 40310 (两原因)", func(t *testing.T) {
		id := seed(false, nil, nil)
		code, msg, reasons := CheckIdentityForOrder(db, id)
		assert.Equal(t, BIZNeedIdentity, code)
		assert.NotEmpty(t, msg)
		assert.ElementsMatch(t, []string{"no_id_photo", "not_verified"}, reasons)
	})

	t.Run("有照但未实名 → 40310 (not_verified)", func(t *testing.T) {
		id := seed(false, str("/uploads/a.jpg"), str("/uploads/b.jpg"))
		code, _, reasons := CheckIdentityForOrder(db, id)
		assert.Equal(t, BIZNeedIdentity, code)
		assert.Equal(t, []string{"not_verified"}, reasons)
	})

	t.Run("已实名但缺照 → 40310 (no_id_photo)", func(t *testing.T) {
		id := seed(true, str("/uploads/a.jpg"), nil)
		code, msg, reasons := CheckIdentityForOrder(db, id)
		assert.Equal(t, BIZNeedIdentity, code)
		assert.Equal(t, []string{"no_id_photo"}, reasons)
		assert.Contains(t, msg, "身份证正反面")
	})

	t.Run("已实名+有照 → 放行", func(t *testing.T) {
		id := seed(true, str("/uploads/a.jpg"), str("/uploads/b.jpg"))
		code, msg, reasons := CheckIdentityForOrder(db, id)
		assert.Equal(t, 0, code)
		assert.Empty(t, msg)
		assert.Nil(t, reasons)
	})
}
