package handlers

// #1847 regression tests: ConfirmDelivery lease-window semantics.
//
// N rental days cover [delivered, delivered+N-1] — the same convention as
// CreateOrder's CalculateEndDate and LeaseInfo's 预期归还 (start+N).
// Covered-days resolution order: ① pricing_breakdown.rent_days ② stored
// date window CalculateDays(sd, ed) with NO end>start gate (1-day leases
// have end==start and CalculateDays returns 1) ③ neither available →
// 1 day + loud warning. The old implementation hardcoded a 30-day
// fallback and added +N instead of +N-1, silently turning 1-day rentals
// into 30-day windows (order 16fdfb81: rented 1 day + renewed 1 day,
// end_date became 2026-10-09 instead of 2026-09-09).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

func setupDeliveryOrder(t *testing.T, pb *string, startDate, endDate *string) (string, string, string, string) {
	t.Helper()
	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "DLV-" + uuid.New().String()[:8],
		BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus:   "shipped",
	}
	require.NoError(t, db.Create(&instrument).Error)

	order := models.Order{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		Status:           models.OrderStatusShipped,
		StartDate:        startDate,
		EndDate:          endDate,
		PricingBreakdown: pb,
	}
	require.NoError(t, db.Create(&order).Error)

	return tenantID, orgID, userID, order.ID
}

func confirmDelivery(t *testing.T, actor testutil.TestActor, orderID string, deliveredAt time.Time) {
	t.Helper()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/delivery", (&WarehouseHandler{}).ConfirmDelivery)

	body, _ := json.Marshal(map[string]interface{}{
		"delivered_at": deliveredAt,
		"photos":       []string{"/uploads/media/test-delivery.jpg"}, // #1806: photos required
	})
	req := httptest.NewRequest(http.MethodPut, "/api/warehouse/orders/"+orderID+"/delivery", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func loadOrderDates(t *testing.T, orderID string) (string, string) {
	t.Helper()
	var order models.Order
	require.NoError(t, database.GetDB().Where("id = ?", orderID).First(&order).Error)
	require.NotNil(t, order.StartDate)
	require.NotNil(t, order.EndDate)
	// DATE columns may scan back as RFC3339 ("2026-09-08T00:00:00Z") depending
	// on the driver — compare the date part only (same as formatDisplayDate).
	clean := func(v string) string {
		if i := strings.IndexByte(v, 'T'); i > 0 {
			return v[:i]
		}
		return v
	}
	return clean(*order.StartDate), clean(*order.EndDate)
}

// TestConfirmDelivery_OneDayLease_EndEqualsDeliveryDate: a 1-day rental has
// end==start at creation; the old ed.After(sd) gate discarded the window and
// fell back to 30 days. The delivered date IS the last lease day (+N-1, N=1).
func TestConfirmDelivery_OneDayLease_EndEqualsDeliveryDate(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	pb := `{"base_daily_rent":10000,"rent_days":1}`
	start := "2026-09-08"
	tenantID, _, userID, orderID := setupDeliveryOrder(t, &pb, &start, &start)

	deliveredAt := time.Date(2026, 9, 8, 12, 56, 0, 0, time.UTC)
	confirmDelivery(t, testutil.MakeCustomer(tenantID, userID), orderID, deliveredAt)

	gotStart, gotEnd := loadOrderDates(t, orderID)
	require.Equal(t, "2026-09-08", gotStart)
	require.Equal(t, "2026-09-08", gotEnd, "1-day lease window ends on the delivery date (delivered + N - 1, N=1)")
}

// TestConfirmDelivery_MultiDay_UsesRentDaysMinusOne: pb.rent_days is the
// authoritative covered-days source; end = delivered + N - 1 (was +N).
func TestConfirmDelivery_MultiDay_UsesRentDaysMinusOne(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	pb := `{"base_daily_rent":10000,"rent_days":7}`
	start := "2026-08-01"
	end := "2026-08-07"
	tenantID, _, userID, orderID := setupDeliveryOrder(t, &pb, &start, &end)

	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	confirmDelivery(t, testutil.MakeCustomer(tenantID, userID), orderID, deliveredAt)

	_, gotEnd := loadOrderDates(t, orderID)
	require.Equal(t, "2026-08-07", gotEnd, "end = delivered + N - 1")
}

// TestConfirmDelivery_NoPricingBreakdown_FallsBackToDateWindow: without pb,
// the stored date window still resolves the covered days (CalculateDays).
func TestConfirmDelivery_NoPricingBreakdown_FallsBackToDateWindow(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	start := "2026-08-01"
	end := "2026-08-05" // 5-day window
	tenantID, _, userID, orderID := setupDeliveryOrder(t, nil, &start, &end)

	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	confirmDelivery(t, testutil.MakeCustomer(tenantID, userID), orderID, deliveredAt)

	_, gotEnd := loadOrderDates(t, orderID)
	require.Equal(t, "2026-08-05", gotEnd, "end = delivered + CalculateDays(sd,ed) - 1 = delivered + 4")
}

// TestConfirmDelivery_MissingBoth_AssumesOneDay: neither pb nor parseable
// dates → 1 day + warning log (never the old silent 30-day expansion).
func TestConfirmDelivery_MissingBoth_AssumesOneDay(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID, _, userID, orderID := setupDeliveryOrder(t, nil, nil, nil)

	deliveredAt := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	confirmDelivery(t, testutil.MakeCustomer(tenantID, userID), orderID, deliveredAt)

	gotStart, gotEnd := loadOrderDates(t, orderID)
	require.Equal(t, "2026-09-08", gotStart)
	require.Equal(t, "2026-09-08", gotEnd, "1-day assumption: end = delivered date")
}

// TestConfirmDelivery_ThenRenewal_ChainsCorrectly: the 16fdfb81 shape —
// 1-day rental delivered 09-08, then a 1-day renewal must produce
// end_date 09-09 (the bug produced 10-09 via the 30-day fallback).
func TestConfirmDelivery_ThenRenewal_ChainsCorrectly(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	testfixtures.SetupWechatPayMock(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	require.NoError(t, database.GetDB().Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "dlv-renew", WxOpenid: "mock-openid-dlv", Status: "active",
	}).Error)

	pb := `{"base_daily_rent":1000,"rent_days":1,"pricing_tiers":[{"days_max":30,"daily_rate":1000,"discount_percent":0}]}`
	start := "2026-09-08"

	db := database.GetDB()
	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "DLV-CHAIN-" + uuid.New().String()[:6],
		BaseDailyRate: models.ToCentsPtr(float64Ptr(10)),
		Pricing:       `{"daily_rent":10,"overdue_daily_fee":15,"tiers":[{"days_max":30,"daily_rate":10},{"days_max":-1,"daily_rate":9}]}`,
		StockStatus:   "shipped",
	}
	require.NoError(t, db.Create(&instrument).Error)

	order := models.Order{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		Status:           models.OrderStatusShipped,
		StartDate:        &start,
		EndDate:          &start,
		PricingBreakdown: &pb,
	}
	require.NoError(t, db.Create(&order).Error)

	actor := testutil.MakeCustomer(tenantID, userID)
	deliveredAt := time.Date(2026, 9, 8, 12, 56, 0, 0, time.UTC)
	confirmDelivery(t, actor, order.ID, deliveredAt)
	_, gotEnd := loadOrderDates(t, order.ID)
	require.Equal(t, "2026-09-08", gotEnd)

	// Renewal +1 day via the payment-callback side effects (the exact code
	// path the wechat callback runs) — end chains from the corrected date.
	days := 1
	record := &models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", Type: "payment", Status: "paid",
		Amount: 1000, Days: &days,
	}
	require.NoError(t, database.GetDB().Transaction(func(tx *gorm.DB) error {
		return applyRenewalSideEffects(tx, record, time.Now())
	}))

	_, gotEnd = loadOrderDates(t, order.ID)
	require.Equal(t, "2026-09-09", gotEnd, "renewal +1 chains from the corrected end date")
}

