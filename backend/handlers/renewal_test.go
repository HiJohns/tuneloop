package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// setupRenewalOrder creates an overdue order with instrument pricing for renewal tests.
// endDateOffset: days from today (negative = overdue).
func setupRenewalOrder(t *testing.T, tenantID, userID, orgID string, endDateOffset int) (string, string) {
	t.Helper()
	db := database.GetDB()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "RN-" + fmt.Sprint(time.Now().UnixNano()),
		BaseDailyRate: models.ToCentsPtr(float64Ptr(10)),
		Pricing:       `{"daily_rent":10,"overdue_daily_fee":15,"tiers":[{"days_max":30,"daily_rate":10},{"days_max":-1,"daily_rate":9}]}`,
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	endDate := time.Now().AddDate(0, 0, endDateOffset).Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, endDateOffset-10).Format("2006-01-02")
	pricingBreakdown := `{"base_daily_rent":10,"rent_days":10,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10}]}`

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        10,
		Status:           models.OrderStatusInLease,
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	return instrument.ID, order.ID
}

func renewalRouter(tenantID, userID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/orders/:id/renewal/calculate", CalculateRenewal)
	router.POST("/api/orders/:id/renewal/confirm", ConfirmRenewal)
	return router
}

// TestRenewal_Overdue_MinDaysValidation verifies #1491: overdue renewal must
// cover the overdue period (min = today - endDate), new end date = endDate + days.
func TestRenewal_Overdue_MinDaysValidation(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-0000-0000-0000000000a1"
	userID := "00000000-0000-0000-0000-0000000000a2"
	orgID := "00000000-0000-0000-0000-0000000000a3"
	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, -4)

	router := renewalRouter(tenantID, userID)

	postCalc := func(additionalDays int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"additional_days": additionalDays})
		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/calculate", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// additional_days < min (4) → 40002
	w := postCalc(2)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	require.Equal(t, 40002, errResp.Code)

	// additional_days >= min → 20000, new_end_date = endDate + days
	w2 := postCalc(10)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			RenewalCost       float64 `json:"renewal_cost"`
			OverdueBalance    float64 `json:"overdue_balance"`
			TotalAmount       float64 `json:"total_amount"`
			NewEndDate        string  `json:"new_end_date"`
			MinAdditionalDays int     `json:"min_additional_days"`
			OverdueDailyRate  float64 `json:"overdue_daily_rate"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 5, resp.Data.MinAdditionalDays, "min_additional_days = today - endDate inclusive = 5")
	require.Zero(t, resp.Data.OverdueBalance, "no overdue balance in renewal")
	require.Zero(t, resp.Data.OverdueDailyRate, "no overdue daily rate in renewal")
	require.Equal(t, resp.Data.TotalAmount, resp.Data.RenewalCost, "total = renewal cost only (no overdue fee)")

	// new_end_date = endDate + 10 = (today-4) + 10 = today+6
	expectedEnd := time.Now().AddDate(0, 0, 6).Format("2006-01-02")
	require.Equal(t, expectedEnd, resp.Data.NewEndDate, "new end date must be endDate + additionalDays")
}

// TestRenewal_NotOverdue_NoMinDays verifies non-overdue renewal has no minimum.
func TestRenewal_NotOverdue_NoMinDays(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-0000-0000-0000000000b1"
	userID := "00000000-0000-0000-0000-0000000000b2"
	orgID := "00000000-0000-0000-0000-0000000000b3"
	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 5)

	router := renewalRouter(tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{"additional_days": 1})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/calculate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			MinAdditionalDays int    `json:"min_additional_days"`
			NewEndDate        string `json:"new_end_date"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Zero(t, resp.Data.MinAdditionalDays, "non-overdue renewal has no minimum")
	expectedEnd := time.Now().AddDate(0, 0, 6).Format("2006-01-02")
	require.Equal(t, expectedEnd, resp.Data.NewEndDate)
}

// TestRenewal_CouponSnapshot (#1744 修正): 续期支付手动输入优惠码 →
// 服务端折扣 + 订单快照回写。ENO 1% → totalAmount 折后、coupon_code/discount 落库。
func TestRenewal_CouponSnapshot(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	testfixtures.SetupWechatPayMock(t)

	db := database.GetDB()
	_ = db.Migrator().DropTable(&models.Coupon{})
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))
	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "ENO", Type: "percent", Value: 10, Active: true,
	}).Error)

	tenantID := "00000000-0000-0000-0000-0000000000b1"
	userID := "00000000-0000-0000-0000-0000000000b2"
	orgID := "00000000-0000-0000-0000-0000000000b3"
	// #1760: ConfirmRenewal resolves openid server-side; give the test user a
	// wx_openid so the JSAPI order path is reached (empty openid would now
	// return 40002 未绑定微信).
	require.NoError(t, database.GetDB().Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "rn-coupon", WxOpenid: "mock-openid-coupon", Status: "active",
	}).Error)
	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 0)

	router := renewalRouter(tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"additional_days": 5,
		"coupon_code":     "ENO",
	})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 快照回写：ENO + 折扣 = 原价 − 折后（分）
	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.NotNil(t, after.CouponCode)
	require.Equal(t, "ENO", *after.CouponCode)
	require.True(t, after.CouponDiscount > 0, "coupon_discount must be written (cents)")

	// ENO 1%：折后支付 = 1% 原价 → 折扣 = 原价 − 1% = 原价 × 99%
	// 从支付记录可推导原价 = amount + discount
	var rec models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND order_type = ?", orderID, "renewal").
		Order("created_at desc").First(&rec).Error)
	original := rec.Amount + after.CouponDiscount
	require.InDelta(t, int64(original)*99/100, int64(after.CouponDiscount), 2,
		"discount ≈ 99% of original renewal cost")
}
