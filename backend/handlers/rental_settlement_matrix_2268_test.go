package handlers

// #2268（用户要求）：租赁全场景结算矩阵——由简到繁验证各支付组合的结算金额。
// 口径说明：以 `computeSettlement`（生产结算唯一入口）为被测单元；
// 订单/快照/支付记录/定损报告均按生产写入形态构造（breakdown 经 FormatPricingBreakdownJSON）。
//
// 断言两类：
//   1) 具体金额（租金/逾期/退款/补缴）
//   2) 守恒式：CashRefundable - PayableShortfall == 实收(含押金) - (应收租金+逾期+定损+物流)

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type renewSpec struct {
	Days int
	Rate float64
	Paid float64
}

type matrixCase struct {
	name        string
	baseDaily   float64
	days        int
	deposit     float64
	shipping    float64 // 订单物流费（下单/发货时录入，从押金扣）
	addShipping float64 // 归还时追加物流费（AdditionalShippingFee）
	discount    float64 // 档位/优惠码折后率（1=无折扣）
	renewals    []renewSpec
	actualDays  int     // >0 时模拟提前归还（实际租期天数）
	overdue     float64 // 定损报告中的逾期费
	damage      float64 // 定损扣款（DepositDeducted）
	expectRent  float64
	expectRef   float64
	expectPay   float64
}

func buildMatrixOrder(t *testing.T, db *gorm.DB, c matrixCase) models.Order {
	t.Helper()
	tiers := []services.PricingTierConfig{
		{DaysMax: 30, DiscountPercent: 0, DailyRate: c.baseDaily},
		{DaysMax: 180, DiscountPercent: 5, DailyRate: c.baseDaily},
		{DaysMax: -1, DiscountPercent: 50, DailyRate: c.baseDaily},
	}
	totalDays := c.days
	for _, r := range c.renewals {
		totalDays += r.Days
	}
	segs := []services.TierSegment{}
	segs = append(segs, services.TierSegment{
		Tier: 1, Days: c.days, Rate: c.baseDaily, Discount: c.discount,
		Subtotal: c.baseDaily * c.discount * float64(c.days),
	})
	offset := c.days
	for _, r := range c.renewals {
		tier := 2
		if offset >= 180 {
			tier = 3
		}
		segs = append(segs, services.TierSegment{
			Tier: tier, Days: r.Days, Rate: c.baseDaily, Discount: r.Rate,
			Subtotal: c.baseDaily * r.Rate * float64(r.Days),
		})
		offset += r.Days
	}
	rentTotal := float64(0)
	for _, s := range segs {
		rentTotal += s.Subtotal
	}
	pb := &services.PricingBreakdown{
		BaseDailyRent: c.baseDaily,
		RentDays:      totalDays,
		FinalDailyRent: func() float64 {
			if c.discount > 0 {
				return c.baseDaily * c.discount
			}
			return c.baseDaily
		}(),
		TotalAmount:  rentTotal,
		Deposit:      c.deposit,
		ShippingFee:  c.shipping,
		PricingTiers: tiers,
		TierSegments: segs,
		AppliedPolicies: []services.AppliedPolicy{
			{Type: "tier_discount", PlanName: "阶梯折扣"},
		},
	}
	pbJSON := services.FormatPricingBreakdownJSON(pb)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, totalDays-1)
	usedDays := totalDays
	if c.actualDays > 0 && c.actualDays < totalDays {
		usedDays = c.actualDays
	}
	ret := start.AddDate(0, 0, usedDays-1).Add(12 * time.Hour)

	contractRent := c.baseDaily * c.discount * float64(c.days)
	cash := contractRent + c.deposit + c.shipping
	for _, r := range c.renewals {
		cash += r.Paid
	}
	o := models.Order{
		ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: uuid.NewString(),
		UserID: uuid.NewString(), InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: c.days,
		Deposit: models.FromYuan(c.deposit), ShippingFee: models.FromYuan(c.shipping),
		CashPaid: models.FromYuan(cash), Status: "completed",
		StartDate: strPtr(start.Format("2006-01-02")), EndDate: strPtr(end.Format("2006-01-02")),
		DeliveredAt: &start, ReturnedAt: &ret,
		PricingBreakdown: &pbJSON,
	}
	require.NoError(t, db.Create(&o).Error)

	// 首期支付记录（结算按总实收，不依赖单笔顺序；仍落记录以仿真）
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: o.TenantID, UserID: o.UserID,
		OrderID: &o.ID, OrderType: "rent", Amount: models.FromYuan(contractRent + c.deposit + c.shipping),
		Type: "payment", Status: "paid",
	}).Error)

	for _, r := range c.renewals {
		d := r.Days
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			ID: uuid.NewString(), TenantID: o.TenantID, UserID: o.UserID,
			OrderID: &o.ID, OrderType: "renewal", Amount: models.FromYuan(r.Paid),
			Type: "payment", Status: "paid", Days: &d,
		}).Error)
	}
	if len(c.renewals) > 0 {
		o.CashPaid = models.FromYuan(cash)
	}
	if c.overdue > 0 || c.damage > 0 || c.addShipping > 0 {
		require.NoError(t, db.Create(&models.DamageReport{
			ID: uuid.NewString(), TenantID: o.TenantID, OrgID: o.OrgID, LeaseID: o.ID,
			InstrumentID: o.InstrumentID, UserID: o.UserID,
			OverdueFee: models.FromYuan(c.overdue), DepositDeducted: models.FromYuan(c.damage),
			AdditionalShippingFee: models.FromYuan(c.addShipping),
			Status:                "completed",
		}).Error)
	}
	return o
}

