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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// TestMembershipFlow covers TC #1553: membership registration fee.
// Register → membership_fee returned → prepay (order_type=membership) in
// mock mode → payment record booked as paid. Membership payment has no
// side effects on activation (gift points are credited at registration).
func TestMembershipFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	// Membership fee setting: 99 (default, made explicit).
	require.NoError(t, db.Create(&models.SystemSetting{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		SettingKey:   "membership_fee",
		SettingValue: "99",
	}).Error)

	// ------------------------------------------------------------------
	// Step 1: Register → membership_fee returned.
	// ------------------------------------------------------------------
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000d1"
	srv := newRegisterMockServer(t, newUserID, "MembershipUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	registerBody, _ := json.Marshal(map[string]interface{}{
		"name":     "Membership User",
		"phone":    "13800138000",
		"password": "secret123",
		"wx_code":  "mem-wx-code",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "register: %s", w.Body.String())

	var registerResp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken   string  `json:"access_token"`
			MembershipFee float64 `json:"membership_fee"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &registerResp))
	require.Equal(t, 20000, registerResp.Code)
	assert.Equal(t, 99.0, registerResp.Data.MembershipFee, "membership_fee from system_settings")

	// Local user created (needed for prepay user resolution).
	var localUser models.User
	require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&localUser).Error)

	// ------------------------------------------------------------------
	// Step 2: Prepay membership (mock mode → paid immediately).
	// ------------------------------------------------------------------
	customer := testutil.MakeCustomer("", localUser.ID)
	prepayRouter := gin.New()
	prepayRouter.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	prepayRouter.POST("/api/pay/prepay", PrepayOrder)

	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
	})
	req = httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	prepayRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var prepayResp struct {
		Code int `json:"code"`
		Data struct {
			Mock    bool `json:"mock"`
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prepayResp))
	require.Equal(t, 20000, prepayResp.Code)
	assert.True(t, prepayResp.Data.Mock, "mock mode active in test env")

	// ------------------------------------------------------------------
	// Step 3: Payment record booked as paid (mock), amount 99.
	// ------------------------------------------------------------------
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_type = ? AND amount = ?", "membership", 99.0).
		Order("created_at desc").First(&record).Error)
	require.Equal(t, "paid", record.Status, "mock mode books payment as paid")
	assert.Equal(t, "membership", record.OrderType)
	assert.Equal(t, 99.0, record.Amount)
	assert.Equal(t, "mock", *record.Method)
	assert.Equal(t, prepayResp.Data.Data.OutTradeNo, *record.OutTradeNo)

	// ------------------------------------------------------------------
	// Step 4: Membership callback → activate level, no error.
	// ------------------------------------------------------------------
	// applySideEffects (membership case) activates the highest level whose
	// MinAmount <= paid amount (#1575).
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        1,
		Name:      "初级会员",
		MinAmount: 99,
	}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        2,
		Name:      "高级会员",
		MinAmount: 199,
	}).Error)

	err := applySideEffects(db, &record, time.Now())
	require.NoError(t, err, "membership payment side effects must not error")

	// Level 1 (99 <= 99) activated, not level 2 (99 < 199).
	var userAfter models.User
	require.NoError(t, db.Where("id = ?", localUser.ID).First(&userAfter).Error)
	require.NotNil(t, userAfter.MembershipLevelID, "membership level set after payment")
	require.Equal(t, 1, *userAfter.MembershipLevelID, "highest level with MinAmount <= 99 activated")

	// Payment record unchanged after side effects (no status flip).
	var after models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&after).Error)
	assert.Equal(t, "paid", after.Status)
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
var _ = uuid.New
