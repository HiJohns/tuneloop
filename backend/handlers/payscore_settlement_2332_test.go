package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// fakePayscoreProvider 记录结算侧的支付分动作，不发起任何网络请求。
type fakePayscoreProvider struct {
	created   []string
	completed []string
	cancelled []string
}

func (f *fakePayscoreProvider) CreateOrder(_ context.Context, params wechatpay.PayScoreCreateParams) (*wechatpay.PayScoreOrder, error) {
	f.created = append(f.created, params.OutOrderNo)
	return &wechatpay.PayScoreOrder{OutOrderNo: params.OutOrderNo, Package: "pkg-" + params.OutOrderNo}, nil
}

func (f *fakePayscoreProvider) QueryOrder(_ context.Context, _, outOrderNo string) (*wechatpay.PayScoreOrder, error) {
	return &wechatpay.PayScoreOrder{OutOrderNo: outOrderNo, State: "DOING"}, nil
}

func (f *fakePayscoreProvider) CancelOrder(_ context.Context, _, outOrderNo string) error {
	f.cancelled = append(f.cancelled, outOrderNo)
	return nil
}

func (f *fakePayscoreProvider) CompleteOrder(_ context.Context, _, outOrderNo string) error {
	f.completed = append(f.completed, outOrderNo)
	return nil
}

func (f *fakePayscoreProvider) VerifyPayScoreCallback(wechatpay.PayScoreCallbackHeaders, []byte) (*wechatpay.PayScoreCallback, error) {
	return nil, nil
}

func (f *fakePayscoreProvider) IsMock() bool { return false }

func seedSettlementPayscoreOrder(t *testing.T, depositYuan float64, cashPaidYuan float64, tier, path, creditState, serviceOrderNo string, damageYuan float64) models.Order {
	t.Helper()
	db := testfixtures.GetTestDB(t)
	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "settle-" + userID[:8], Status: "active",
	}).Error)

	returnedAt := time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC) // 23h → 1 day
	order := models.Order{
		ID:                    uuid.New().String(),
		TenantID:              tenantID,
		OrgID:                 orgID,
		UserID:                userID,
		InstrumentID:          uuid.New().String(),
		StartDate:             strPtr1867("2026-08-01"),
		EndDate:               strPtr1867("2026-08-02"),
		LeaseTerm:             1,
		Status:                models.OrderStatusCompleted,
		ReturnedAt:            &returnedAt,
		Deposit:               models.FromYuan(depositYuan),
		CashPaid:              models.FromYuan(cashPaidYuan),
		DepositTier:           tier,
		DepositCreditPath:     path,
		CreditState:           creditState,
		CreditServiceOrderNo:  serviceOrderNo,
		CreditRiskAmountCents: int64(models.FromYuan(300)),
		PricingBreakdown:      strPtr1867(`{"base_daily_rent":3600,"rent_days":1,"pricing_tiers":[{"days_max":1,"discount_percent":0,"daily_rate":3600}],"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}],"total_amount":3600}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-PS-" + order.ID[:8], StockStatus: "rented",
	}).Error)
	if damageYuan > 0 {
		require.NoError(t, db.Create(&models.DamageReport{
			ID: uuid.New().String(), LeaseID: order.ID, TenantID: tenantID, OrgID: orgID,
			InstrumentID: order.InstrumentID, UserID: order.UserID,
			Status: "agreed", DepositDeducted: models.FromYuan(damageYuan),
		}).Error)
	}
	return order
}

// TestSettlementCashVsPayscore（R4）：reduced 档结算时，
// 现金只退「实付押金 − 扣款」，支付分只做 CompleteOrder，不产生额外现金退款。
func TestSettlementCashVsPayscore(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	provider := &fakePayscoreProvider{}
	SetPayscoreSettlementProvider(provider, "SVC1")
	t.Cleanup(func() { SetPayscoreSettlementProvider(nil, "") })

	// 押金 200，租金 36 已付清 → 应退押金 200。
	order := seedSettlementPayscoreOrder(t, 200, 36+200, "reduced", models.DepositCreditPathPayscore,
		services.CreditStateApproved, "so-reduced-1", 0)

	result, err := executeRefund(db, order)
	require.NoError(t, err)
	assert.Equal(t, services.PayscoreActionComplete, result.PayscoreAction)
	assert.InDelta(t, 200.0, result.CashRefundable, 0.01, "现金退款只能来自实付押金")

	var refunds []models.OrderRefundRecord
	require.NoError(t, db.Where("order_id = ?", order.ID).Find(&refunds).Error)
	require.Len(t, refunds, 1, "现金退款只允许一笔")
	assert.Equal(t, models.FromYuan(result.CashRefundable), refunds[0].Amount)

	assert.Equal(t, []string{"so-reduced-1"}, provider.completed)
	assert.Empty(t, provider.cancelled)

	// 负例：支付分担保额度（risk=300）绝不能出现在现金退款里。
	assert.LessOrEqual(t, refunds[0].Amount, models.FromYuan(200),
		"支付分担保金额不得进入现金退款通道")
}

// TestRefundNotForPayscore（R4）：zero 档无现金押金 + 定损 50 →
// 现金退款为 0（不得为负），缺口记为支付分应收取金额；支付分只做 complete。
func TestRefundNotForPayscore(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	provider := &fakePayscoreProvider{}
	SetPayscoreSettlementProvider(provider, "SVC1")
	t.Cleanup(func() { SetPayscoreSettlementProvider(nil, "") })

	// 零押金：现金押金 0，租金 36 已付清；定损 50 无法从押金扣除。
	order := seedSettlementPayscoreOrder(t, 0, 36, "zero", models.DepositCreditPathPayscore,
		services.CreditStateApproved, "so-zero-1", 50)

	result, err := executeRefund(db, order)
	require.NoError(t, err)
	assert.Zero(t, result.CashRefundable, "零押金订单不得产生现金退款")
	assert.Equal(t, int64(5000), result.PayscoreDeductCents, "超出押金的扣款应记为支付分应收（分）")

	var refundCount int64
	require.NoError(t, db.Model(&models.OrderRefundRecord{}).
		Where("order_id = ?", order.ID).Count(&refundCount).Error)
	assert.Zero(t, refundCount, "支付分担保不得触发现金退款记录")

	assert.Equal(t, []string{"so-zero-1"}, provider.completed)
	assert.Equal(t, services.PayscoreActionComplete, result.PayscoreAction)
}

// TestRefundPayscoreCancel：未履约（订单取消）→ CancelOrder，且不发起结算退款。
func TestRefundPayscoreCancel(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	provider := &fakePayscoreProvider{}
	SetPayscoreSettlementProvider(provider, "SVC1")
	t.Cleanup(func() { SetPayscoreSettlementProvider(nil, "") })

	order := seedSettlementPayscoreOrder(t, 200, 236, "reduced", models.DepositCreditPathPayscore,
		services.CreditStateApproved, "so-cancel-1", 0)
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusCancelled).Error)
	order.Status = models.OrderStatusCancelled

	action := payscoreActionForOrder(order)
	assert.Equal(t, services.PayscoreActionCancel, action)
	require.NoError(t, settlePayscoreGuarantee(context.Background(), order, action))
	assert.Equal(t, []string{"so-cancel-1"}, provider.cancelled)
	assert.Empty(t, provider.completed)
}