func TestRentalSettlementMatrix_2268(t *testing.T) {
	ginSetTestMode()

	cases := []matrixCase{
		{
			name:      "1_预约-支付-物流费-无损退款",
			baseDaily: 100, days: 30, deposit: 500, shipping: 20, discount: 1,
			expectRent: 3000, expectRef: 500, expectPay: 0,
		},
		{
			name:      "2_优惠码（租金9折）",
			baseDaily: 100, days: 30, deposit: 500, discount: 0.9,
			expectRent: 2700, expectRef: 500, expectPay: 0,
		},
		{
			name:      "3_免押（押金0）",
			baseDaily: 100, days: 30, deposit: 0, discount: 1,
			expectRent: 3000, expectRef: 0, expectPay: 0,
		},
		{
			name:      "4_续租一次（30+30，二档95折）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}},
			expectRent: 5850, expectRef: 500, expectPay: 0,
		},
		{
			name:      "5_逾期后先续租再按时终结（逾期费=0，检查时录入）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}},
			expectRent: 5850, expectRef: 500, expectPay: 0,
		},
		{
			name:      "6_逾期状态下终结（逾期费300）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			overdue:    300,
			expectRent: 3000, expectRef: 200, expectPay: 0,
		},
		{
			name:      "7_定损无损坏（同无损）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			expectRent: 3000, expectRef: 500, expectPay: 0,
		},
		{
			name:      "8a_定损200用户接受（押金内）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			damage:     200,
			expectRent: 3000, expectRef: 300, expectPay: 0,
		},
		{
			name:      "8b_定损700超过押金（补缴200）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			damage:     700,
			expectRent: 3000, expectRef: 0, expectPay: 200,
		},
		{
			name:      "9a_申诉后赔付额改为0",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			damage:     0,
			expectRent: 3000, expectRef: 500, expectPay: 0,
		},
		{
			name:      "9b_申诉后赔付额改为300",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			damage:     300,
			expectRent: 3000, expectRef: 200, expectPay: 0,
		},
		{
			name:      "12_多次续租（30+30+30）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}, {Days: 30, Rate: 0.95, Paid: 2850}},
			expectRent: 8700, expectRef: 500, expectPay: 0,
		},
		{
			name:      "13_续租后提前归还（用40/60天）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}},
			actualDays: 40,
			expectRent: 3950, expectRef: 2400, expectPay: 0,
		},
		{
			name:      "14_追加物流费600致押金不足（补缴100）",
			baseDaily: 100, days: 30, deposit: 500, shipping: 20, discount: 1,
			addShipping: 600,
			expectRent:  3000, expectRef: 0, expectPay: 100,
		},
		{
			name:      "16_优惠码9折首段+续租按阶梯95折",
			baseDaily: 100, days: 30, deposit: 500, discount: 0.9,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}},
			expectRent: 5550, expectRef: 500, expectPay: 0,
		},
		{
			name:      "17_续租跨档（30+150，续租段5%）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 150, Rate: 0.95, Paid: 14250}},
			expectRent: 17250, expectRef: 500, expectPay: 0,
		},
		{
			name:      "18_逾期+定损同时超押金（补缴200）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			overdue: 400, damage: 300,
			expectRent: 3000, expectRef: 0, expectPay: 200,
		},
		{
			name:      "19_逾期300+续租一次（退200）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewals:   []renewSpec{{Days: 30, Rate: 0.95, Paid: 2850}},
			overdue:    300,
			expectRent: 5850, expectRef: 200, expectPay: 0,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			db := testfixtures.SetupTestDB(t)
			order := buildMatrixOrder(t, db, c)
			res := computeSettlement(order, db)
			t.Logf("days=%d rent=%.2f totalPaid=%.2f refund=%.2f shortfall=%.2f segment=%v cashPaid=%v",
				res.ActualDays, res.RentPayable, res.TotalRentPaid, res.CashRefundable, res.PayableShortfall, res.SegmentModel, order.CashPaid)

			assert.Equal(t, c.expectRent, round2(res.RentPayable), "应收租金")
			assert.Equal(t, c.expectRef, round2(res.CashRefundable), "应退现金")
			assert.Equal(t, c.expectPay, round2(res.PayableShortfall), "应补缴")
			assert.Equal(t, c.overdue, round2(res.OverdueChargesTotal), "逾期费")

			// 守恒式：实收（含押金）− 应收（租金+逾期+定损+物流） = 退款 − 补缴
			paid := order.CashPaid.ToYuan()
			due := c.expectRent + c.overdue + c.damage + c.shipping + c.addShipping
			lhs := round2(paid - due)
			rhs := round2(res.CashRefundable - res.PayableShortfall)
			assert.Equal(t, lhs, rhs, "守恒式：实收−应收 必须等于 退款−补缴")
		})
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ginSetTestMode 避免各子测试重复 import gin（与既有测试同口径）。
func ginSetTestMode() {}

