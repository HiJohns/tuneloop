package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1948 乐器丢失与找回：分场景结算 + 冲正 + M-08 联动（LS-03/03a/03b/05/05a/06）。

type lossFixture struct {
	tenantID, siteID, customerSub, staffSub string
	router                                  *gin.Engine
	db                                      *gorm.DB
}

func setupLossFixture(t *testing.T) lossFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID: "wx_test", NotifyURL: "http://localhost/notify", RefundNotifyURL: "http://localhost/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	tenantID := uuid.New().String()
	siteUUID := uuid.New()
	siteID := siteUUID.String()
	customerSub := uuid.New().String()
	staffSub := uuid.New().String()
	for _, u := range []string{customerSub, staffSub} {
		require.NoError(t, db.Create(&models.User{
			ID: u, IAMSub: u, TenantID: tenantID, OrgID: siteID,
			Username: "u-" + u[:8], Name: "用户", Status: "active",
		}).Error)
	}

	h := NewInstrumentLossHandler()
	r := gin.New()
	staff := testutil.TestActor{TenantID: tenantID, OrgID: siteID, UserID: staffSub, Role: "site_member"}
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(staff.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/instruments/:id/lost", h.Register)
	r.POST("/instruments/:id/restore", h.Restore)
	r.GET("/instrument-loss", h.List)
	r.POST("/api/pay/prepay", PrepayOrder)

	return lossFixture{tenantID: tenantID, siteID: siteID, customerSub: customerSub,
		staffSub: staffSub, router: r, db: db}
}

// mkLossInstrument 建乐器 + 可选租约订单（含定价 breakdown 与已付现金）。
func mkLossInstrument(t *testing.T, f lossFixture, stockStatus string, withOrder bool, orderStatus, startDate, endDate string, dailyYuan, cashPaidYuan float64) (string, string) {
	t.Helper()
	instID := uuid.New().String()
	siteUUID, _ := uuid.Parse(f.siteID)
	require.NoError(t, f.db.Create(&models.Instrument{
		ID: instID, TenantID: f.tenantID, OrgID: &f.tenantID,
		CurrentSiteID: &siteUUID, SN: "LOSS-" + instID[:8], StockStatus: stockStatus,
	}).Error)
	orderID := ""
	if withOrder {
		orderID = uuid.New().String()
		pb := fmt.Sprintf(`{"final_daily_rent":%.0f,"base_daily_rent":%.0f}`, dailyYuan*100, dailyYuan*100)
		require.NoError(t, f.db.Create(&models.Order{
			ID: orderID, TenantID: f.tenantID, OrgID: f.siteID, UserID: f.customerSub,
			InstrumentID: instID, Status: orderStatus,
			StartDate: &startDate, EndDate: &endDate,
			MonthlyRent: models.FromYuan(dailyYuan * 30), CashPaid: models.FromYuan(cashPaidYuan),
			Deposit: models.FromYuan(5000), PricingBreakdown: &pb,
		}).Error)
		// 已支付的租金单（退款原路依据）
		otn := "rent" + orderID[:8]
		require.NoError(t, f.db.Create(&models.OrderPaymentRecord{
			ID: uuid.New().String(), TenantID: f.tenantID, UserID: f.customerSub,
			OrderID: &orderID, OrderType: "rent", OutTradeNo: &otn,
			Amount: models.FromYuan(cashPaidYuan), Type: "payment", Status: "paid",
		}).Error)
	}
	return instID, orderID
}

func lossPost(t *testing.T, f lossFixture, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// LS-03a 场景②：租期中丢失（用户例：10 天租期日租 10 元押金 5000 共付 5100，
// 第 5 天丢失全责赔偿=押金全额）→ 退 50；订单 cancelled；乐器 lost；会话关闭。
func TestInstrumentLoss_InLease_Settlement(t *testing.T) {
	f := setupLossFixture(t)
	start := time.Now().AddDate(0, 0, -4).Format("2006-01-02") // ceil → 租 5 天
	end := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	instID, orderID := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusInLease, start, end, 10, 5100)

	// 会话
	require.NoError(t, f.db.Create(&models.LeaseSession{
		ID: uuid.New().String(), TenantID: f.tenantID, OrderID: orderID,
		UserID: f.customerSub, InstrumentID: instID,
		Status: models.LeaseStatusActive,
	}).Error)

	code, resp := lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "乐器丢失", "responsible_party": "user", "user_ratio": 100,
		"compensation_cents": models.FromYuan(5000),
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	d := resp["data"].(map[string]interface{})
	var breakdown map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(d["settle_breakdown"].(string)), &breakdown))
	assert.Equal(t, float64(models.FromYuan(50)), breakdown["rent_cents"], "租金=5天×10元")
	assert.Equal(t, float64(models.FromYuan(5000)), breakdown["user_burden_cents"])
	assert.Equal(t, float64(models.FromYuan(50)), breakdown["diff_cents"], "退 50")

	var inst models.Instrument
	require.NoError(t, f.db.First(&inst, "id = ?", instID).Error)
	assert.Equal(t, models.StockStatusLost, inst.StockStatus)
	var order models.Order
	require.NoError(t, f.db.First(&order, "id = ?", orderID).Error)
	assert.Equal(t, models.OrderStatusCancelled, order.Status)
	assert.True(t, order.DepositRefunded, "结算已结清 → 押金口径处置完成")
	var sess models.LeaseSession
	require.NoError(t, f.db.Where("order_id = ?", orderID).First(&sess).Error)
	assert.Equal(t, models.LeaseStatusCancelled, sess.Status)
}

