package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
)

// PayscoreHandler 提供微信支付分（免押租借）的信用评测入口。
//
// 设计要点（#2327 Rev.2）：
//   - 外部资质是配置项：provider 默认是 mock，前端响应里带回 mock=true，便于测试态展示；
//   - 本层只做「取订单 → 算风险金额 → 建支付分单 → 落状态」，不承载业务规则；
//   - 支付闸门在 handlers/payscore_gate.go，由支付路径调用。
type PayscoreHandler struct {
	db       *gorm.DB
	provider wechatpay.PayscoreProvider
	cfg      *wechatpay.Config
}

func NewPayscoreHandler(db *gorm.DB, provider wechatpay.PayscoreProvider, cfg *wechatpay.Config) *PayscoreHandler {
	return &PayscoreHandler{db: db, provider: provider, cfg: cfg}
}

func centsValue(p *models.Cents) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

// serviceOrderNo 用订单 UUID 去掉连字符作为支付分 out_order_no（微信上限 32 字符）。
func serviceOrderNo(order *models.Order) string {
	return strings.ReplaceAll(order.ID, "-", "")
}

func (h *PayscoreHandler) creditPayload(order *models.Order) gin.H {
	return gin.H{
		"order_id":            order.ID,
		"deposit_tier":        order.DepositTier,
		"credit_state":        order.CreditState,
		"risk_amount_cents":   order.CreditRiskAmountCents,
		"service_order_no":    order.CreditServiceOrderNo,
		"mock":                h.provider.IsMock(),
		"requires_assessment": services.RequiresCreditAssessment(order.DepositTier),
	}
}

// CreateAssessment POST /orders/:id/credit-assessment
// 幂等：已 approved 直接返回现状；已 pending 复用同一 out_order_no（微信允许重复创建吗？不允许，
// 所以 pending 时返回现状，由前端轮询 status 或重新进入评测页）。
func (h *PayscoreHandler) CreateAssessment(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未登录"})
		return
	}

	var order models.Order
	if err := h.db.WithContext(ctx).Where("id = ?", c.Param("id")).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "订单不存在"})
		return
	}
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无权操作该订单"})
		return
	}
	if !services.RequiresCreditAssessment(order.DepositTier) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "该押金档位无需信用评估"})
		return
	}
	switch order.CreditState {
	case services.CreditStateApproved, services.CreditStatePending:
		// 幂等：不重复建单（微信侧同一 out_order_no 重复创建会报错）
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": h.creditPayload(&order)})
		return
	}

	price := int64(0)
	if h.db != nil && order.InstrumentID != "" {
		var inst models.Instrument
		if err := h.db.WithContext(ctx).Where("id = ?", order.InstrumentID).First(&inst).Error; err == nil {
			price = centsValue(inst.TotalPrice)
		}
	}
	risk, err := services.ResolveCreditRiskAmount(order.DepositTier, centsValue(order.DepositFull), centsValue(order.DepositReduced), price)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	outOrderNo := serviceOrderNo(&order)
	var appID, serviceID, notifyURL string
	if h.cfg != nil {
		appID = h.cfg.AppID
		serviceID = h.cfg.PayScoreServiceID
		notifyURL = h.cfg.NotifyURL
	}
	if _, err := h.provider.CreateOrder(ctx, wechatpay.PayScoreCreateParams{
		ServiceID:  serviceID,
		OutOrderNo: outOrderNo,
		AppID:      appID,
		RiskFund:   wechatpay.PayScoreRiskFund{Name: wechatpay.RiskFundName, Amount: risk},
		NotifyURL:  notifyURL,
	}); err != nil {
		// 外部依赖失败必须透传，不得静默降级为“已通过”
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "发起信用评估失败: " + err.Error()})
		return
	}

	order.CreditState = services.CreditStatePending
	order.CreditRiskAmountCents = risk
	order.CreditServiceOrderNo = outOrderNo
	if err := h.db.WithContext(ctx).Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"credit_state":             order.CreditState,
		"credit_risk_amount_cents": order.CreditRiskAmountCents,
		"credit_service_order_no":  order.CreditServiceOrderNo,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "保存信用评估状态失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": h.creditPayload(&order)})
}

// GetCreditStatus GET /orders/:id/credit-status
func (h *PayscoreHandler) GetCreditStatus(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未登录"})
		return
	}

	var order models.Order
	if err := h.db.WithContext(ctx).Where("id = ?", c.Param("id")).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "订单不存在"})
		return
	}
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无权查看该订单"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": h.creditPayload(&order)})
}
