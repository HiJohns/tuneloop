package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1867: deposit-free applications are restricted to verified students /
// faculty with a credit score above the configured threshold, and require a
// recommendation letter. Gates apply to both order creation paths.

func setup1867Fixture(t *testing.T) (*gorm.DB, *UserRentalHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("DEPOSIT_WAIVER_MIN_CREDIT", "600")
	db := testfixtures.SetupTestDB(t)
	// Guarantor tables are not part of the shared fixture set — create them
	// for the deposit-free flow under test.
	require.NoError(t, db.Migrator().CreateTable(&models.Guarantor{}, &models.OrderGuarantor{}))
	return db, NewUserRentalHandler()
}

// seed1867User creates a local user resolvable via iam_sub (EnsureLocalUser).
func seed1867User(t *testing.T, db *gorm.DB, tenantID, orgID, iamSub string, faceVerified bool, idType *string, credit int) string {
	t.Helper()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, TenantID: tenantID, OrgID: orgID,
		Username: "u-" + iamSub[:8], Status: "active",
		FaceVerified: faceVerified, IdPhotoOtherType: idType, CreditScore: credit,
	}).Error)
	return userID
}

func seed1867Instrument(t *testing.T, db *gorm.DB, tenantID, orgID string) string {
	t.Helper()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1867-INST", StockStatus: "available",
		Pricing: `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 300.0, "deposit": 500.0}]`,
	}).Error)
	return instID
}

func new1867Router(handler *UserRentalHandler, tenantID, orgID, iamSub string) *gin.Engine {
	// Order creation persists orders.org_id — the actor must carry the org.
	actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: iamSub, Role: "USER"}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/orders", handler.CreateOrder)
	router.POST("/api/user/orders/batch", handler.BatchCreateOrder)
	router.GET("/api/user/deposit-waiver/eligibility", GetDepositWaiverEligibility)
	return router
}

func post1867JSON(t *testing.T, router *gin.Engine, path string, body interface{}) (int, int, map[string]interface{}) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Code, resp.Data
}

func strPtr1867(s string) *string { return &s }

// Table-driven coverage of the eligibility rules themselves.
func TestDepositWaiverEligibilityRules(t *testing.T) {
	db, _ := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1867f1a2b3c4")

	cases := []struct {
		name        string
		faceOK      bool
		idType      *string
		credit      int
		wantOK      bool
		wantReason  string
	}{
		{"verified student passes", true, strPtr1867("student"), 700, true, ""},
		{"verified teacher passes", true, strPtr1867("teacher"), 600, true, ""},
		{"unverified rejected", false, strPtr1867("student"), 700, false, "face_not_verified"},
		{"work identity rejected", true, strPtr1867("work"), 700, false, "identity_not_student_or_teacher"},
		{"nil identity rejected", true, nil, 700, false, "identity_not_student_or_teacher"},
		{"low credit rejected", true, strPtr1867("student"), 500, false, "credit_below_threshold"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := seed1867User(t, db, tenantID, orgID, uuid.New().String(), tc.faceOK, tc.idType, tc.credit)
			got := EvaluateDepositWaiverEligibility(db, userID)
			assert.Equal(t, tc.wantOK, got.Eligible)
			if tc.wantReason == "" {
				assert.Empty(t, got.Reasons)
			} else {
				assert.Contains(t, got.Reasons, tc.wantReason)
			}
			assert.Equal(t, 600, got.MinCreditScore)
		})
	}
}

