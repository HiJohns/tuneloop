package handlers

// #2279：非 rent 路径（damage/loss）不得再因 gift 扣 cash_paid。

import (
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiftNotDoubleDeducted_ByOrderType_2268(t *testing.T) {
	for _, orderType := range []string{"damage", "loss"} {
		t.Run(orderType, func(t *testing.T) {
			db := testfixtures.SetupTestDB(t)
			org := uuid.NewString()
			user := uuid.NewString()
			require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
			order := models.Order{
				ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user,
				InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
				CashPaid: 0,
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
			assert.Equal(t, models.FromYuan(1000), after.CashPaid, "%s：实收现金入账，不得再扣 gift", orderType)
			assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
		})
	}
}

// #2279 返工：renewal / shortfall 的 gift 路径端到端。
func TestGiftNotDoubleDeducted_RenewalShortfall_2268(t *testing.T) {
	t.Run("renewal", func(t *testing.T) {
		db := testfixtures.SetupTestDB(t)
		org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
		end := "2026-07-01"
		require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
			InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "in_lease",
			EndDate: &end, CashPaid: 0}).Error)
		days := 30
		gift := `{"gift_used":20000}`
		rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
			OrderType: "renewal", Amount: models.FromYuan(1000), Type: "payment", Status: "paid", Days: &days, RawResponse: &gift}
		require.NoError(t, db.Create(&rec).Error)
		require.NoError(t, applySideEffects(db, &rec, time.Now()))

		var after models.Order
		require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
		assert.Equal(t, models.FromYuan(1000), after.CashPaid, "renewal：现金净额只加一次")
		assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
		paid, refunded, net, err := RecomputeOrderFinancials(db, orderID)
		require.NoError(t, err)
		assert.Equal(t, paid-refunded, net)
		assert.Equal(t, after.CashPaid, net)
	})

	t.Run("payment_shortfall", func(t *testing.T) {
		db := testfixtures.SetupTestDB(t)
		org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
		require.NoError(t, db.Create(&models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
			InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "returned", CashPaid: 0}).Error)
		require.NoError(t, db.Create(&models.Settlement{OrderID: orderID, ActualRentAmount: models.FromYuan(1000), RefundStatus: "pending"}).Error)
		gift := `{"gift_used":20000}`
		rec := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
			OrderType: "payment_shortfall", Amount: models.FromYuan(1000), Type: "payment", Status: "paid", RawResponse: &gift}
		require.NoError(t, db.Create(&rec).Error)
		require.NoError(t, applySideEffects(db, &rec, time.Now()))

		var after models.Order
		require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
		assert.Equal(t, models.FromYuan(1000), after.CashPaid, "shortfall：补缴现金只加一次")
		assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
	})
}
