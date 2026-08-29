package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1803 T2 regression tests: fee_detail 三段统一费用明细结构
// （paid_block/payable_block/net_block），单一数据源 computeSettlement。

// pricingBreakdown1803 构建带阶梯的 pricing_breakdown（分语义）：
// base_daily_rent=100 分（1元/天），35 天首期 + 10 天续费。
func pricingBreakdown1803() string {
	return `{"base_daily_rent":100,"rent_days":35,"final_daily_rent":99,
		"pricing_tiers":[{"days_max":30,"daily_rate":100,"discount_percent":0},{"days_max":180,"daily_rate":100,"discount_percent":5}],
		"tier_segments":[{"tier":1,"days":30,"rate":100,"discount":1.0,"subtotal":3000},{"tier":2,"days":5,"rate":100,"discount":0.95,"subtotal":475}],
		"total_amount":3475}`
}

// setupFeeDetailOrder 创建带首期+续费支付记录的订单：
//   - 首期：35 天 × 1 元/天 = 35 元租金 + 押金 1 元 → CashPaid 3600 分
//   - 续费：10 天 × 1 元/天 = 10 元（renewal record amount=1000 分, days=10）
//   - CashPaid 合计 = 3600 + 1000 = 4600 分（押金 100 分不重复）
//
// totalRentPaid = (4600-100)/100 = 45 元；contractRent = 45-10 = 35 元。
func setupFeeDetailOrder(t *testing.T, status string) (string, string, string, *models.OrderPaymentRecord) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, userID := testfixtures.NewTenantIDs("c1d2e3f4a5b6")
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "fee-detail-1803", Name: "Fee Detail", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	instRate := models.Cents(100) // 1 元/天 = 100 分
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEEDETAIL", BaseDailyRate: &instRate, StockStatus: "rented",
	}).Error)

	start := "2026-08-01"
	end := "2026-09-04"
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	pb := pricingBreakdown1803()
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    &start, EndDate: &end,
		DeliveredAt: &deliveredAt, ReturnedAt: &returnedAt,
		LeaseTerm:        35,
		Status:           status,
		Deposit:          models.FromYuan(1),
		CashPaid:         models.FromYuan(46), // 45 元租金 + 1 元押金
		ShippingFee:      models.FromYuan(0),
		PricingBreakdown: &pb,
		CreatedAt:        time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&order).Error)

	// 首期支付：租金 35 元 + 押金 1 元 = 3600 分。
	outTradeNo1 := "fd-otn-1-" + uuid.New().String()[:8]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo1,
		Amount: 3600, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	// 续费支付：10 天 × 1 元 = 1000 分（days 列 = T1）。
	days := 10
	outTradeNo2 := "fd-otn-2-" + uuid.New().String()[:8]
	renewal := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", OutTradeNo: &outTradeNo2,
		Amount: 1000, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		Days: &days, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&renewal).Error)

	return tenantID, userID, order.ID, &renewal
}

