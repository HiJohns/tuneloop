package handlers

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #2327 D13: 下单信用路径（internal / payscore）。
//   - internal（默认）保持 #1867 资格 + 推荐信 + 2 担保人；
//   - payscore 走支付分：不要求内部资格/推荐信，reduced 仍需 2 位本人担保人；
//   - payscore + zero 允许免押金（押金由支付分担保）。
func TestPayscorePath(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2332a1b2c3d4")
	sub := uuid.New().String()
	// 该用户未实名/无内部信用：internal 路径必然不通过，用于区分两条路径。
	userID := seed1867User(t, db, tenantID, orgID, sub, true, nil, false, 0)
	guarantors := seedEligibleGuarantors(t, db, userID)
	router := new1867Router(handler, tenantID, orgID, sub)

	t.Run("payscore reduced 无推荐信可下单", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "deposit_credit_path": "payscore",
			"guarantor_ids": guarantors,
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierReduced, order.DepositTier)
		assert.Equal(t, models.DepositCreditPathPayscore, order.DepositCreditPath)
		assert.Equal(t, models.FromYuan(200), order.Deposit)
	})

	t.Run("payscore reduced 缺担保人仍拒绝", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		httpCode, _, _ := post1867JSON(t, router, "/api/user/orders", map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "deposit_credit_path": "payscore",
			"guarantor_ids": []string{guarantors[0]},
		})
		assert.Equal(t, http.StatusBadRequest, httpCode)
	})

	t.Run("internal reduced 无推荐信拒绝", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		httpCode, _, _ := post1867JSON(t, router, "/api/user/orders", map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "deposit_credit_path": "internal",
			"guarantor_ids": guarantors,
		})
		assert.Equal(t, http.StatusForbidden, httpCode, "internal 路径仍要求 #1867 资格")
	})

	t.Run("payscore zero 档免押金", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "zero", "deposit_credit_path": "payscore",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierZero, order.DepositTier)
		assert.Equal(t, models.DepositCreditPathPayscore, order.DepositCreditPath)
		assert.Equal(t, models.Cents(0), order.Deposit)
		require.NotNil(t, order.DepositFull)
		assert.Equal(t, models.FromYuan(500), *order.DepositFull, "免押金额按全额押金快照")
	})

	t.Run("非法 deposit_credit_path 拒绝", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		httpCode, _, _ := post1867JSON(t, router, "/api/user/orders", map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_credit_path": "vip",
		})
		assert.Equal(t, http.StatusBadRequest, httpCode)
	})
}