// Eligibility endpoint: guests get 40001, users get a structured verdict.
func TestDepositWaiverEligibilityEndpoint(t *testing.T) {
	db, _ := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1867a2b3c4d5")

	sub := uuid.New().String()
	seed1867User(t, db, tenantID, orgID, sub, true, strPtr1867("work"), 700)

	router := new1867Router(NewUserRentalHandler(), tenantID, orgID, sub)
	req := httptest.NewRequest(http.MethodGet, "/api/user/deposit-waiver/eligibility", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Eligible bool     `json:"eligible"`
			Reasons  []string `json:"reasons"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.False(t, resp.Data.Eligible)
	assert.Contains(t, resp.Data.Reasons, "identity_not_student_or_teacher")

	// Guest (no user id in context) → 401/40001
	guestRouter := new1867Router(NewUserRentalHandler(), tenantID, orgID, "")
	req2 := httptest.NewRequest(http.MethodGet, "/api/user/deposit-waiver/eligibility", nil)
	w2 := httptest.NewRecorder()
	guestRouter.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

// CreateOrder gate: ineligible → 40301, missing letter → 40002, valid passes
// and persists letter + zeroed deposit + guarantor links.
func TestCreateOrder_DepositWaiverGate(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1867b3c4d5e6")
	iamSub := uuid.New().String()

	instID := seed1867Instrument(t, db, tenantID, orgID)

	// 3a: work identity → 403/40301 (user row seeded with work type)
	seed1867User(t, db, tenantID, orgID, iamSub, true, strPtr1867("work"), 700)
	router := new1867Router(handler, tenantID, orgID, iamSub)
	httpCode, bizCode, _ := post1867JSON(t, router, "/api/user/orders", map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
		"deposit_waived": true, "guarantor_ids": []string{uuid.New().String(), uuid.New().String()},
		"recommendation_letter": "/uploads/letter.jpg",
	})
	assert.Equal(t, http.StatusForbidden, httpCode)
	assert.Equal(t, 40301, bizCode)

	// 3b: eligible user, no letter → 400/40002
	db.Exec(`DELETE FROM users WHERE iam_sub = ?`, iamSub)
	seed1867User(t, db, tenantID, orgID, iamSub, true, strPtr1867("student"), 700)
	httpCode, bizCode, _ = post1867JSON(t, router, "/api/user/orders", map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
		"deposit_waived": true, "guarantor_ids": []string{uuid.New().String(), uuid.New().String()},
		"recommendation_letter": "",
	})
	assert.Equal(t, http.StatusBadRequest, httpCode)
	assert.Equal(t, 40002, bizCode)

	// 3c: fully eligible → 201/20000, order persisted with letter + zero deposit
	g1, g2 := uuid.New().String(), uuid.New().String()
	sub2 := uuid.New().String()
	userID := seed1867User(t, db, tenantID, orgID, sub2, true, strPtr1867("student"), 700)
	require.NoError(t, db.Create(&models.Guarantor{ID: g1, UserID: userID, Name: "G1", Phone: "13800000001"}).Error)
	require.NoError(t, db.Create(&models.Guarantor{ID: g2, UserID: userID, Name: "G2", Phone: "13800000002"}).Error)

	router2 := new1867Router(handler, tenantID, orgID, sub2)
	httpCode, bizCode, data := post1867JSON(t, router2, "/api/user/orders", map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
		"deposit_waived": true, "guarantor_ids": []string{g1, g2},
		"recommendation_letter": "/uploads/letter-happy.jpg",
	})
	require.Equal(t, http.StatusCreated, httpCode)
	require.Equal(t, 20000, bizCode)

	orderID := data["order_id"].(string)
	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	assert.True(t, order.DepositWaived)
	assert.Equal(t, "/uploads/letter-happy.jpg", order.RecommendationLetter)
	assert.Equal(t, models.Cents(0), order.Deposit, "deposit must be zeroed for waived orders")

	var linkCount int64
	db.Model(&models.OrderGuarantor{}).Where("order_id = ?", orderID).Count(&linkCount)
	assert.Equal(t, int64(2), linkCount)
}

// BatchCreateOrder shares the same gate (#1867).
func TestBatchCreateOrder_DepositWaiverGate(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1867c4d5e6f7")

	instID := seed1867Instrument(t, db, tenantID, orgID)

	// 4a: ineligible → 403/40301
	iamSub := uuid.New().String()
	seed1867User(t, db, tenantID, orgID, iamSub, true, strPtr1867("other"), 900)
	router := new1867Router(handler, tenantID, orgID, iamSub)
	httpCode, bizCode, _ := post1867JSON(t, router, "/api/user/orders/batch", map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20"},
		},
		"deposit_waived":        true,
		"guarantor_ids":         []string{uuid.New().String(), uuid.New().String()},
		"recommendation_letter": "/uploads/letter.jpg",
	})
	assert.Equal(t, http.StatusForbidden, httpCode)
	assert.Equal(t, 40301, bizCode)

	// 4b: eligible → order persisted with letter and zeroed deposit
	sub := uuid.New().String()
	userID := seed1867User(t, db, tenantID, orgID, sub, true, strPtr1867("teacher"), 700)
	g1, g2 := uuid.New().String(), uuid.New().String()
	require.NoError(t, db.Create(&models.Guarantor{ID: g1, UserID: userID, Name: "G1", Phone: "13800000003"}).Error)
	require.NoError(t, db.Create(&models.Guarantor{ID: g2, UserID: userID, Name: "G2", Phone: "13800000004"}).Error)

	router2 := new1867Router(handler, tenantID, orgID, sub)
	httpCode, bizCode, _ = post1867JSON(t, router2, "/api/user/orders/batch", map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20"},
		},
		"deposit_waived":        true,
		"guarantor_ids":         []string{g1, g2},
		"recommendation_letter": "/uploads/letter-batch.jpg",
	})
	require.Equal(t, 20000, bizCode)

	var order models.Order
	require.NoError(t, db.Where("user_id = ? AND instrument_id = ?", userID, instID).First(&order).Error)
	assert.True(t, order.DepositWaived)
	assert.Equal(t, "/uploads/letter-batch.jpg", order.RecommendationLetter)
	assert.Equal(t, models.Cents(0), order.Deposit)
}
