package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// billingTestRouter wires a merchant_admin context (tenant == org) to GetBillingReport.
func billingTestRouter(t *testing.T, tenantID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/billing/report", GetBillingReport)
	return r
}

// TestBillingReport_CentsToYuanAndFields 断言 #1999 S1：
// 金额分→元（两位小数）+ CSV/JSON 补全对账字段（order_no/out_trade_no/物流费/续费/逾期费/退款/押金已退）。
func TestBillingReport_CentsToYuanAndFields(t *testing.T) {
	db := database.GetDB()
	tenantID := uuid.New().String()
	_, instrumentID, userID := setupTestData(t, db, tenantID)

	orderID := uuid.New().String()
	paymentID := uuid.New().String()
	outTradeNo := "WX-TEST-PAY-" + uuid.New().String()[:8]
	renewTradeNo := "WX-TEST-RENEW-" + uuid.New().String()[:8]
	now := time.Now()

	order := models.Order{
		ID:                orderID,
		TenantID:          tenantID,
		OrgID:             tenantID,
		UserID:            userID,
		InstrumentID:      instrumentID,
		Level:             "standard",
		LeaseTerm:         1,
		MonthlyRent:       models.FromYuan(1000),
		Deposit:           models.FromYuan(200),
		ShippingFee:       models.FromYuan(15),
		OrderNo:           "YL20260920-001",
		Status:            "completed",
		CashPaid:          models.FromYuan(888.88),
		PrepaidPointsUsed: models.FromYuan(10),
		GiftPointsUsed:    models.FromYuan(5),
		DepositRefunded:   true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	require.NoError(t, db.Create(&order).Error)

	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: paymentID, TenantID: tenantID, UserID: userID,
		OrderID: &orderID, OrderType: "rent", Type: "payment",
		OutTradeNo: &outTradeNo, Amount: models.FromYuan(888.88),
		Status: "paid", CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, UserID: userID,
		OrderID: &orderID, OrderType: "renewal", Type: "payment",
		OutTradeNo: &renewTradeNo, Amount: models.FromYuan(30),
		Status: "paid", CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, db.Create(&models.OrderRefundRecord{
		ID: uuid.New().String(), TenantID: tenantID,
		PaymentRecordID: &paymentID, Amount: models.FromYuan(50),
		Status: "refunded", CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, db.Create(&models.DamageReport{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		LeaseID: orderID, InstrumentID: instrumentID, UserID: userID,
		OverdueFee: models.FromYuan(8),
		CreatedAt:  now, UpdatedAt: now,
	}).Error)

	router := billingTestRouter(t, tenantID)

	// --- JSON ---
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/billing/report?page=1&page_size=10", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Summary map[string]float64 `json:"summary"`
			List    []struct {
				OrderID         string  `json:"order_id"`
				OrderNo         string  `json:"order_no"`
				OutTradeNo      string  `json:"out_trade_no"`
				CashPaid        float64 `json:"cash_paid"`
				PrepaidUsed     float64 `json:"prepaid_used"`
				GiftUsed        float64 `json:"gift_used"`
				Deposit         float64 `json:"deposit"`
				ShippingFee     float64 `json:"shipping_fee"`
				RenewalAmount   float64 `json:"renewal_amount"`
				OverdueAmount   float64 `json:"overdue_amount"`
				RefundAmount    float64 `json:"refund_amount"`
				DepositRefunded bool    `json:"deposit_refunded"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	var row *struct {
		OrderID         string  `json:"order_id"`
		OrderNo         string  `json:"order_no"`
		OutTradeNo      string  `json:"out_trade_no"`
		CashPaid        float64 `json:"cash_paid"`
		PrepaidUsed     float64 `json:"prepaid_used"`
		GiftUsed        float64 `json:"gift_used"`
		Deposit         float64 `json:"deposit"`
		ShippingFee     float64 `json:"shipping_fee"`
		RenewalAmount   float64 `json:"renewal_amount"`
		OverdueAmount   float64 `json:"overdue_amount"`
		RefundAmount    float64 `json:"refund_amount"`
		DepositRefunded bool    `json:"deposit_refunded"`
	}
	for i := range resp.Data.List {
		if resp.Data.List[i].OrderID == orderID {
			row = &resp.Data.List[i]
			break
		}
	}
	require.NotNil(t, row, "响应中应包含本次创建的订单 %s", orderID)
	// 分→元：88888 分 → 888.88 元（若未转换会是 88888）
	assert.InDelta(t, 888.88, row.CashPaid, 0.001)
	assert.InDelta(t, 10.00, row.PrepaidUsed, 0.001)
	assert.InDelta(t, 5.00, row.GiftUsed, 0.001)
	assert.InDelta(t, 200.00, row.Deposit, 0.001)
	assert.InDelta(t, 15.00, row.ShippingFee, 0.001)
	assert.InDelta(t, 30.00, row.RenewalAmount, 0.001)
	assert.InDelta(t, 8.00, row.OverdueAmount, 0.001)
	assert.InDelta(t, 50.00, row.RefundAmount, 0.001)

	// 对账字段
	assert.Equal(t, orderID, row.OrderID)
	assert.Equal(t, "YL20260920-001", row.OrderNo)
	assert.Equal(t, outTradeNo, row.OutTradeNo)
	assert.True(t, row.DepositRefunded)

	// summary 同样分→元
	assert.InDelta(t, 888.88, resp.Data.Summary["total_cash_paid"], 0.001)
	assert.InDelta(t, 10.00, resp.Data.Summary["total_prepaid_used"], 0.001)
	assert.InDelta(t, 5.00, resp.Data.Summary["total_gift_used"], 0.001)
	assert.InDelta(t, 50.00, resp.Data.Summary["total_refund"], 0.001)

	// --- CSV ---
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/billing/report?format=csv", nil)
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	records, err := csv.NewReader(strings.NewReader(w2.Body.String())).ReadAll()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(records), 2, "CSV 应至少为表头 + 1 行数据")

	header := strings.Join(records[0], "|")
	for _, col := range []string{"订单号", "微信支付单号", "物流费", "续费", "逾期费", "退款", "押金已退"} {
		assert.Contains(t, header, col)
	}

	var body string
	for _, rec := range records[1:] {
		if line := strings.Join(rec, "|"); strings.Contains(line, outTradeNo) {
			body = line
			break
		}
	}
	require.NotEmpty(t, body, "CSV 应包含本次订单（按 out_trade_no 定位）")
	assert.Contains(t, body, "YL20260920-001")
	assert.Contains(t, body, outTradeNo)
	assert.Contains(t, body, "888.88", "CSV 实付应为元（888.88），非分（88888）")
	assert.Contains(t, body, "30.00", "CSV 续费应为元")
	assert.Contains(t, body, "8.00", "CSV 逾期费应为元")
	assert.Contains(t, body, "50.00", "CSV 退款应为元")
	assert.NotContains(t, body, "88888", "不应输出分")
}
