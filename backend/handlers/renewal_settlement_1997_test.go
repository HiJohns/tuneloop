package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1997 Bug ①：逾期 N 天必须只能续期 N 天（而非 N+1）。
// 逾期覆盖天数口径 = overdueDays（end_date → 昨天，含终点日）；minAdditionalDays
// 之前用 CalculateDays(endDate, today) 会多算 1 天 → 强制续 N+1 且费用按 N+1 计。
func TestRenewal1997_Overdue_MinDaysNotOffByOne(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	tenantID := "00000000-0000-4000-8000-0000000197a1"
	userID := "00000000-0000-4000-8000-0000000197a2"
	orgID := "00000000-0000-4000-8000-0000000197a3"
	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, -4) // 逾期 4 天

	router := renewalRouter(tenantID, userID)
	postCalc := func(days int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"additional_days": days})
		req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/calculate", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// 逾期 4 天 → 最少续 4 天即可让 new_end_date = 今天（此前强制 5 天）。
	w := postCalc(4)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			MinAdditionalDays int     `json:"min_additional_days"`
			OverdueDays       int     `json:"overdue_days"`
			NewEndDate        string  `json:"new_end_date"`
			RenewalCost       float64 `json:"renewal_cost"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, resp.Data.OverdueDays, resp.Data.MinAdditionalDays,
		"min_additional_days 必须等于逾期天数（不再 +1）")
	require.Equal(t, 4, resp.Data.MinAdditionalDays)

	// new_end_date = end_date + 4 = 今天（连续，无缺口）
	require.Equal(t, time.Now().Format("2006-01-02"), resp.Data.NewEndDate)

	// 逾期 3 天（< min 4）→ 拒绝
	require.Equal(t, http.StatusBadRequest, postCalc(3).Code)
}

// #1997 Bug ②：续期后立即退租——实际占用天数（含逾期）必须按序消费续期段，
// 续期费只退「未消费」部分（全部消费则 0），不得整笔退回。
func TestRenewal1997_Settlement_ChargesConsumedRenewalDays(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageReport{}, &models.OrderPaymentRecord{}))

	tenantID := "00000000-0000-4000-8000-0000000197b1"
	userID := "00000000-0000-4000-8000-0000000197b2"
	orgID := "00000000-0000-4000-8000-0000000197b3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "1997-" + uuid.New().String()[:8],
		BaseDailyRate: models.ToCentsPtr(float64Ptr(10)),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	// 10 天合同（¥10/天），已逾期 4 天：delivered 13 天前（交付当天不计满 1 天，
	// CalculateLeaseDays 取 ceil → 实际 14 天），原 end_date 4 天前。
	delivered := time.Now().AddDate(0, 0, -13)
	startDate := delivered.Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, -4).Format("2006-01-02")
	pricingBreakdown := `{"base_daily_rent":1000,"rent_days":10,"total_amount":100000,` +
		`"pricing_tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000}],` +
		`"tier_segments":[{"tier":1,"days":10,"rate":1000,"discount":1,"subtotal":100000}]}`

	order := models.Order{
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		StartDate:        &startDate,
		EndDate:          &endDate,
		LeaseTerm:        10,
		Status:           models.OrderStatusInLease,
		DeliveredAt:      &delivered,
		CashPaid:         models.FromYuan(100), // 合同 10 天 × ¥10
		PricingBreakdown: &pricingBreakdown,
	}
	require.NoError(t, db.Create(&order).Error)

	// 续期 4 天（覆盖逾期），支付 ¥40。
	additionalDays := 4
	outTradeNo := "RN-1997-" + uuid.New().String()[:8]
	meta := renewalMetadata{AdditionalDays: additionalDays, OrderID: order.ID, OutTradeNo: outTradeNo}
	metaJSON, _ := json.Marshal(meta)
	metaStr := string(metaJSON)
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", OutTradeNo: &outTradeNo,
		Amount: models.FromYuan(40), Type: "payment", Status: "paid",
		RawResponse: &metaStr, Days: &additionalDays,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&rec).Error)

	tx := db.Begin()
	require.NoError(t, applyRenewalSideEffects(tx, &rec, time.Now()))
	require.NoError(t, tx.Commit().Error)

	// 立即归还。
	returned := time.Now()
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Updates(map[string]interface{}{"status": "returned", "returned_at": returned}).Error)

	var reloaded models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&reloaded).Error)

	result := computeSettlement(reloaded, db)
	t.Logf("actualDays=%d rentPayable=%.2f totalRentPaid=%.2f refund=%.2f shortfall=%.2f segmentModel=%v",
		result.ActualDays, result.RentPayable, result.TotalRentPaid, result.TotalRefund, result.PayableShortfall, result.SegmentModel)

	// 合同 10 天 + 续期 4 天 = 覆盖 14 天；实际占用从交付至今 14 天 → 续期段全部
	// 被消费 → 续期费不得退回（退款仅可能来自押金，而押金为 0）。
	require.Equal(t, 14, result.ActualDays)
	require.True(t, result.SegmentModel, "段级模型必须生效（不得回退 #1743）")
	require.InDelta(t, 140.0, result.TotalRentPaid, 0.001, "合同 ¥100 + 续期 ¥40")
	require.InDelta(t, 0.0, result.TotalRefund, 0.001, "全部续期天数已消费 → 不退款")
	require.InDelta(t, 0.0, result.PayableShortfall, 0.001)
}