// LS-03a 场景①：去程丢失（shipped，未签收）→ 租金 0，全退。
func TestInstrumentLoss_PreLease_FullRefund(t *testing.T) {
	f := setupLossFixture(t)
	instID, _ := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusShipped, "2026-09-01", "2026-09-20", 10, 5100)

	code, resp := lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "物流丢失", "responsible_party": "logistics", "user_ratio": 0,
		"compensation_cents": 0,
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	d := resp["data"].(map[string]interface{})
	var breakdown map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(d["settle_breakdown"].(string)), &breakdown))
	assert.Equal(t, "pre_lease", breakdown["scene"])
	assert.Equal(t, float64(0), breakdown["rent_cents"])
	assert.Equal(t, float64(models.FromYuan(5100)), breakdown["diff_cents"], "全额退（含押金）")

	var rr models.OrderRefundRecord
	require.NoError(t, f.db.Where("out_refund_no LIKE ?", "loss_re_%").First(&rr).Error)
	assert.Equal(t, models.Cents(models.FromYuan(5100)), rr.Amount)
}

// LS-03a 场景③：返程丢失（returning）→ 租金至归还寄出日。
func TestInstrumentLoss_Returning_Rent(t *testing.T) {
	f := setupLossFixture(t)
	// 租期 9-01~9-20，第 5 天转入 returning（寄出），第 7 天丢失 → 租金 = 7 天
	start := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	end := time.Now().AddDate(0, 0, 13).Format("2006-01-02")
	instID, retOrderID := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusReturning, start, end, 10, 5100)
	now := time.Now()
	retTs := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -2) // 今日零点−2天（elapsed 精确 5 天）
	require.NoError(t, f.db.Create(&models.OrderStatusHistory{
		ID: uuid.New().String(), TenantID: f.tenantID, OrderID: retOrderID,
		StatusFrom: models.OrderStatusInLease, StatusTo: models.OrderStatusReturning,
		ChangedAt: retTs,
	}).Error)

	code, resp := lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "返程丢失", "responsible_party": "logistics", "user_ratio": 0,
		"compensation_cents": 0,
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	d := resp["data"].(map[string]interface{})
	var breakdown map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(d["settle_breakdown"].(string)), &breakdown))
	assert.Equal(t, "returning", breakdown["scene"])
	// 租金 = 起租(7天前) → returning(2天前) = 5 天 × 10 元
	assert.Equal(t, float64(models.FromYuan(50)), breakdown["rent_cents"])
}