// TestConfirmDelivery_LateEveningUTC_UsesBeijingNextDay (#1857): frontend
// delivers delivered_at as a UTC string (toISOString). The lease-window
// calendar dates must follow the business timezone Asia/Shanghai
// (time.Local set in main.go) — a 01:14 Beijing sign-off (17:14Z previous
// UTC day) must NOT be dated one day early by the UTC calendar.
func TestConfirmDelivery_LateEveningUTC_UsesBeijingNextDay(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	pb := `{"base_daily_rent":10000,"rent_days":1}`
	start := "2026-09-08"
	tenantID, _, userID, orderID := setupDeliveryOrder(t, &pb, &start, &start)

	// Business timezone is Asia/Shanghai in production (main.go); the Go
	// test environment defaults to UTC, so pin the local zone for this case.
	origLocal := time.Local
	sh, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	time.Local = sh
	t.Cleanup(func() { time.Local = origLocal })

	// 2026-09-08T16:30:00Z == 2026-09-09 00:30 Beijing.
	deliveredAt := time.Date(2026, 9, 8, 16, 30, 0, 0, time.UTC)
	confirmDelivery(t, testutil.MakeCustomer(tenantID, userID), orderID, deliveredAt)

	gotStart, gotEnd := loadOrderDates(t, orderID)
	require.Equal(t, "2026-09-09", gotStart, "start_date = Beijing calendar day of delivery, not UTC")
	require.Equal(t, "2026-09-09", gotEnd, "1-day lease ends on the Beijing delivery date")
}
