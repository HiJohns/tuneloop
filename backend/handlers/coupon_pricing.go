package handlers

// #2291：优惠码定价纯函数 + 校验接口。
// 前端不再硬编码码值与折扣幅度，统一由服务端按 coupons 表计算。

import (
	"errors"
	"net/http"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

var (
	ErrCouponConfig = errors.New("coupon config invalid")
	ErrCouponType   = errors.New("coupon type invalid")
	ErrCouponAmount = errors.New("coupon amount invalid")
)

// CouponPricing 金额单位均为分（models.Cents）。
type CouponPricing struct {
	FinalCents    models.Cents
	DiscountCents models.Cents
}

// resolveCouponPricing：waive → 0；percent → base*value/1000（整数分）；
// value 必须在 1..1000‰，未知 type / 负金额 → error。
func resolveCouponPricing(c models.Coupon, baseCents models.Cents) (CouponPricing, error) {
	if baseCents < 0 {
		return CouponPricing{}, ErrCouponAmount
	}
	switch c.Type {
	case "waive":
		return CouponPricing{FinalCents: 0, DiscountCents: baseCents}, nil
	case "percent":
		if c.Value < 1 || c.Value > 1000 {
			return CouponPricing{}, ErrCouponConfig
		}
		final := baseCents * models.Cents(c.Value) / 1000
		if final < 0 {
			final = 0
		}
		if final > baseCents {
			final = baseCents
		}
		return CouponPricing{FinalCents: final, DiscountCents: baseCents - final}, nil
	default:
		return CouponPricing{}, ErrCouponType
	}
}

type validateCouponRequest struct {
	Code        string `json:"code" binding:"required"`
	AmountCents int64  `json:"amount_cents"`
}

// ValidateCoupon：POST /api/coupons/validate（仅预览，最终扣款仍由 prepay 重算）。
func ValidateCoupon(c *gin.Context) {
	var req validateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}
	if req.AmountCents < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "amount_cents must be >= 0"})
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	var coupon models.Coupon
	if err := database.GetDB().WithContext(c.Request.Context()).
		Where("code = ? AND active = ?", code, true).First(&coupon).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "优惠码无效"})
		return
	}
	pricing, err := resolveCouponPricing(coupon, models.Cents(req.AmountCents))
	if err != nil {
		if errors.Is(err, ErrCouponConfig) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "优惠码配置异常，请联系客服"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "优惠码无效"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"code":               coupon.Code,
			"type":               coupon.Type,
			"value":              coupon.Value,
			"description":        coupon.Description,
			"discount_cents":     int64(pricing.DiscountCents),
			"final_amount_cents": int64(pricing.FinalCents),
		},
	})
}
