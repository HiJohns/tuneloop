package handlers

// #2282 P4a：续租追加不可变 term_version（version=N），旧快照暂保留。

import (
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderTermVersion_RenewalAppend_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	start, end := "2026-06-01", "2026-07-01"
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
		StartDate: &start, EndDate: &end, CashPaid: models.FromYuan(1000)}).Error)
	require.NoError(t, db.Create(&models.OrderTermVersion{OrderID: orderID, Version: 1,
		StartDate: &start, EndDate: &end, Days: 30, TierSegments: "[]", TotalAmount: models.FromYuan(3000)}).Error)

	days := 30
	rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "renewal", Amount: models.FromYuan(1000), Type: "payment", Status: "paid", Days: &days}
	require.NoError(t, db.Create(&rec).Error)
	require.NoError(t, applySideEffects(db, &rec, time.Now()))

	var versions []models.OrderTermVersion
	require.NoError(t, db.Where("order_id = ?", orderID).Order("version").Find(&versions).Error)
	require.Len(t, versions, 2, "续租必须追加 version=2")
	v2 := versions[1]
	assert.Equal(t, 2, v2.Version)
	assert.Equal(t, 30, v2.Days)
	assert.Equal(t, models.FromYuan(1000), v2.TotalAmount)
	require.NotNil(t, v2.StartDate)
	assert.True(t, len(*v2.StartDate) >= 10 && (*v2.StartDate)[:10] == "2026-07-01", "新版本起点=旧结束日")
	require.NotNil(t, v2.EndDate)
	assert.True(t, len(*v2.EndDate) >= 10 && (*v2.EndDate)[:10] == "2026-07-31")
	assert.NotNil(t, v2.PaymentRecordID)
	assert.Equal(t, rec.ID, *v2.PaymentRecordID)
}

// TestOrderTermVersion_MultiRenewalImmutable_2282：30+30+30 多次续租 →
// 版本链正确、pricing_breakdown 逐字节不变。
func TestOrderTermVersion_MultiRenewalImmutable_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	start, end := "2026-06-01", "2026-07-01"
	pb := `{"base_daily_rent":10000,"rent_days":30,"final_daily_rent":10000,"total_amount":3000,` +
		`"pricing_tiers":[{"days_max":30,"discount_percent":0},{"days_max":180,"discount_percent":5}],` +
		`"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}]}`
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	order := models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
		StartDate: &start, EndDate: &end, CashPaid: models.FromYuan(3000), PricingBreakdown: &pb}
	require.NoError(t, db.Create(&order).Error)
	insertInitialTermVersion(db, &order)

	var before string
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", orderID).Select("pricing_breakdown").Scan(&before).Error)

	for i := 1; i <= 3; i++ {
		days := 30
		rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
			OrderType: "renewal", Amount: models.FromYuan(3000), Type: "payment", Status: "paid", Days: &days}
		require.NoError(t, db.Create(&rec).Error)
		require.NoError(t, applySideEffects(db, &rec, time.Now()))
	}

	var versions []models.OrderTermVersion
	require.NoError(t, db.Where("order_id = ?", orderID).Order("version").Find(&versions).Error)
	require.Len(t, versions, 4, "v1 + 3 次续租")
	sumDays, sumCents := 0, models.Cents(0)
	for _, v := range versions {
		sumDays += v.Days
		sumCents += v.TotalAmount
	}
	assert.Equal(t, 120, sumDays, "30+30+30+30 覆盖天数")
	assert.Equal(t, models.FromYuan(12000), sumCents)
	assert.Equal(t, "2026-07-01", (*versions[1].StartDate)[:10])
	assert.Equal(t, "2026-07-31", (*versions[1].EndDate)[:10])
	assert.Equal(t, "2026-07-31", (*versions[2].StartDate)[:10])
	assert.Equal(t, "2026-08-30", (*versions[2].EndDate)[:10])
	assert.Equal(t, "2026-08-30", (*versions[3].StartDate)[:10])
	assert.Equal(t, "2026-09-29", (*versions[3].EndDate)[:10])
	assert.NotEqual(t, "[]", versions[1].TierSegments, "续租版本必须记录档位段")

	var after string
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", orderID).Select("pricing_breakdown").Scan(&after).Error)
	assert.Equal(t, before, after, "pricing_breakdown 创建后不得被续租改写")
}
