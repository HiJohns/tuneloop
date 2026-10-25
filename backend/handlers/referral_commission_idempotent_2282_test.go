package handlers

// #2282 P4c：referral_commissions 以 order_id 幂等（已存在则不再发批次）。

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferralCommission_Idempotent_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	level2 := 2
	referrer, referee := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: referrer, IAMSub: referrer, TenantID: org, OrgID: org, Status: "active", MembershipLevelID: &level2}).Error)
	require.NoError(t, db.Create(&models.User{ID: referee, IAMSub: referee, TenantID: org, OrgID: org, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{LevelID: level2, PayRatio: 1, ReferralRatio: 0.05, IsActive: true}).Error)
	require.NoError(t, db.Create(&models.Referral{ReferrerID: referrer, RefereeID: referee, RefCode: "REFIDEM2282", Status: "registered"}).Error)

	order := buildMatrixOrder(t, db, matrixCase{baseDaily: 100, days: 30, deposit: 500, discount: 1})
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"user_id": referee, "referral_level_id": level2,
	}).Error)
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Where("order_id = ?", order.ID).Update("user_id", referee).Error)
	// 预置幂等台账（模拟已发放过）
	require.NoError(t, db.Create(&models.ReferralCommission{ID: uuid.NewString(), OrderID: order.ID,
		ReferrerID: referrer, AmountCents: models.FromYuan(150), LevelID: &level2, BasisCents: models.FromYuan(3000)}).Error)

	var loaded models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&loaded).Error)
	_, err := executeRefund(db, loaded)
	require.NoError(t, err)

	var n int64
	require.NoError(t, db.Model(&models.PointBatch{}).
		Where("user_id = ? AND source_type = ?", referrer, services.PointBatchSourceFission).Count(&n).Error)
	assert.Zero(t, n, "已存在返点台账时不得再发批次")
	require.NoError(t, db.Model(&models.ReferralCommission{}).Where("order_id = ?", order.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}