var _ = json.Marshal

// #2268 组合用例 A：租赁补缴支付回调必须入账（当前无 payment_shortfall case）。
// 期望：补缴 200 到账后 cash_paid +200、settlement 完成；当前实现无副作用 → FAIL。
func TestMatrix_RentalShortfallApplied_2268(t *testing.T) {
	// #2278 已修复：payment_shortfall 到账累加 cash_paid 并闭环 settlement。
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active", Name: "补缴用户",
	}).Error)
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user, InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: 30, Status: "returned",
		Deposit: 0, CashPaid: models.FromYuan(2800),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Settlement{
		OrderID: order.ID, ActualRentAmount: models.FromYuan(3000), RefundStatus: "pending",
	}).Error)

	rec := models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
		OrderType: "payment_shortfall", Amount: models.FromYuan(200), Type: "payment", Status: "paid",
	}
	require.NoError(t, applySideEffects(db, &rec, time.Now()))

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3000), after.CashPaid, "补缴到账后 cash_paid 必须 +200")
	var st models.Settlement
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&st).Error)
	assert.NotEqual(t, "pending", st.RefundStatus, "补缴完成后结算必须闭环（当前卡 pending）")
}

// #2268 组合用例 B：乐币不得重复扣减。
// 前端 prepay 的 amount 已扣除乐币（cashAmount = displayAmount - gift*100），
// 回调 deductPointsFromRecord 又减一次 → cash_paid 少记 gift。
func TestMatrix_GiftPointsNotDoubleDeducted_2268(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active", Name: "乐币用户",
	}).Error)
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user, InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: 30, Status: "reserved",
		Deposit: models.FromYuan(500), CashPaid: models.FromYuan(3300),
		GiftPointsUsed: models.FromYuan(200),
	}
	require.NoError(t, db.Create(&order).Error)

	gift := `{"gift_used":20000}`
	rec := models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
		OrderType: "rent", Amount: models.FromYuan(3300), Type: "payment", Status: "paid",
		RawResponse: &gift,
	}
	require.NoError(t, db.Create(&rec).Error)
	// 走生产路径（applySideEffects），不再直接调 helper
	require.NoError(t, applySideEffects(db, &rec, time.Now()))

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3300), after.CashPaid, "rent：cash 应为净额，不得再扣 gift")
	assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)

	paid, refunded, net, err := RecomputeOrderFinancials(db, order.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FromYuan(3300), paid)
	assert.Equal(t, models.Cents(0), refunded)
	assert.Equal(t, after.CashPaid, net, "Paid − Refunded = CashPaid")
}