// TestFeeDetail_PaidBlock_Tiers (#1803): 续费阶梯展开 — 首期 35 天
// （tier1=30天 + tier2=5天），续费 10 天落在 tier2（全局编号=2）。
func TestFeeDetail_PaidBlock_Tiers(t *testing.T) {
	tenantID, userID, orderID, _ := setupFeeDetailOrder(t, models.OrderStatusReturning)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			FeeDetail struct {
				Settled   bool `json:"settled"`
				PaidBlock struct {
					ContractRent struct {
						Amount int64  `json:"amount"`
						Date   string `json:"date"`
						Tiers  []struct {
							Tier     int   `json:"tier"`
							Rate     int64 `json:"rate"`
							Days     int   `json:"days"`
							Subtotal int64 `json:"subtotal"`
						} `json:"tiers"`
					} `json:"contract_rent"`
					Deposit struct {
						Amount int64 `json:"amount"`
					} `json:"deposit"`
					Renewals []struct {
						Amount int64 `json:"amount"`
						Days   *int  `json:"days"`
						Tiers  []struct {
							Tier     int   `json:"tier"`
							Rate     int64 `json:"rate"`
							Days     int   `json:"days"`
							Subtotal int64 `json:"subtotal"`
						} `json:"tiers"`
					} `json:"renewals"`
					Subtotal int64 `json:"subtotal"`
				} `json:"paid_block"`
				PayableBlock struct {
					Subtotal int64 `json:"subtotal"`
				} `json:"payable_block"`
				NetBlock struct {
					Direction string `json:"direction"`
					Amount    int64  `json:"amount"`
				} `json:"net_block"`
			} `json:"fee_detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())

	fd := resp.Data.FeeDetail
	// returning 态 → 未 settled。
	require.False(t, fd.Settled)

	// 首期合同租金 35 元 = 3500 分，日期 = 下单日期。
	require.Equal(t, int64(3500), fd.PaidBlock.ContractRent.Amount)
	require.Equal(t, "2026-08-01", fd.PaidBlock.ContractRent.Date)

	// 首期阶梯：tier1=30 天（rate 100 分，subtotal 3000），tier2=5 天（475）。
	require.Len(t, fd.PaidBlock.ContractRent.Tiers, 2)
	ct0 := fd.PaidBlock.ContractRent.Tiers[0]
	require.Equal(t, 1, ct0.Tier)
	require.Equal(t, 30, ct0.Days)
	require.Equal(t, int64(100), ct0.Rate)
	require.Equal(t, int64(3000), ct0.Subtotal)
	ct1 := fd.PaidBlock.ContractRent.Tiers[1]
	require.Equal(t, 2, ct1.Tier)
	require.Equal(t, 5, ct1.Days)
	require.Equal(t, int64(475), ct1.Subtotal)

	// 押金 1 元 = 100 分。
	require.Equal(t, int64(100), fd.PaidBlock.Deposit.Amount)

	// 续费块：金额 1000 分，days=10，阶梯 tier 全局编号=2（延续首期）。
	require.Len(t, fd.PaidBlock.Renewals, 1)
	rn := fd.PaidBlock.Renewals[0]
	require.Equal(t, int64(1000), rn.Amount)
	require.NotNil(t, rn.Days)
	require.Equal(t, 10, *rn.Days)
	require.Len(t, rn.Tiers, 1)
	require.Equal(t, 2, rn.Tiers[0].Tier, "renewal tier continues global numbering (tier 2)")
	require.Equal(t, 10, rn.Tiers[0].Days)
	require.Equal(t, int64(950), rn.Tiers[0].Subtotal, "10 days × 100 cents × 0.95 discount")

	// 合计实付 = 3500 + 100 + 1000 = 4600 分。
	require.Equal(t, int64(4600), fd.PaidBlock.Subtotal)
}

// TestFeeDetail_LegacyRenewal_NoTiers (#1803): 历史续费记录（days 为 NULL）
// → 该次续费块 tiers 为空数组（展示降级：金额+日期，无阶梯）。
func TestFeeDetail_LegacyRenewal_NoTiers(t *testing.T) {
	tenantID, userID, orderID, renewal := setupFeeDetailOrder(t, models.OrderStatusReturning)

	db := database.GetDB()
	// 清掉 days（模拟 T1 之前的存量记录）。
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).
		Where("id = ?", renewal.ID).Update("days", nil).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			FeeDetail struct {
				PaidBlock struct {
					Renewals []struct {
						Amount int64 `json:"amount"`
						Days   *int  `json:"days"`
						Tiers  []any `json:"tiers"`
					} `json:"renewals"`
				} `json:"paid_block"`
			} `json:"fee_detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())

	require.Len(t, resp.Data.FeeDetail.PaidBlock.Renewals, 1)
	rn := resp.Data.FeeDetail.PaidBlock.Renewals[0]
	require.Equal(t, int64(1000), rn.Amount)
	require.Nil(t, rn.Days, "legacy renewal has no days")
	require.Len(t, rn.Tiers, 0, "legacy renewal shows amount+date only, no tiers")
}

// TestFeeDetail_SettledReconstruction (#1803): 两态一致性 — returning 态
// 实时计算与 settled 态从落库 Breakdown 读回的金额完全一致。
func TestFeeDetail_SettledReconstruction(t *testing.T) {
	tenantID, userID, orderID, _ := setupFeeDetailOrder(t, models.OrderStatusReturning)
	db := database.GetDB()

	// returning 态 fee_detail。
	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)
	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var returningResp struct {
		Data struct {
			FeeDetail struct {
				PaidBlock    map[string]interface{} `json:"paid_block"`
				PayableBlock map[string]interface{} `json:"payable_block"`
				NetBlock     map[string]interface{} `json:"net_block"`
			} `json:"fee_detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &returningResp))
	returningPaid := returningResp.Data.FeeDetail.PaidBlock

	// 模拟 ConfirmSettlement/executeRefund：computeSettlement 落库。
	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	result := computeSettlement(order, db)
	breakdownJSON, _ := json.Marshal(result.Breakdown)
	require.NoError(t, db.Create(&models.Settlement{
		ID:                  uuid.New().String(),
		OrderID:             order.ID,
		ActualRentDays:      result.ActualDays,
		ActualRentAmount:    models.FromYuan(result.RentPayable),
		OriginalRentAmount:  models.FromYuan(result.TotalRentPaid),
		GiftPointsRefunded:  models.FromYuan(result.GiftPointsRefunded),
		CashRefundable:      models.FromYuan(result.CashRefundable),
		RefundMethod:        "wechat_pay",
		RefundStatus:        "pending",
		OverdueChargesTotal: models.FromYuan(result.OverdueChargesTotal),
		Breakdown:           string(breakdownJSON),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}).Error)

	// 订单转为 completed（settled 态）。
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", orderID).
		Update("status", models.OrderStatusCompleted).Error)

	// settled 态 fee_detail（从落库 Breakdown 读回）。
	req2 := httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var settledResp struct {
		Data struct {
			FeeDetail struct {
				Settled      bool                   `json:"settled"`
				PaidBlock    map[string]interface{} `json:"paid_block"`
				PayableBlock map[string]interface{} `json:"payable_block"`
				NetBlock     map[string]interface{} `json:"net_block"`
			} `json:"fee_detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &settledResp))
	require.True(t, settledResp.Data.FeeDetail.Settled, "completed order is settled")

	// 两态 paid_block 的 subtotal 必须一致（单一数据源）。
	returningSub, _ := returningPaid["subtotal"].(float64)
	settledSub, _ := settledResp.Data.FeeDetail.PaidBlock["subtotal"].(float64)
	require.Equal(t, returningSub, settledSub, "paid subtotal identical across states")
}
