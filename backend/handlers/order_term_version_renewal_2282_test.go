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
