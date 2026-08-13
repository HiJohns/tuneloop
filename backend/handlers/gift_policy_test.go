package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

func giftPolicyRouter(actor testutil.TestActor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/admin/gift-policies", ListGiftPolicies)
	router.PUT("/api/admin/gift-policies", UpdateGiftPolicy)
	return router
}

func makeAdminActor() testutil.TestActor {
	return testutil.MakeSiteAdmin(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-0000000000f1",
	)
}

// TestGiftPolicyCRUD: PUT create/update → GET reads back identical fields (#1605).
func TestGiftPolicyCRUD(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 0, PayRatio: 0.3, RefundRatio: 0, IsActive: true,
	}).Error)

	router := giftPolicyRouter(makeAdminActor())

	// PUT create policy for level 1
	body, _ := json.Marshal(map[string]interface{}{
		"level_id": 1, "pay_ratio": 0.5, "refund_ratio": 0.2, "is_active": true,
	})
	req := httptest.NewRequest("PUT", "/api/admin/gift-policies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "create: %s", w.Body.String())

	// PUT update policy for level 1
	body2, _ := json.Marshal(map[string]interface{}{
		"level_id": 1, "pay_ratio": 0.6, "refund_ratio": 0.25, "is_active": true,
	})
	req2 := httptest.NewRequest("PUT", "/api/admin/gift-policies", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, "update: %s", w2.Body.String())

	// GET reads back updated values
	req3 := httptest.NewRequest("GET", "/api/admin/gift-policies", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusOK, w3.Code)
	var resp struct {
		Code int                      `json:"code"`
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data, 2, "default row + level 1 row")

	found := false
	for _, row := range resp.Data {
		if row["level_id"].(float64) == 1 {
			require.Equal(t, 0.6, row["pay_ratio"].(float64))
			require.Equal(t, 0.25, row["refund_ratio"].(float64))
			require.Equal(t, "初级", row["name"].(string))
			found = true
		}
	}
	require.True(t, found, "level 1 policy present in list")
}

// TestGiftPolicyLevelFallback: unconfigured level falls back to default row (#1605).
func TestGiftPolicyLevelFallback(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 0, PayRatio: 0.3, RefundRatio: 0, IsActive: true,
	}).Error)

	// Level 2 has no policy → falls back to default (0.3)
	policy := services.GetGiftPolicyByLevel(db, 2)
	require.NotNil(t, policy)
	require.Equal(t, 0.3, policy.PayRatio)

	// Level 1 with explicit policy → uses own ratio
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.7, RefundRatio: 0.1, IsActive: true,
	}).Error)
	policy1 := services.GetGiftPolicyByLevel(db, 1)
	require.NotNil(t, policy1)
	require.Equal(t, 0.7, policy1.PayRatio)

	// Inactive level policy → falls back to default.
	// NOTE: GORM zero-value issue — IsActive:false is dropped by Create
	// (column default true applies). Use explicit Update.
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 3, PayRatio: 0.9, RefundRatio: 0, IsActive: true,
	}).Error)
	require.NoError(t, db.Model(&models.GiftPolicy{}).
		Where("level_id = ?", 3).Update("is_active", false).Error)
	policy3 := services.GetGiftPolicyByLevel(db, 3)
	require.NotNil(t, policy3)
	require.Equal(t, 0.3, policy3.PayRatio, "inactive policy falls back to default")
}

// TestGiftPolicyAffectsPaymentCalculate: /pay/calculate max_gift_amount
// follows the user's level pay_ratio (#1605, L-05).
func TestGiftPolicyAffectsPaymentCalculate(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	userID := "00000000-0000-4000-8000-0000000000f2"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID,
		TenantID: "00000000-0000-4000-8000-000000000001",
		OrgID:    "00000000-0000-4000-8000-000000000001",
		Username: "giftuser", Status: "active",
		MembershipLevelID: intPtr(1),
		PromoPoints:       1000,
	}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.5, RefundRatio: 0.1, IsActive: true,
	}).Error)

	// GET /pay/calculate is a different handler; call getWalletInfo directly
	info, err := getWalletInfo(db, userID, "00000000-0000-4000-8000-000000000001", 100)
	require.NoError(t, err)
	require.Equal(t, 0.5, info.MaxGiftRatio)
	require.Equal(t, 50.0, info.MaxGiftAmount, "100 × 0.5 = 50")
}

// TestGiftPolicyPermissionGate: request without rebate:manage → 403.
// Uses the real permission registry (mock registry in tests always passes).
func TestGiftPolicyPermissionGate(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db

	// Swap in the real registry so RequireCusPerm("rebate:manage") resolves
	// to a real bit (21) and rejects users lacking it.
	realRegistry := services.NewPermissionRegistry()
	orig := middleware.PermissionRegistry
	middleware.PermissionRegistry = realRegistry
	defer func() { middleware.PermissionRegistry = orig }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Customer with cus_perm=1 (bit 0 only, missing rebate:manage bit 21).
	// cusPerm=0 is treated as unrestricted by RequireCusPerm, so a non-zero
	// value lacking the target bit is required to exercise the 403 branch.
	customerActor := testutil.MakeCustomer(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-0000000000f3")
	customerActor.CusPerm = 1
	router.Use(func(c *gin.Context) {
		ctx := customerActor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/admin/gift-policies", middleware.RequireCusPerm("rebate:manage"), UpdateGiftPolicy)

	body, _ := json.Marshal(map[string]interface{}{
		"level_id": 1, "pay_ratio": 0.3, "refund_ratio": 0,
	})
	req := httptest.NewRequest("PUT", "/api/admin/gift-policies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Customer (no cus_perm) → 403
	require.Equal(t, http.StatusForbidden, w.Code, "customer denied: %s", w.Body.String())
}
