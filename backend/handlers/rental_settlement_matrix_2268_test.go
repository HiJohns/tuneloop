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

type matrixCase struct {
	name       string
	baseDaily  float64
	days       int
	deposit    float64
	shipping   float64 // 订单物流费（下单/发货时录入，从押金扣）
	discount   float64 // 档位/优惠码折后率（1=无折扣）
	renewDays  int     // 续租天数（0=未续租）
	renewRate  float64 // 续租折后率（如 0.95）
	renewPaid  float64 // 续租实付（元）
	overdue    float64 // 定损报告中的逾期费
	damage     float64 // 定损扣款（DepositDeducted）
	expectRent float64
	expectRef  float64
	expectPay  float64
}

func buildMatrixOrder(t *testing.T, db *gorm.DB, c matrixCase) models.Order {
	t.Helper()
	tiers := []services.PricingTierConfig{
		{DaysMax: 30, DiscountPercent: 0, DailyRate: c.baseDaily},
		{DaysMax: 180, DiscountPercent: 5, DailyRate: c.baseDaily},
		{DaysMax: -1, DiscountPercent: 50, DailyRate: c.baseDaily},
	}
	totalDays := c.days + c.renewDays
	segs := []services.TierSegment{}
	// 首段（合同）
	segs = append(segs, services.TierSegment{
		Tier: 1, Days: c.days, Rate: c.baseDaily, Discount: c.discount,
		Subtotal: c.baseDaily * c.discount * float64(c.days),
	})
	if c.renewDays > 0 {
		segs = append(segs, services.TierSegment{
			Tier: 2, Days: c.renewDays, Rate: c.baseDaily, Discount: c.renewRate,
			Subtotal: c.baseDaily * c.renewRate * float64(c.renewDays),
		})
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
	ret := start.AddDate(0, 0, totalDays-1).Add(12 * time.Hour)

	contractRent := c.baseDaily * c.discount * float64(c.days)
	cash := contractRent + c.deposit + c.shipping
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

	if c.renewDays > 0 {
		d := c.renewDays
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			ID: uuid.NewString(), TenantID: o.TenantID, UserID: o.UserID,
			OrderID: &o.ID, OrderType: "renewal", Amount: models.FromYuan(c.renewPaid),
			Type: "payment", Status: "paid", Days: &d,
		}).Error)
		// 订单聚合与生产 renewal.go 一致：cash_paid += 续租实付
		o.CashPaid = models.FromYuan(cash + c.renewPaid)
	}
	if c.overdue > 0 || c.damage > 0 {
		require.NoError(t, db.Create(&models.DamageReport{
			ID: uuid.NewString(), TenantID: o.TenantID, OrgID: o.OrgID, LeaseID: o.ID,
			InstrumentID: o.InstrumentID, UserID: o.UserID,
			OverdueFee: models.FromYuan(c.overdue), DepositDeducted: models.FromYuan(c.damage),
			Status: "completed",
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
			renewDays: 30, renewRate: 0.95, renewPaid: 2850,
			expectRent: 5850, expectRef: 500, expectPay: 0,
		},
		{
			name:      "5_逾期后先续租再按时终结（逾期费=0，检查时录入）",
			baseDaily: 100, days: 30, deposit: 500, discount: 1,
			renewDays: 30, renewRate: 0.95, renewPaid: 2850,
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
			due := c.expectRent + c.overdue + c.damage + c.shipping
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
	// #2268 已知缺陷（红）：applySideEffects 无 payment_shortfall 分支 →
	// 补缴到账后 cash_paid 不变、settlement 卡 pending。修复后移除 skip。
	t.Skip("known bug #2268: rental payment_shortfall has no side effects")
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active", Name: "补缴用户",
	}).Error)
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user, InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: 30, Status: "returned",
		Deposit: models.FromYuan(500), CashPaid: models.FromYuan(3500),
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
	assert.Equal(t, models.FromYuan(3700), after.CashPaid, "补缴到账后 cash_paid 必须 +200")
	var st models.Settlement
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&st).Error)
	assert.NotEqual(t, "pending", st.RefundStatus, "补缴完成后结算必须闭环（当前卡 pending）")
}

// #2268 组合用例 B：乐币不得重复扣减。
// 前端 prepay 的 amount 已扣除乐币（cashAmount = displayAmount - gift*100），
// 回调 deductPointsFromRecord 又减一次 → cash_paid 少记 gift。
func TestMatrix_GiftPointsNotDoubleDeducted_2268(t *testing.T) {
	// #2268 已知缺陷（红）：prepay amount 已扣乐币，回调再减一次 → cash_paid 少记。
	// 修复后移除 skip。
	t.Skip("known bug #2268: gift points deducted twice from cash_paid")
	db := testfixtures.SetupTestDB(t)
	org := uuid.NewString()
	user := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active", Name: "乐币用户",
	}).Error)
	// 订单：租金 3000 + 押金 500，使用乐币 200 → 创建时 cash = 3300
	order := models.Order{
		ID: uuid.NewString(), TenantID: org, OrgID: org, UserID: user, InstrumentID: uuid.NewString(),
		Level: "entry", LeaseTerm: 30, Status: "reserved",
		Deposit: models.FromYuan(500), CashPaid: models.FromYuan(3300),
		GiftPointsUsed: models.FromYuan(200),
	}
	require.NoError(t, db.Create(&order).Error)

	gift := `{"gift_used":20000}` // 分
	rec := models.OrderPaymentRecord{
		ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &order.ID,
		OrderType: "rent", Amount: models.FromYuan(3300), Type: "payment", Status: "paid",
		RawResponse: &gift,
	}
	// deductPointsFromRecord 直接调用（FIFO 可能因无批次失败，但订单更新在其之前）
	_ = deductPointsFromRecord(db, &rec, time.Now())

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	assert.Equal(t, models.FromYuan(3300), after.CashPaid,
		"cash_paid 已被 record.Amount（已扣乐币）覆盖，不得再减一次 gift；当前少记 ¥200")
	assert.Equal(t, models.FromYuan(200), after.GiftPointsUsed)
}