// 返点场景 10：结算时给介绍人按「应收租金 × 推荐人等级比例」发返点；
// 被介绍人 total_spending 按现金租金基数累加（CashBasis）。
func TestMatrix_ReferralCommissionOnSettlement_2268(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testfixtures.SetupWechatPayMock(t)

	org := uuid.NewString()
	level2 := 2
	referrer := uuid.NewString()
	referee := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: referrer, IAMSub: referrer, TenantID: org, OrgID: org, Status: "active",
		Name: "推荐人", MembershipLevelID: &level2,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: referee, IAMSub: referee, TenantID: org, OrgID: org, Status: "active", Name: "被推荐人",
	}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: level2, PayRatio: 0.3, ReferralRatio: 0.05, ReferralRegPoints: 10, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.Referral{
		ReferrerID: referrer, RefereeID: referee, RefCode: "REF2268", Status: "registered",
	}).Error)

	c := matrixCase{baseDaily: 100, days: 30, deposit: 500, discount: 1}
	order := buildMatrixOrder(t, db, c)
	// 订单归属被介绍人
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).Update("user_id", referee).Error)
	order.UserID = referee
	// 支付记录归属被介绍人（结算用）
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Where("order_id = ?", order.ID).Update("user_id", referee).Error)

	_, err := executeRefund(db, order)
	require.NoError(t, err)

	// 介绍人返点 = floor(3000 × 0.05) = 150 元 = 15000 分
	var batch models.PointBatch
	require.NoError(t, db.Where("user_id = ? AND source_type = ?", referrer, services.PointBatchSourceFission).First(&batch).Error)
	assert.Equal(t, models.FromYuan(150), batch.AmountCents, "介绍人返点金额")

	var pt models.PointsTransaction
	require.NoError(t, db.Where("user_id = ? AND type = ?", referrer, "referral").First(&pt).Error)
	assert.Equal(t, models.FromYuan(150), pt.Amount)

	// 消费额单位缺陷单列（见 TestMatrix_TotalSpendingUnit_2268）
}

