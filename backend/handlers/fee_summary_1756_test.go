package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1756/#1800 regression tests: GetOrder fee_detail 三段统一费用明细
// （#1800 起 fee_summary 双口径废除，改由 fee_detail 单一数据源）。
// 金额全部分（#1728 P3 契约）。

// TestGetOrder_FeeDetail_Unsettled (#1800): returning order with a paid
// record and shipping fee → paid_block = contract rent + deposit, payable_block
// = actual rent + shipping, net_block = refund (paid − payable, 0 差额).
func TestGetOrder_FeeDetail_Unsettled(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "feesum-unsettled", Name: "Fee User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEESUM", StockStatus: "rented",
	}).Error)

	start := "2026-08-01"
	end := "2026-08-04"
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    &start, EndDate: &end,
		DeliveredAt: &deliveredAt, ReturnedAt: &returnedAt,
		LeaseTerm:   3,
		Status:      models.OrderStatusReturning,
		Deposit:     models.FromYuan(1), // 1 元
		CashPaid:    models.FromYuan(4), // 租金 3 元 + 押金 1 元 = 4 元 (400 分)
		ShippingFee: models.FromYuan(1), // 1 元未付
		// total_amount = 300 分（3 天 × 1 元）
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":100,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":3,"rate":100,"discount":1,"subtotal":300}],"total_amount":300}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Paid payment record (order_type=rent, 400 分 = rent 300 + deposit 100).
	outTradeNo := "feesum-otn-1"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: 400, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
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
					} `json:"renewals"`
					Subtotal int64 `json:"subtotal"`
				} `json:"paid_block"`
				PayableBlock struct {
					ActualRent struct {
						Amount int64 `json:"amount"`
					} `json:"actual_rent"`
					ShippingFee struct {
						Amount int64 `json:"amount"`
					} `json:"shipping_fee"`
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
	require.Equal(t, 20000, resp.Code)

	fd := resp.Data.FeeDetail
	// Unsettled (returning) → settled=false.
	require.False(t, fd.Settled)

	// Paid: contract rent 300 分 + deposit 100 分 = subtotal 400.
	require.Equal(t, int64(300), fd.PaidBlock.ContractRent.Amount, "contract rent cents")
	require.Len(t, fd.PaidBlock.ContractRent.Tiers, 1)
	require.Equal(t, 1, fd.PaidBlock.ContractRent.Tiers[0].Tier)
	require.Equal(t, 3, fd.PaidBlock.ContractRent.Tiers[0].Days)
	require.Equal(t, int64(100), fd.PaidBlock.ContractRent.Tiers[0].Rate)
	require.Equal(t, int64(300), fd.PaidBlock.ContractRent.Tiers[0].Subtotal)
	require.Equal(t, int64(100), fd.PaidBlock.Deposit.Amount)
	require.Len(t, fd.PaidBlock.Renewals, 0)
	require.Equal(t, int64(400), fd.PaidBlock.Subtotal)

	// Payable: actual rent 300 + shipping 100 = 400.
	require.Equal(t, int64(300), fd.PayableBlock.ActualRent.Amount, "actual rent cents")
	require.Equal(t, int64(100), fd.PayableBlock.ShippingFee.Amount, "shipping cents")
	require.Equal(t, int64(400), fd.PayableBlock.Subtotal)

	// Unsettled 且 0 差额 → refund 方向（兼容旧 expected 行为），金额 0。
	require.Equal(t, "refund", fd.NetBlock.Direction)
	require.Equal(t, int64(0), fd.NetBlock.Amount)
}

// TestGetOrder_FeeDetail_Settled (#1800): completed order → settled=true,
// net_block.direction = none（settled 且无差额，不展示净额段）。
func TestGetOrder_FeeDetail_Settled(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "feesum-settled", Name: "Settled User", Status: "active",
	}).Error)

	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-FEESUM-S", StockStatus: "available",
	}).Error)

	start := "2026-08-01"
	end := "2026-08-04"
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    &start, EndDate: &end,
		DeliveredAt: &deliveredAt, ReturnedAt: &returnedAt,
		LeaseTerm:        3,
		Status:           models.OrderStatusCompleted,
		Deposit:          models.FromYuan(1),
		CashPaid:         models.FromYuan(4),
		ShippingFee:      models.FromYuan(1),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":100,"rent_days":3,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":3,"rate":100,"discount":1,"subtotal":300}],"total_amount":300}`),
	}
	require.NoError(t, db.Create(&order).Error)

	outTradeNo := "feesum-otn-2"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: 400, Type: "payment", Status: "paid", Method: str1743Ptr("jsapi"),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+order.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			FeeDetail struct {
				Settled   bool `json:"settled"`
				PaidBlock struct {
					Subtotal int64 `json:"subtotal"`
				} `json:"paid_block"`
				PayableBlock struct {
					Subtotal int64 `json:"subtotal"`
				} `json:"payable_block"`
				NetBlock struct {
					Direction string `json:"direction"`
				} `json:"net_block"`
			} `json:"fee_detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	fd := resp.Data.FeeDetail
	require.True(t, fd.Settled, "completed order is settled")
	require.Equal(t, "none", fd.NetBlock.Direction, "settled with no balance → none")
	require.Equal(t, int64(400), fd.PaidBlock.Subtotal)
	require.Equal(t, int64(400), fd.PayableBlock.Subtotal)
}
