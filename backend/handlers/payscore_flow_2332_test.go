package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// #2327 D5/package：信用评估端点 → 状态轮询 → approved 的完整链路（mock provider）。
func TestCreditAssessmentFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	iamSub := uuid.New().String()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, TenantID: tenantID, OrgID: tenantID,
		Username: "credit-flow", Status: "active",
	}).Error)

	full := models.FromYuan(500)
	reduced := models.FromYuan(200)
	orderID := uuid.New().String()
	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &tenantID,
		SN: "SN-CREDIT-FLOW", StockStatus: "available",
		TotalPrice: ptrCents(models.FromYuan(1200)),
	}).Error)
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID:      instrumentID,
		Status:            models.OrderStatusReserved,
		DepositTier:       models.DepositTierReduced,
		DepositCreditPath: models.DepositCreditPathPayscore,
		Deposit:           reduced,
		DepositFull:       &full,
		DepositReduced:    &reduced,
	}).Error)

	mock := wechatpay.NewMockPayscoreProvider()
	handler := NewPayscoreHandler(db, mock, &wechatpay.Config{PayScoreServiceID: "SVC1"})
	actor := testutil.MakeCustomer(tenantID, iamSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/orders/:id/credit-assessment", handler.CreateAssessment)
	router.GET("/api/orders/:id/credit-status", handler.GetCreditStatus)

	// 1) 发起评估：mock 无真实 package，必须标记 mock=true。
	req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/credit-assessment", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "pending", resp.Data["credit_state"])
	assert.Equal(t, true, resp.Data["mock"])
	assert.Equal(t, "", resp.Data["credit_package"], "mock 不伪造 package")

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	assert.Equal(t, services.CreditStatePending, order.CreditState)
	require.NotEmpty(t, order.CreditServiceOrderNo)
	assert.Equal(t, int64(models.FromYuan(300)), order.CreditRiskAmountCents, "reduced 风险金额 = 全额-差额")

	// 2) 轮询状态：mock QueryOrder → approved 落库（否则 enforce 开启后永远无法支付）。
	req = httptest.NewRequest(http.MethodGet, "/api/orders/"+orderID+"/credit-status", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	assert.Equal(t, services.CreditStateApproved, order.CreditState)
	assert.NotNil(t, order.CreditAssessedAt)

	// 3) 负例：mock 拒绝 → rejected（不是 approved）。
	rejectedOrderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: rejectedOrderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID:         instrumentID,
		Status:               models.OrderStatusReserved,
		DepositTier:          models.DepositTierReduced,
		DepositCreditPath:    models.DepositCreditPathPayscore,
		CreditState:          services.CreditStatePending,
		CreditServiceOrderNo: "rejected-order",
	}).Error)
	mock.Approve = false
	require.NoError(t, func() error {
		_, err := mock.CreateOrder(t.Context(), wechatpay.PayScoreCreateParams{OutOrderNo: "rejected-order"})
		return err
	}())
	req = httptest.NewRequest(http.MethodGet, "/api/orders/"+rejectedOrderID+"/credit-status", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var rejected models.Order
	require.NoError(t, db.Where("id = ?", rejectedOrderID).First(&rejected).Error)
	assert.Equal(t, services.CreditStateRejected, rejected.CreditState)
}

func ptrCents(v models.Cents) *models.Cents { return &v }
