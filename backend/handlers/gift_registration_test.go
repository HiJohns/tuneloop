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
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// TestGiftRegistration covers TC #1551: gift points on registration.
// Two reward paths in PostRegister:
//  1. Registration gift: membership_gift_points (default 99) credited to
//     the new user's promo_points + PointsTransaction(type=registration).
//  2. Referral bonus: referrer's MembershipGiftRatio.ReferralRegPoints
//     credited to the referrer + PointsTransaction(type=referral_reg).
func TestGiftRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	// Registration gift points: 99 (explicit).
	require.NoError(t, db.Create(&models.SystemSetting{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		SettingKey:   "membership_gift_points",
		SettingValue: "99",
	}).Error)

	// Referrer with a membership level that grants 50 referral points.
	referrerID := "6d1e2c3a-0000-4000-8000-0000000000aa"
	referrer := models.User{
		ID:                referrerID,
		IAMSub:            referrerID,
		TenantID:          "00000000-0000-0000-0000-000000000001",
		OrgID:             "00000000-0000-0000-0000-000000000001",
		Username:          "referrer",
		MembershipLevelID: intPtr(1),
		PromoPoints:       models.FromYuan(100), // #1757: cents (100 元)
		Status:            "active",
	}
	require.NoError(t, db.Create(&referrer).Error)
	require.NoError(t, db.Model(&referrer).Update("ref_code", "abc12345").Error)

	require.NoError(t, db.Create(&models.MembershipGiftRatio{
		ID:                uuid.New().String(),
		LevelID:           1,
		SelfSpendRatio:    0.1,
		ReferralRegPoints: 50,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}).Error)

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	// ------------------------------------------------------------------
	// Scenario 1: register with ref → registration gift + referral bonus.
	// ------------------------------------------------------------------
	t.Run("register_with_ref_credits_both", func(t *testing.T) {
		newUserID := "6d1e2c3a-0000-4000-8000-0000000000c1"
		srv := newRegisterMockServer(t, newUserID, "GiftUser")
		defer srv.Close()
		services.SetIAMInternalURLForTesting(srv.URL)

		router := gin.New()
		router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

		body, _ := json.Marshal(map[string]interface{}{
			"name":     "Gift User",
			"phone":    "13900139001",
			"password": "secret123",
			"wx_code":  "gift-wx-code",
			"ref":      "abc12345",
		})
		req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "register: %s", w.Body.String())

		var resp struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)

		// New user: promo_points 0 + 99 元 = 9900 分 (cents, #1757).
		var newUser models.User
		require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&newUser).Error)
		require.Equal(t, models.Cents(9900), newUser.PromoPoints, "registration gift 9900 cents")

		// Registration PointsTransaction.
		var regTx int64
		require.NoError(t, db.Model(&models.PointsTransaction{}).
			Where("user_id = ? AND type = ? AND amount = ?", newUser.ID, "registration", 9900).
			Count(&regTx).Error)
		require.Equal(t, int64(1), regTx, "registration transaction recorded")

		// Referrer: 10000 + 50 元 = 15000 分 (cents, #1757).
		var refUser models.User
		require.NoError(t, db.Where("id = ?", referrerID).First(&refUser).Error)
		require.Equal(t, models.Cents(15000), refUser.PromoPoints, "referral bonus 5000 cents credited")

		// Referral PointsTransaction.
		var refTx int64
		require.NoError(t, db.Model(&models.PointsTransaction{}).
			Where("user_id = ? AND type = ? AND amount = ?", referrerID, "referral_reg", 5000).
			Count(&refTx).Error)
		require.Equal(t, int64(1), refTx, "referral transaction recorded")

		// Referral row created.
		var referralCount int64
		require.NoError(t, db.Model(&models.Referral{}).
			Where("referrer_id = ? AND ref_code = ?", referrerID, "abc12345").
			Count(&referralCount).Error)
		require.Equal(t, int64(1), referralCount, "referral row created")
	})

	// ------------------------------------------------------------------
	// Scenario 2: register without ref → registration gift only.
	// ------------------------------------------------------------------
	t.Run("register_without_ref_credits_only_gift", func(t *testing.T) {
		newUserID := "6d1e2c3a-0000-4000-8000-0000000000c2"
		srv := newRegisterMockServer(t, newUserID, "NoRefUser")
		defer srv.Close()
		services.SetIAMInternalURLForTesting(srv.URL)

		router := gin.New()
		router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

		body, _ := json.Marshal(map[string]interface{}{
			"name":     "No Ref User",
			"phone":    "13900139002",
			"password": "secret123",
			"wx_code":  "noref-wx-code",
		})
		req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "register: %s", w.Body.String())

		var resp struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)

		// New user: registration gift 99.
		var newUser models.User
		require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&newUser).Error)
		require.Equal(t, models.Cents(9900), newUser.PromoPoints, "registration gift 9900 cents without ref")

		// Referrer unchanged (no new referral bonus for this user).
		var refUser models.User
		require.NoError(t, db.Where("id = ?", referrerID).First(&refUser).Error)
		require.Equal(t, models.Cents(15000), refUser.PromoPoints, "referrer promo unchanged (no ref passed)")
	})
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()

func intPtr(v int) *int { return &v }