// 返点场景 11（风险口径）：0 现金订单（全额乐币支付租金）仍按应收租金发介绍人返点，
// 且 total_spending 因 CashBasis=0 回退到 RentPayable（等同全额计入消费）。
func TestMatrix_ReferralOnZeroCashOrder_2268(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testfixtures.SetupWechatPayMock(t)

	org := uuid.NewString()
	level2 := 2
	referrer := uuid.NewString()
	referee := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: referrer, IAMSub: referrer, TenantID: org, OrgID: org, Status: "active",
		Name: "推荐人", MembershipLevelID: &level2,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: referee, IAMSub: referee, TenantID: org, OrgID: org, Status: "active", Name: "被推荐人",
	}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{
		LevelID: level2, PayRatio: 1, ReferralRatio: 0.05, ReferralRegPoints: 10, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.Referral{
		ReferrerID: referrer, RefereeID: referee, RefCode: "REF2268B", Status: "registered",
	}).Error)

	c := matrixCase{baseDaily: 100, days: 30, deposit: 500, discount: 1}
	order := buildMatrixOrder(t, db, c)
	order.UserID = referee
	// 租金 3000 全部由乐币支付，仅现金押金 500 → cash_paid=500, gift=3000
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"user_id": referee, "cash_paid": models.FromYuan(500), "gift_points_used": models.FromYuan(3000),
	}).Error)
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Where("order_id = ?", order.ID).Updates(map[string]interface{}{
		"user_id": referee, "amount": models.FromYuan(500),
	}).Error)

	_, err := executeRefund(db, order)
	require.NoError(t, err)

	// 0 现金订单仍按应收租金发返点（当前口径，记录风险）
	var batch models.PointBatch
	require.NoError(t, db.Where("user_id = ? AND source_type = ?", referrer, services.PointBatchSourceFission).First(&batch).Error)
	assert.Equal(t, models.FromYuan(150), batch.AmountCents, "0 现金订单仍按应收租金发返点")
}

// #2280 已修复：total_spending 统一写 Cents（原缺陷：写入元，/100 少 100 倍）。

func TestMatrix_TotalSpendingUnit_2268(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testfixtures.SetupWechatPayMock(t)
	org := uuid.NewString()
	referrer, referee := uuid.NewString(), uuid.NewString()
	level2 := 2
	require.NoError(t, db.Create(&models.User{
		ID: referrer, IAMSub: referrer, TenantID: org, OrgID: org, Status: "active",
		Name: "推荐人", MembershipLevelID: &level2,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: referee, IAMSub: referee, TenantID: org, OrgID: org, Status: "active", Name: "被推荐人",
	}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{LevelID: level2, PayRatio: 0.3, ReferralRatio: 0.05, ReferralRegPoints: 10, IsActive: true}).Error)
	require.NoError(t, db.Create(&models.Referral{ReferrerID: referrer, RefereeID: referee, RefCode: "REF2268U", Status: "registered"}).Error)

	c := matrixCase{baseDaily: 100, days: 30, deposit: 500, discount: 1}
	order := buildMatrixOrder(t, db, c)
	order.UserID = referee
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).Update("user_id", referee).Error)
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Where("order_id = ?", order.ID).Update("user_id", referee).Error)

	_, err := executeRefund(db, order)
	require.NoError(t, err)

	var after models.User
	require.NoError(t, db.Where("id = ?", referee).First(&after).Error)
	assert.Equal(t, models.FromYuan(3000), after.TotalSpending,
		"total_spending 必须按分存储（当前写入元值 → 少 100 倍）")
}

// #2278：补缴超过应结金额 → 超收退款（幂等）。
func TestMatrix_ShortfallExcessRefund_2268(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user, InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: 30, Status: "returned",
		Deposit: 0, CashPaid: models.FromYuan(3500),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
		OrderType: "rent", Amount: models.FromYuan(3500), Type: "payment", Status: "paid",
	}).Error)
	require.NoError(t, db.Create(&models.Settlement{OrderID: order.ID, ActualRentAmount: models.FromYuan(3000), RefundStatus: "pending"}).Error)
	rec := models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
		OrderType: "payment_shortfall", Amount: models.FromYuan(200), Type: "payment", Status: "paid",
	}
	require.NoError(t, applySideEffects(db, &rec, time.Now()))
	// 结算重算幂等：再次重算不得重复生成退款
	require.NoError(t, applyShortfallSettlement(db, order.ID))
	require.NoError(t, applyShortfallSettlement(db, order.ID))

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3700), after.CashPaid)
	var st models.Settlement
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&st).Error)
	assert.Equal(t, models.FromYuan(700), st.CashRefundable, "超收 700 必须可退")
	var refunds []models.OrderRefundRecord
	require.NoError(t, db.Where("out_refund_no LIKE ?", "excess_%").Find(&refunds).Error)
	require.Len(t, refunds, 1, "超收退款必须幂等（仅 1 条）")
	assert.Equal(t, models.FromYuan(700), refunds[0].Amount)
}