// #1997 Bug② 兜底：pricing_breakdown.tier_segments 未随续期扩展（存量快照）
// 且段模型因 actualDays > cover 被拒时，#1743 兜底不得按过期的 Σsegment 少计
// 续期消费天数（否则续期费整笔退回）。
func TestRenewal1997_Fallback_StaleTierSegmentsChargesRenewal(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.DamageReport{}, &models.OrderPaymentRecord{}))

	tenantID := "00000000-0000-4000-8000-0000000197c1"
	userID := "00000000-0000-4000-8000-0000000197c2"
	orgID := "00000000-0000-4000-8000-0000000197c3"

	instrument := models.Instrument{
		TenantID: tenantID, OrgID: &orgID, SN: "1997c-" + uuid.New().String()[:8],
		BaseDailyRate: models.ToCentsPtr(float64Ptr(10)), StockStatus: "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	delivered := time.Now().AddDate(0, 0, -14) // 逾期 4 天 → actualDays 15（ceil）
	startDate := delivered.Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, -4).Format("2006-01-02")
	// rent_days=14（合同 10 + 续期 4），但 tier_segments 过期仍为 10 天。
	stalePB := `{"base_daily_rent":1000,"rent_days":14,"total_amount":140000,` +
		`"pricing_tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000}],` +
		`"tier_segments":[{"tier":1,"days":10,"rate":1000,"discount":1,"subtotal":100000}]}`

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instrument.ID,
		StartDate: &startDate, EndDate: &endDate, LeaseTerm: 10,
		Status: models.OrderStatusInLease, DeliveredAt: &delivered,
		CashPaid: models.FromYuan(140), PricingBreakdown: &stalePB,
	}
	require.NoError(t, db.Create(&order).Error)

	days := 4
	otn := "RN-1997c-" + uuid.New().String()[:8]
	meta, _ := json.Marshal(renewalMetadata{AdditionalDays: days, OrderID: order.ID, OutTradeNo: otn})
	metaStr := string(meta)
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", OutTradeNo: &otn,
		Amount: models.FromYuan(40), Type: "payment", Status: "paid",
		RawResponse: &metaStr, Days: &days, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&rec).Error)

	returned := time.Now()
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Updates(map[string]interface{}{"status": "returned", "returned_at": returned}).Error)
	var reloaded models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&reloaded).Error)

	result := computeSettlement(reloaded, db)
	t.Logf("fallback: actualDays=%d rentPayable=%.2f paid=%.2f refund=%.2f segmentModel=%v",
		result.ActualDays, result.RentPayable, result.TotalRentPaid, result.TotalRefund, result.SegmentModel)

	require.False(t, result.SegmentModel, "本用例应触发 #1743 兜底（actualDays > cover）")
	require.InDelta(t, 140.0, result.RentPayable, 0.001, "兜底须按 rent_days 覆盖天数计费（含续期）")
	require.InDelta(t, 0.0, result.TotalRefund, 0.001, "续期天数已消费 → 不得整笔退回")
}
