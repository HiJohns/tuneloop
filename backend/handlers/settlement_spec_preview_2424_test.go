package handlers

// #2424 Phase A：spec v2.3 分账户预览接线（不改旧字段语义）。
//
// R1 锁定 #2339 fixture 上的新字段（net 40 / gross 500 / payable 460）与旧字段
// （666.67 / 506.67 / 0）同时成立；R2 由既有结算用例回归。

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// seedSpecPreviewFixture 复刻 #2339 fixture：cash 2000 / gift 1000 / deposit 500，
// 30 天租约、28 天归还、日租 100 → R1=2800、A0=1000、pay_ratio=0.3（A1=840）。
func seedSpecPreviewFixture(t *testing.T) models.Order {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Skip("test db unavailable")
	}

	tenantID := "00000000-0000-4000-8000-0000000000c1"
	orgID := "00000000-0000-4000-8000-0000000000c2"
	userID := "00000000-0000-4000-8000-0000000000c3"

	require.NoError(t, db.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: 1, PayRatio: 0.3, ReferralRatio: 0, IsActive: true,
	}).Error)
	testfixtures.SeedUserFK(t, db, userID, tenantID, orgID)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).
		Update("membership_level_id", 1).Error)

	returnedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SPEC-PREVIEW", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
		StockStatus: "rented",
	}).Error)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     instrumentID,
		StartDate:        strPtr("2026-07-01"),
		EndDate:          strPtr("2026-07-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         models.FromYuan(2000),
		GiftPointsUsed:   models.FromYuan(1000),
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":10000}],"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	testfixtures.SeedOrderParents(t, db, &order)
	require.NoError(t, db.Create(&order).Error)
	return order
}

// R1：Phase A 在 #2339 fixture 上同时满足「新字段 = spec」与「旧字段不变」。
func TestSettlementSpecPreview_2424(t *testing.T) {
	order := seedSpecPreviewFixture(t)
	db := testfixtures.GetTestDB(t)

	result := computeSettlement(order, db)

	// Phase B（D2）：对外字段切换为 spec 语义。
	require.InDelta(t, 660.00, result.TotalRefund, 0.01, "TotalRefund = 乐币面值 160 + 现金毛额 500")
	require.InDelta(t, 500.00, result.CashRefundable, 0.01, "CashRefundable 为毛额")
	require.InDelta(t, 460.00, result.PayableShortfall, 0.01, "PayableShortfall 为毛额")
	require.InDelta(t, 160.00, result.GiftPointsRefunded, 0.01, "Gift 退点 160")
	require.InDelta(t, 1960.00, result.CashBasis, 0.01, "CashBasis = spec.RentCashDue")
	require.InDelta(t, 500.00, result.RemainingDeposit, 0.01, "RemainingDeposit = spec.DepositRefund")

	// 新增字段：spec v2.3 分账户（#2339：net ¥40 / gross ¥500 / payable ¥460）。
	require.InDelta(t, 40.00, result.NetCash, 0.01, "net_cash = ¥40")
	require.InDelta(t, 500.00, result.CashRefundableGross, 0.01, "cash_refundable 毛额 = ¥500")
	require.InDelta(t, 460.00, result.PayableShortfallGross, 0.01, "payable_shortfall 毛额 = ¥460")

	// breakdown 键以分落库，且与结构体字段一致。
	require.EqualValues(t, 4000, result.Breakdown["net_cash"])
	require.EqualValues(t, 50000, result.Breakdown["cash_refundable_gross"])
	require.EqualValues(t, 46000, result.Breakdown["payable_shortfall_gross"])

	t.Logf("PHASE_A_OK net_cash=%.2f gross=%.2f payable=%.2f legacy_total=%.2f legacy_cash=%.2f",
		result.NetCash, result.CashRefundableGross, result.PayableShortfallGross,
		result.TotalRefund, result.CashRefundable)
}

