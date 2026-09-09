package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// #1858 regression tests:
// ① GetNotificationDetail 对 damage_report 通知挂载 ref.damage（与订单详情
//    同源，refund/shortfall 不再恒 0——此前消息详情把退款单误导向付款页）；
// ② AgreeDamage / SubmitAppeal 幂等守卫（非 pending → 40900，不重复落账）。

// build1858Fixture creates the 84ab8ebd-style shape: pending damage report
// (¥1.00) + notification, deterministic actualRent=0 (pb without tier
// segments, no settlement, no delivered→returned window), optional paid
// records. Refund math: net = paidTotal − damage − actualRent − shipping.
func build1858Fixture(t *testing.T, paid []int64) (tenantID, userID, orderID, damageID, notifID string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)

	tenantID = uuid.New().String()
	orgID := tenantID
	userID = uuid.New().String()
	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "notify-damage-1858", Name: "Damage User", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-1858-" + time.Now().Format("150405"), StockStatus: "rented",
	}).Error)

	orderID = uuid.New().String()
	order := models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     instrumentID,
		Status:           models.OrderStatusPendingDamageResponse,
		Deposit:          0,
		ShippingFee:      0,
		PricingBreakdown: strPtr(`{"base_daily_rent":100,"rent_days":1}`),
	}
	require.NoError(t, db.Create(&order).Error)

	for i, amt := range paid {
		otn := uuid.New().String()[:20]
		_ = i
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
			OrderID: &order.ID, OrderType: "rent", OutTradeNo: &otn,
			Amount: models.Cents(amt), Type: "payment", Status: "paid", Method: strPtr("jsapi"),
			CouponDiscount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)
	}

	damageID = uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instrumentID, UserID: userID,
		DamageAmount: models.ToCentsPtr(float64Ptr(1)), // 定损 ¥1.00 = 100 分
		Status:       "pending",
	}).Error)

	notifID = uuid.New().String()
	ad := `{"damage_amount":100,"overdue_fee":0,"total_deduction":0,"deposit":0,"order_id":"` + orderID + `"}`
	require.NoError(t, db.Create(&models.Notification{
		ID: notifID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		Type: "damage", Title: "定损通知", Content: "验收发现损坏",
		RefID: damageID, RefType: "damage_report", ActionType: "damage_accept_reject",
		ActionData: &ad, Status: "unread",
	}).Error)
	return tenantID, userID, orderID, damageID, notifID
}

