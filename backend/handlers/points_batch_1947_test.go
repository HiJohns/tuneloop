package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #1947 Sub-D：发放/扣减集成——注册赠点建批次、支付抵扣 FIFO 扣批次并留痕。

func TestPointsBatch_RegistrationGrant(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	user := models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(),
		TenantID: tenantID, OrgID: tenantID,
		Username: "pb-reg", Status: "active", Name: "注册赠点用户",
	}
	require.NoError(t, db.Create(&user).Error)

	grantRegistrationRewards(db, &user, &registerForm{})

	var b models.PointBatch
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&b).Error)
	require.Equal(t, services.PointBatchSourceSignup, b.SourceType)
	require.Equal(t, models.Cents(9900), b.AmountCents, "默认入会赠 99 元")
	require.Equal(t, models.Cents(9900), b.RemainingCents)
	require.NotNil(t, b.ExpiresAt, "普通批次有到期日")

	var u models.User
	require.NoError(t, db.Where("id = ?", user.ID).First(&u).Error)
	require.Equal(t, models.Cents(9900), u.PromoPoints, "快照同步")
}

func TestPointsBatch_DeductOnPayment(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	iamSub := uuid.New().String()
	user := models.User{
		ID: uuid.New().String(), IAMSub: iamSub,
		TenantID: tenantID, OrgID: tenantID,
		Username: "pb-pay", Status: "active", Name: "支付扣点用户",
		PromoPoints: 500,
	}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now()
	require.NoError(t, db.Create(&models.PointBatch{
		ID: uuid.New().String(), UserID: user.ID, SourceType: services.PointBatchSourceSignup,
		AmountCents: 500, RemainingCents: 500, AcquiredAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	raw := `{"gift_used": 200}`
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, UserID: iamSub,
		OrderType: "rent", Amount: 1000, Type: "payment", Status: "paid", RawResponse: &raw,
	}
	require.NoError(t, db.Create(&rec).Error)

	require.NoError(t, deductPointsFromRecord(db, &rec, now))

	var b models.PointBatch
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&b).Error)
	require.Equal(t, models.Cents(300), b.RemainingCents, "FIFO 扣减批次")

	var consCount int64
	db.Model(&models.PointBatchConsumption{}).Where("transaction_id = ?", rec.ID).Count(&consCount)
	require.Equal(t, int64(1), consCount, "扣减留痕")

	var u models.User
	require.NoError(t, db.Where("id = ?", user.ID).First(&u).Error)
	require.Equal(t, models.Cents(300), u.PromoPoints, "快照同步扣减")

	snapshot, batchSum, err := services.ReconcilePoints(db, user.ID)
	require.NoError(t, err)
	require.Equal(t, snapshot, batchSum, "快照 == SUM(未过期 remaining)")
}
