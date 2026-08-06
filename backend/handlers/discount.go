package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateDiscountPolicy creates a discount policy (#1539).
func CreateDiscountPolicy(c *gin.Context) {
	var req struct {
		Name             string     `json:"name" binding:"required"`
		RentDiscount     float64    `json:"rent_discount"`
		DepositDiscount  float64    `json:"deposit_discount"`
		ShippingDiscount float64    `json:"shipping_discount"`
		MaxAmount        float64    `json:"max_amount"`
		ValidFrom        *time.Time `json:"valid_from"`
		ValidTo          *time.Time `json:"valid_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	policy := models.DiscountPolicy{
		ID:               uuid.New().String(),
		Name:             req.Name,
		RentDiscount:     req.RentDiscount,
		DepositDiscount:  req.DepositDiscount,
		ShippingDiscount: req.ShippingDiscount,
		MaxAmount:        req.MaxAmount,
		ValidFrom:        req.ValidFrom,
		ValidTo:          req.ValidTo,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.Create(&policy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": policy})
}

// ListDiscountPolicies lists all discount policies (#1539).
func ListDiscountPolicies(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var policies []models.DiscountPolicy
	if err := db.Order("created_at DESC").Find(&policies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": policies})
}

// CreateDiscountCode creates a discount code under a policy (#1539).
func CreateDiscountCode(c *gin.Context) {
	var req struct {
		Code      string     `json:"code" binding:"required"`
		PolicyID  string     `json:"policy_id" binding:"required"`
		MaxUses   int        `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	code := models.DiscountCode{
		ID:        uuid.New().String(),
		Code:      req.Code,
		PolicyID:  req.PolicyID,
		MaxUses:   req.MaxUses,
		ExpiresAt: req.ExpiresAt,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&code).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": code})
}

// ListDiscountCodes lists discount codes, optionally filtered by ?code=XXX (#1539).
func ListDiscountCodes(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	query := db.Model(&models.DiscountCode{})
	if code := c.Query("code"); code != "" {
		query = query.Where("code = ?", code)
	}
	var codes []models.DiscountCode
	if err := query.Order("created_at DESC").Find(&codes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": codes})
}

// ListDiscountCodeUsages lists code usage records for reporting (#1539).
// Optional filters: ?code_id=, ?order_id=, ?user_id=
func ListDiscountCodeUsages(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	query := db.Model(&models.DiscountCodeUsage{})
	if codeID := c.Query("code_id"); codeID != "" {
		query = query.Where("code_id = ?", codeID)
	}
	if orderID := c.Query("order_id"); orderID != "" {
		query = query.Where("order_id = ?", orderID)
	}
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	var usages []models.DiscountCodeUsage
	if err := query.Order("created_at DESC").Find(&usages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": usages})
}

// ApplyDiscountCode validates a code and returns its policy discount
// factors. Used at checkout to compute adjusted pricing (#1539).
// Returns 404 if the code is unknown/inactive/expired/usage-exhausted.
func ApplyDiscountCode(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "code is required"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var code models.DiscountCode
	if err := db.Where("code = ?", req.Code).First(&code).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "discount code not found"})
		return
	}
	if !code.IsActive {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount code is inactive"})
		return
	}
	if code.MaxUses > 0 && code.UsageCount >= code.MaxUses {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount code usage limit reached"})
		return
	}
	if code.ExpiresAt != nil && time.Now().After(*code.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount code has expired"})
		return
	}

	var policy models.DiscountPolicy
	if err := db.Where("id = ? AND is_active = ?", code.PolicyID, true).First(&policy).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount policy is inactive or not found"})
		return
	}
	if policy.ValidFrom != nil && time.Now().Before(*policy.ValidFrom) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount policy not yet valid"})
		return
	}
	if policy.ValidTo != nil && time.Now().After(*policy.ValidTo) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount policy has expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"code_id":           code.ID,
			"policy_id":         policy.ID,
			"policy_name":       policy.Name,
			"rent_discount":     policy.RentDiscount,
			"deposit_discount":  policy.DepositDiscount,
			"shipping_discount": policy.ShippingDiscount,
			"max_amount":        policy.MaxAmount,
		},
	})
}
