package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

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
	payload := gin.H{
		"order_id":            order.ID,
		"deposit_tier":        order.DepositTier,
		"deposit_credit_path": order.DepositCreditPath,
		"credit_state":        order.CreditState,
		"risk_amount_cents":   order.CreditRiskAmountCents,
		"service_order_no":    order.CreditServiceOrderNo,
		"mock":                h.provider.IsMock(),
		"requires_assessment": services.RequiresCreditApprovalForOrder(order.DepositTier, order.DepositCreditPath),
		// #2327 D0: 前端 openBusinessView 需要的拉起凭证与过期时间。
		"credit_package": order.CreditPackage,
	}
	if order.CreditPackageExpireAt != nil {
		payload["credit_package_expire_at"] = order.CreditPackageExpireAt.Format(time.RFC3339)
	} else {
		payload["credit_package_expire_at"] = nil
	}
	return payload
}

// CreateAssessment POST /orders/:id/credit-assessment
// 幂等：已 approved 直接返回现状；已 pending 复用同一 out_order_no（微信允许重复创建吗？不允许，
// 所以 pending 时返回现状，由前端轮询 status 或重新进入评测页）。
func (h *PayscoreHandler) CreateAssessment(c *gin.Context) {
	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	localUserID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未登录"})
		return
	}

	var order models.Order
	if err := db.Where("id = ?", c.Param("id")).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "订单不存在"})
		return
	}
	if order.UserID != localUserID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无权操作该订单"})
		return
	}
	if !services.RequiresCreditApprovalForOrder(order.DepositTier, order.DepositCreditPath) {
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
	created, err := h.provider.CreateOrder(ctx, wechatpay.PayScoreCreateParams{
		ServiceID:  serviceID,
		OutOrderNo: outOrderNo,
		AppID:      appID,
		RiskFund:   wechatpay.PayScoreRiskFund{Name: wechatpay.RiskFundName, Amount: risk},
		NotifyURL:  notifyURL,
	})
	if err != nil {
		// 外部依赖失败必须透传，不得静默降级为“已通过”
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "发起信用评估失败: " + err.Error()})
		return
	}

	// #2327 D0: package 是前端拉起微信支付分业务视图的唯一凭证。
	// 真实 provider 必须带回 package；缺失则拒绝（不得返回 200 让前端空拉起）。
	order.CreditPackage = created.Package
	if created.Package != "" {
		expireAt := time.Now().Add(wechatpay.PayScorePackageTTL())
		order.CreditPackageExpireAt = &expireAt
	} else if !h.provider.IsMock() {
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": "发起信用评估失败: 微信未返回 package"})
		return
	}

	order.CreditState = services.CreditStatePending
	order.CreditRiskAmountCents = risk
	order.CreditServiceOrderNo = outOrderNo
	if err := h.db.WithContext(ctx).Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"credit_state":             order.CreditState,
		"credit_risk_amount_cents": order.CreditRiskAmountCents,
		"credit_service_order_no":  order.CreditServiceOrderNo,
		"credit_package":           order.CreditPackage,
		"credit_package_expire_at": order.CreditPackageExpireAt,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "保存信用评估状态失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": h.creditPayload(&order)})
}

// GetCreditStatus GET /orders/:id/credit-status
func (h *PayscoreHandler) GetCreditStatus(c *gin.Context) {
	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	localUserID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未登录"})
		return
	}

	var order models.Order
	if err := db.Where("id = ?", c.Param("id")).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "订单不存在"})
		return
	}
	if order.UserID != localUserID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无权查看该订单"})
		return
	}

	// #2327 D5: 前端轮询本接口时顺带向 provider 查询评估结果并落库，
	// 否则 credit_state 永远停在 pending，enforce 开启后无法支付。
	h.refreshCreditState(ctx, &order)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": h.creditPayload(&order)})
}

// refreshCreditState 在 pending 态向 provider 查询并推进状态机（幂等）。
func (h *PayscoreHandler) refreshCreditState(ctx context.Context, order *models.Order) {
	if h == nil || h.provider == nil || h.db == nil {
		return
	}
	if order.CreditState != services.CreditStatePending || order.CreditServiceOrderNo == "" {
		return
	}
	serviceID := ""
	if h.cfg != nil {
		serviceID = h.cfg.PayScoreServiceID
	}
	remote, err := h.provider.QueryOrder(ctx, serviceID, order.CreditServiceOrderNo)
	if err != nil {
		// 外部查询失败不改状态（保持 pending，等待下次轮询/回调）。
		return
	}
	event := creditEventFromProviderState(remote.State)
	if event == "" {
		return
	}
	next, err := services.NextCreditState(order.CreditState, event)
	if err != nil {
		return
	}
	if next == order.CreditState {
		return
	}
	updates := map[string]interface{}{"credit_state": next}
	if services.IsCreditSettled(next) {
		updates["credit_assessed_at"] = time.Now()
	}
	if err := h.db.Model(&models.Order{}).Where("id = ?", order.ID).Updates(updates).Error; err != nil {
		return
	}
	order.CreditState = next
}

// creditEventFromProviderState 把 provider 状态映射为状态机事件。
// mock provider 直接返回 approved/rejected；真实微信返回 CREATED/DOING/DONE/CANCELED/EXPIRED。
func creditEventFromProviderState(state string) string {
	switch strings.ToLower(state) {
	case "doing", "approved", "done":
		return services.CreditEventApprove
	case "rejected":
		return services.CreditEventReject
	case "canceled", "cancelled", "expired":
		return services.CreditEventExpire
	default:
		return ""
	}
}