// 派生边界：空押金、押金已扣用、券后租金为 0、预付跨界费（contract_list 小于现金租金）。
func TestDeriveRefundSpecInput_2424(t *testing.T) {
	cases := []struct {
		name        string
		params      refundSpecInputParams
		wantNet     int64
		wantGross   int64
		wantPayable int64
	}{
		{
			name: "无押金",
			params: refundSpecInputParams{
				AfterCouponYuan: 100, CashPaidCents: 10000, DepositCents: 0,
				ContractListCents: 10000, GiftPointsUsedCents: 0, PayRatio: 0.3,
			},
			wantNet: 0, wantGross: 0, wantPayable: 0,
		},
		{
			name: "押金已扣用（other_fee 吃满押金）",
			params: refundSpecInputParams{
				AfterCouponYuan: 100, CashPaidCents: 20000, DepositCents: 10000,
				ContractListCents: 10000, GiftPointsUsedCents: 0,
				OtherFeeYuan: 150, PayRatio: 0.3,
			},
			wantNet: -5000, wantGross: 0, wantPayable: 5000,
		},
		{
			name: "券后租金为 0",
			params: refundSpecInputParams{
				AfterCouponYuan: 0, CashPaidCents: 20000, DepositCents: 10000,
				ContractListCents: 10000, GiftPointsUsedCents: 0, PayRatio: 0.3,
			},
			wantNet: 20000, wantGross: 20000, wantPayable: 0,
		},
		{
			name: "预付跨界费（contract_list < 现金租金）",
			params: refundSpecInputParams{
				AfterCouponYuan: 250, CashPaidCents: 30000, DepositCents: 5000,
				ContractListCents: 20000, GiftPointsUsedCents: 0, PayRatio: 0.3,
			},
			wantNet: 0, wantGross: 5000, wantPayable: 5000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := deriveRefundSpecInput(tc.params)
			require.NoError(t, err)
			got, err := services.ComputeRefundSpec(in)
			require.NoError(t, err)
			require.Equal(t, tc.wantNet, got.NetCashCents, "net cash")
			require.Equal(t, tc.wantGross, got.CashRefundableCents, "gross refundable")
			require.Equal(t, tc.wantPayable, got.PayableShortfallCents, "gross payable")
			require.True(t, got.ConservationOK, "conservation: left=%d right=%d",
				got.ConservationLeftCents, got.ConservationRightCents)
		})
	}
}

// R3：Phase B 字段切换（#2339 核心期望值表）。
func TestSettlementSpecSwitch_2424(t *testing.T) {
	order := seedSpecPreviewFixture(t)
	db := testfixtures.GetTestDB(t)

	result := computeSettlement(order, db)

	require.InDelta(t, 500.00, result.CashRefundable, 0.01, "CashRefundable（毛）")
	require.InDelta(t, 460.00, result.PayableShortfall, 0.01, "PayableShortfall（毛）")
	require.InDelta(t, 40.00, result.NetCash, 0.01, "NetCash（净）")
	require.InDelta(t, 660.00, result.TotalRefund, 0.01, "TotalRefund = 160 点 + 500 现金")
	require.InDelta(t, 160.00, result.GiftPointsRefunded, 0.01, "Gift 退点")
	require.InDelta(t, 1960.00, result.CashBasis, 0.01, "CashBasis")

	// breakdown 同时保留毛额与 net_cash（D3）。
	require.EqualValues(t, 50000, result.Breakdown["cash_refundable_gross"])
	require.EqualValues(t, 46000, result.Breakdown["payable_shortfall_gross"])
	require.EqualValues(t, 4000, result.Breakdown["net_cash"])
	require.EqualValues(t, 50000, result.Breakdown["cash_refundable"])
	require.EqualValues(t, 46000, result.Breakdown["payable_shortfall"])
	require.EqualValues(t, 840, result.Breakdown["gift_cap"])

	t.Logf("SWITCH_OK cash_refundable=%.2f payable=%.2f net=%.2f total=%.2f gift=%.2f cash_basis=%.2f",
		result.CashRefundable, result.PayableShortfall, result.NetCash,
		result.TotalRefund, result.GiftPointsRefunded, result.CashBasis)
}