// newCustomerRouter wires a handler under a customer (no-tenant) JWT context.
func newCustomerRouter(userID string, register func(router *gin.Engine)) *gin.Engine {
	customer := testutil.MakeCustomer("", userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	register(router)
	return router
}

// TestGetNotificationDetail_DamageRef_RefundShape (#1858): paid ¥3.00、定损
// ¥1.00 → refund=200 分、shortfall=0 → ref.damage 挂载（消息详情不再把退款
// 单当补缴）；ref.order 保持原始行不变。
func TestGetNotificationDetail_DamageRef_RefundShape(t *testing.T) {
	_, userID, _, damageID, notifID := build1858Fixture(t, []int64{300})

	router := newCustomerRouter(userID, func(r *gin.Engine) {
		r.GET("/api/user/notifications/:id", GetNotificationDetail)
	})
	req := httptest.NewRequest("GET", "/api/user/notifications/"+notifID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Ref struct {
				Order  map[string]interface{} `json:"order"`
				Damage map[string]interface{} `json:"damage"`
			} `json:"ref"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())
	require.NotNil(t, resp.Data.Ref.Damage, "ref.damage must be attached (#1858)")

	d := resp.Data.Ref.Damage
	require.Equal(t, damageID, d["report_id"])
	require.Equal(t, float64(100), d["damage_amount"], "damage ¥1.00 = 100 分")
	require.Equal(t, float64(0), d["shortfall"], "no shortfall on refund shape")
	require.Equal(t, float64(200), d["refund"], "refund = paid 300 − damage 100 = 200 分")
	require.Equal(t, float64(300), d["paid_total"])
	require.Equal(t, "pending", d["status"])
	// raw order row stays plain — the damage object lives at ref.damage only
	require.NotContains(t, resp.Data.Ref.Order, "damage")
}

// TestGetNotificationDetail_DamageRef_ShortfallShape (#1858): 未付款 + 定损
// ¥1.00 → shortfall=100 分、refund=0。
func TestGetNotificationDetail_DamageRef_ShortfallShape(t *testing.T) {
	_, userID, _, _, notifID := build1858Fixture(t, nil)

	router := newCustomerRouter(userID, func(r *gin.Engine) {
		r.GET("/api/user/notifications/:id", GetNotificationDetail)
	})
	req := httptest.NewRequest("GET", "/api/user/notifications/"+notifID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Ref struct {
				Damage map[string]interface{} `json:"damage"`
			} `json:"ref"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.NotNil(t, resp.Data.Ref.Damage)
	require.Equal(t, float64(100), resp.Data.Ref.Damage["shortfall"])
	require.Equal(t, float64(0), resp.Data.Ref.Damage["refund"])
}

// TestAgreeDamage_IdempotentGuard (#1858): 二次 agree 返回 40900 且不重复发
// 「押金退还通知」（首次同意走 refund 分支 → order=deposit_refunding）。
func TestAgreeDamage_IdempotentGuard(t *testing.T) {
	tenantID, userID, orderID, damageID, _ := build1858Fixture(t, []int64{300})

	db := database.GetDB()
	router := newCustomerRouter(userID, func(r *gin.Engine) {
		r.POST("/api/user/appeals/:id/agree", (&AppealHandler{}).AgreeDamage)
	})
	agree := func() (code int, orderStatus string) {
		req := httptest.NewRequest("POST", "/api/user/appeals/"+damageID+"/agree", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var r struct {
			Code int `json:"code"`
			Data struct {
				OrderStatus string `json:"order_status"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		return r.Code, r.Data.OrderStatus
	}

	code1, status1 := agree()
	require.Equal(t, 20000, code1, "first agree succeeds")
	require.Equal(t, models.OrderStatusDepositRefunding, status1, "refund shape → deposit_refunding")

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	require.Equal(t, models.OrderStatusDepositRefunding, order.Status)

	var refundNotifs int64
	require.NoError(t, db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", userID, "refund").Count(&refundNotifs).Error)
	require.Equal(t, int64(1), refundNotifs, "first agree creates one refund notification")

	code2, _ := agree()
	require.Equal(t, 40900, code2, "second agree blocked by idempotent guard")
	var refundNotifs2 int64
	require.NoError(t, db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", userID, "refund").Count(&refundNotifs2).Error)
	require.Equal(t, int64(1), refundNotifs2, "no duplicate refund notification after second agree")
	_ = tenantID
}

// TestSubmitAppeal_GuardPendingOnly (#1858): 对已 agreed 的定损提交申诉 →
// 40900，不置 damage_appealing、不重复通知。
func TestSubmitAppeal_GuardPendingOnly(t *testing.T) {
	tenantID, userID, _, damageID, _ := build1858Fixture(t, []int64{300})

	db := database.GetDB()
	// 先同意 → report=agreed
	agreeRouter := newCustomerRouter(userID, func(r *gin.Engine) {
		r.POST("/api/user/appeals/:id/agree", (&AppealHandler{}).AgreeDamage)
	})
	req := httptest.NewRequest("POST", "/api/user/appeals/"+damageID+"/agree", nil)
	w := httptest.NewRecorder()
	agreeRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 对 agreed 报告提交申诉 → 40900
	appealRouter := newCustomerRouter(userID, func(r *gin.Engine) {
		r.POST("/api/user/appeals", (&AppealHandler{}).SubmitAppeal)
	})
	body, _ := json.Marshal(map[string]interface{}{
		"damage_report_id": damageID,
		"appeal_reason":    "重复申诉测试",
	})
	req2 := httptest.NewRequest("POST", "/api/user/appeals", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	appealRouter.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, 40900, resp.Code, "appeal on agreed report must be blocked: %s", w2.Body.String())

	var report models.DamageReport
	require.NoError(t, db.Where("id = ?", damageID).First(&report).Error)
	require.Equal(t, "agreed", report.Status, "status unchanged by blocked appeal")
	_ = tenantID
}