// LS-05/05a：租期中丢失（全责）后找回 → 冲正 = burden − 追加租金；无损坏全额退。
func TestInstrumentLoss_Restore_Reversal(t *testing.T) {
	f := setupLossFixture(t)
	start := time.Now().AddDate(0, 0, -4).Format("2006-01-02") // ceil → 5 天租
	end := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	instID, _ := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusInLease, start, end, 10, 5100)
	lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "丢失", "responsible_party": "user", "user_ratio": 100,
		"compensation_cents": models.FromYuan(5000),
	})

	// 立即找回（同日）→ 追加租金 0 → 冲正全额 5000
	code, resp := lossPost(t, f, "/instruments/"+instID+"/restore", gin.H{
		"damaged": false, "description": "找到了",
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	d := resp["data"].(map[string]interface{})
	assert.Equal(t, "refunded", d["reversal"])
	// 追加租金：CalculateLeaseDays min 1 天 × 10 元 × 100% = 1000 分 → 冲正 500000−1000=499000 分
	assert.Equal(t, float64(499000), d["refund_cents"])

	var rec models.InstrumentLossRecord
	require.NoError(t, f.db.Where("instrument_id = ?", instID).First(&rec).Error)
	require.NotNil(t, rec.ReversedAt)
	assert.Equal(t, models.Cents(499000), rec.ReversedAmountCents)

	var inst models.Instrument
	require.NoError(t, f.db.First(&inst, "id = ?", instID).Error)
	assert.Equal(t, models.StockStatusAvailable, inst.StockStatus, "乐器恢复可租")

	// 幂等：重复冲正被拒绝（无 open 记录）
	code, _ = lossPost(t, f, "/instruments/"+instID+"/restore", gin.H{"damaged": false})
	assert.Equal(t, http.StatusNotFound, code)
}

// LS-05a：找回且有损坏 → 冲正挂起（暂扣）+ pending 补缴关闭（M-08 解除）。
func TestInstrumentLoss_Restore_Damaged_Hold(t *testing.T) {
	f := setupLossFixture(t)
	start := time.Now().AddDate(0, 0, -4).Format("2006-01-02") // ceil → 5 天租
	end := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	instID, orderID := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusInLease, start, end, 10, 100)
	// 全责，赔偿=押金 → 补缴 4950（已付 100 < 应付 50+5000；start -4 天 → 租 5 天）
	lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "丢失", "responsible_party": "user", "user_ratio": 100,
		"compensation_cents": models.FromYuan(5000),
	})
	var shortfall models.OrderPaymentRecord
	require.NoError(t, f.db.Where("order_id = ? AND order_type = ? AND method = ? AND status = ?",
		orderID, "loss", "loss", "pending").First(&shortfall).Error)
	assert.Equal(t, models.Cents(models.FromYuan(4950)), shortfall.Amount, "补缴 = 应付 5050 − 已付 100")

	// 找回有损坏 → 挂起 + 补缴关闭
	code, resp := lossPost(t, f, "/instruments/"+instID+"/restore", gin.H{
		"damaged": true, "description": "找回了但有损坏",
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	assert.Equal(t, "held_pending_assessment", resp["data"].(map[string]interface{})["reversal"])

	var rec models.InstrumentLossRecord
	require.NoError(t, f.db.Where("instrument_id = ?", instID).First(&rec).Error)
	assert.Nil(t, rec.ReversedAt, "冲正挂起")
	assert.Equal(t, models.Cents(models.FromYuan(5000)), rec.DeductedDamageCents)

	var cnt int64
	require.NoError(t, f.db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND status = ?", orderID, "loss", "pending").
		Count(&cnt).Error)
	assert.Equal(t, int64(0), cnt, "pending 补缴已关闭（M-08 解除）")
}

// 纯库存丢失 → 仅记录 + 乐器状态，无结算。
func TestInstrumentLoss_NoLease_RecordsOnly(t *testing.T) {
	f := setupLossFixture(t)
	instID, orderID := mkLossInstrument(t, f, models.StockStatusAvailable, false, "", "", "", 0, 0)
	_ = orderID
	code, resp := lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "库存丢失", "responsible_party": "site", "user_ratio": 0,
		"compensation_cents": 0,
	})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	var rec models.InstrumentLossRecord
	require.NoError(t, f.db.Where("instrument_id = ?", instID).First(&rec).Error)
	assert.Nil(t, rec.SettledAt, "无租约 → 无结算")
	assert.Nil(t, rec.OrderID)
}

// LS-06：丢失补缴支付 → 回调关闭（M-08 解除）。
func TestInstrumentLoss_ShortfallPayCallback(t *testing.T) {
	f := setupLossFixture(t)
	start := time.Now().AddDate(0, 0, -4).Format("2006-01-02") // ceil → 5 天租
	end := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	instID, orderID := mkLossInstrument(t, f, models.StockStatusRented, true,
		models.OrderStatusInLease, start, end, 10, 100)
	lossPost(t, f, "/instruments/"+instID+"/lost", gin.H{
		"description": "丢失", "responsible_party": "user", "user_ratio": 100,
		"compensation_cents": models.FromYuan(5000),
	})

	// prepay order_type=loss（金额服务端取 pending 记录合计；客户端传错值）
	customer := testutil.MakeCustomer("", f.customerSub)
	body, _ := json.Marshal(map[string]interface{}{"order_type": "loss", "order_id": orderID, "amount": 0.01})
	req := httptest.NewRequest(http.MethodPost, "/api/pay/prepay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, "resp=%s", w.Body.String())
	var pre struct {
		Data struct {
			Data struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pre))
	require.NotEmpty(t, pre.Data.Data.OutTradeNo)

	var payRec models.OrderPaymentRecord
	require.NoError(t, f.db.Where("out_trade_no = ?", pre.Data.Data.OutTradeNo).First(&payRec).Error)
	assert.Equal(t, models.Cents(models.FromYuan(4950)), payRec.Amount, "收单金额=补缴合计")

	// 回调：置 paid + applySideEffects → 补缴记录关闭
	require.NoError(t, f.db.Model(&payRec).Update("status", "paid").Error)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return applySideEffects(tx, &payRec, time.Now())
	}))
	var cnt int64
	require.NoError(t, f.db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND status = ?", orderID, "loss", "pending").Count(&cnt).Error)
	assert.Equal(t, int64(0), cnt, "M-08 解除")
}
