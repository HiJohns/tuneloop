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
	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// TestMembershipFlow covers TC #1553: membership registration fee.
// Register → membership_fee returned → prepay (order_type=membership) in
// mock mode → payment record booked as paid. Membership payment has no
// side effects on activation (gift points are credited at registration).
func TestMembershipFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wechatpay.InitGlobal(wechatpay.LoadConfig()) // WECHAT_PAY_MOCK_MODE=true → mock paid
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	// Isolated tables per test run.
	_ = db.Migrator().DropTable(&models.Instrument{}, &models.Order{}, &models.LeaseSession{}, &models.OrderStatusHistory{}, &models.DamageAssessment{}, &models.Settlement{}, &models.OrderRefundRecord{}, &models.OrderPaymentRecord{}, &models.PointsTransaction{}, &models.User{}, &models.DamageReport{}, &models.MembershipGiftRatio{}, &models.PricingTemplate{}, &models.MerchantPricingConfig{}, &models.PointsPolicy{}, &models.MerchantSettlementConfig{}, &models.SystemSetting{}, &models.PromoPlan{}, &models.Referral{}, &models.MembershipLevel{})
	require.NoError(t, db.Migrator().CreateTable(&models.Instrument{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Order{}))
	require.NoError(t, db.Migrator().CreateTable(&models.LeaseSession{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderStatusHistory{}))
	require.NoError(t, db.Migrator().CreateTable(&models.DamageAssessment{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Settlement{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderRefundRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderPaymentRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.PointsTransaction{}))
	require.NoError(t, db.Migrator().AutoMigrate(&models.User{}))
	require.NoError(t, db.Migrator().CreateTable(&models.DamageReport{}))
	require.NoError(t, db.Migrator().CreateTable(&models.MembershipGiftRatio{}))
	require.NoError(t, db.Migrator().CreateTable(&models.PricingTemplate{}))
	require.NoError(t, db.Migrator().CreateTable(&models.MerchantPricingConfig{}))
	require.NoError(t, db.Migrator().CreateTable(&models.PointsPolicy{}))
	require.NoError(t, db.Migrator().CreateTable(&models.MerchantSettlementConfig{}))
	require.NoError(t, db.Migrator().CreateTable(&models.SystemSetting{}))
	require.NoError(t, db.Migrator().CreateTable(&models.PromoPlan{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Referral{}))
	require.NoError(t, db.Migrator().CreateTable(&models.MembershipLevel{}))
	if !db.Migrator().HasColumn(&models.User{}, "iam_sub") {
		require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN iam_sub varchar(255)`).Error)
	}

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
	// Step 4: Membership callback → no side effects, no error.
	// ------------------------------------------------------------------
	// applySideEffects is called by the wechatpay callback; the membership
	// case returns nil (nothing to apply — gift points were credited at
	// registration). Verify no panic / no error via a direct call.
	err = applySideEffects(db, &record, time.Now())
	require.NoError(t, err, "membership payment side effects must not error")

	// Payment record unchanged after side effects (no status flip).
	var after models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&after).Error)
	assert.Equal(t, "paid", after.Status)
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
var _ = uuid.New
