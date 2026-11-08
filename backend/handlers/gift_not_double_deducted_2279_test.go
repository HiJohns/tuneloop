package handlers

// #2279：非 rent 路径（damage/loss）不得再因 gift 扣 cash_paid。

import (
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedGiftBatch(t *testing.T, db *gorm.DB, user string) {
	t.Helper()
	require.NoError(t, db.Create(&models.PointBatch{
		ID: uuid.NewString(), UserID: user, SourceType: "manual",
		AmountCents: models.FromYuan(500), RemainingCents: models.FromYuan(500), AcquiredAt: time.Now(),
	}).Error)
}

func assertGiftFIFO(t *testing.T, db *gorm.DB, user string) {
	t.Helper()
	var batch models.PointBatch
	require.NoError(t, db.Where("user_id = ?", user).First(&batch).Error)
	assert.Equal(t, models.FromYuan(300), batch.RemainingCents, "FIFO 批次余额应减少 200")
	var n int64
	require.NoError(t, db.Model(&models.PointsTransaction{}).
		Where("user_id = ? AND type = ?", user, "gift_used").Count(&n).Error)
	assert.EqualValues(t, 1, n, "gift_used 交易必须仅 1 条")
	var tx models.PointsTransaction
	require.NoError(t, db.Where("user_id = ? AND type = ?", user, "gift_used").First(&tx).Error)
	assert.Equal(t, models.FromYuan(200), tx.Amount)
}

func TestGiftNotDoubleDeducted_ByOrderType_2268(t *testing.T) {
	for _, orderType := range []string{"damage", "loss"} {
		t.Run(orderType, func(t *testing.T) {
			db := testfixtures.SetupTestDB(t)
			org := uuid.NewString()
			user := uuid.NewString()
			require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
			seedGiftBatch(t, db, user)
			order := models.Order{
				ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user,
				InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
				CashPaid: models.FromYuan(500),
			}
			require.NoError(t, db.Create(&order).Error)
			gift := `{"gift_used":20000}`
			rec := models.OrderPaymentRecord{
				ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
				OrderType: orderType, Amount: models.FromYuan(1000), Type: "payment", Status: "paid",
				RawResponse: &gift,
			}
			require.NoError(t, db.Create(&rec).Error)
			require.NoError(t, applySideEffects(db, &rec, time.Now()))

			var after models.Order
			require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
			assert.Equal(t, models.FromYuan(1500), after.CashPaid, "%s：初始 500 + 实收 1000，不得再扣 gift", orderType)
			assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
			assertGiftFIFO(t, db, user)
		})
	}
}

// #2279 返工：renewal / shortfall 的 gift 路径端到端。
func TestGiftNotDoubleDeducted_RenewalShortfall_2268(t *testing.T) {
	t.Run("renewal", func(t *testing.T) {
		db := testfixtures.SetupTestDB(t)
		org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
		seedGiftBatch(t, db, user)
		end := "2026-07-01"
		require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
			InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
			EndDate: &end, CashPaid: models.FromYuan(500)}).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user,
			OrderID: &orderID, OrderType: "rent", Amount: models.FromYuan(500), Type: "payment", Status: "paid"}).Error)
		days := 30
		gift := `{"gift_used":20000}`
		rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
			OrderType: "renewal", Amount: models.FromYuan(1000), Type: "payment", Status: "paid", Days: &days, RawResponse: &gift}
		require.NoError(t, db.Create(&rec).Error)
		var before models.Order
		require.NoError(t, db.Where("id = ?", orderID).First(&before).Error)
		require.Equal(t, models.FromYuan(500), before.CashPaid, "debug: initial cash")
		require.NoError(t, applySideEffects(db, &rec, time.Now()))

		var after models.Order
		require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
		assert.Equal(t, models.FromYuan(1500), after.CashPaid, "renewal：初始 500 + 净额 1000，不得扣 gift")
		assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
		assertGiftFIFO(t, db, user)
		_, _, net, err := RecomputeOrderFinancials(db, orderID)
		require.NoError(t, err)
		assert.Equal(t, after.CashPaid, net, "Paid − Refunded = CashPaid")
	})

	t.Run("payment_shortfall", func(t *testing.T) {
		db := testfixtures.SetupTestDB(t)
		org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
		seedGiftBatch(t, db, user)
		require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
			InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "returned", CashPaid: models.FromYuan(500)}).Error)
		require.NoError(t, db.Create(&models.Settlement{OrderID: orderID, ActualRentAmount: models.FromYuan(1000), RefundStatus: "pending"}).Error)
		gift := `{"gift_used":20000}`
		rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
			OrderType: "payment_shortfall", Amount: models.FromYuan(1000), Type: "payment", Status: "paid", RawResponse: &gift}
		require.NoError(t, db.Create(&rec).Error)
		require.NoError(t, applySideEffects(db, &rec, time.Now()))

		var after models.Order
		require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
		assert.Equal(t, models.FromYuan(1500), after.CashPaid, "shortfall：初始 500 + 补缴 1000，不得扣 gift")
		assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
		assertGiftFIFO(t, db, user)
	})
}
