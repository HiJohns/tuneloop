package services

import (
	"encoding/json"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

type PricingTierConfig struct {
	DaysMax         int     `json:"days_max"`
	DailyRate       float64 `json:"daily_rate"`
	DiscountPercent int     `json:"discount_percent"`
}

type PricingBreakdown struct {
	// Rent calculation
	BaseDailyRent         float64            `json:"base_daily_rent"`
	RentDays              int                `json:"rent_days"`
	TierDiscountRate      float64            `json:"tier_discount_rate"`
	MembershipDiscountRate float64           `json:"membership_discount_rate,omitempty"`
	PromoDiscountRates    []float64          `json:"promo_discount_rates,omitempty"`
	FinalDailyRent        float64            `json:"final_daily_rent"`
	TotalAmount           float64            `json:"total_amount"` // rent subtotal (before deposit + shipping)

	// Deposit calculation evidence
	Deposit           float64  `json:"deposit"`
	DepositMethod     string   `json:"deposit_method"`              // "total_price" or "base_daily_rate"
	DepositRatio      float64  `json:"deposit_ratio,omitempty"`     // config ratio when total_price-based
	DepositMultiplier float64  `json:"deposit_multiplier,omitempty"` // config multiplier when daily-rate-based
	TotalPrice        float64  `json:"total_price,omitempty"`       // instrument total_price (if used for deposit)
	ShippingFee       float64  `json:"shipping_fee,omitempty"`

	// Pricing strategy snapshot
	PricingTiers    []PricingTierConfig `json:"pricing_tiers,omitempty"`

	// All applied discounts
	AppliedPolicies  []AppliedPolicy `json:"applied_policies"`
}

type AppliedPolicy struct {
	Type     string  `json:"type"`
	PlanName string  `json:"plan_name"`
	Rate     float64 `json:"rate"`
}

type RentCalcInput struct {
	BaseDailyRate     float64
	LeaseTerm         int
	MembershipLevelID *int
	InstrumentID      string
	TenantID          string
	OrgID             *string
	Deposit           float64
	DepositMethod     string
	DepositRatio      float64
	DepositMultiplier float64
	TotalPrice        float64
	ShippingFee       float64
	PricingTiers      []PricingTierConfig
}

func GetTierDiscount(leaseTerm int) float64 {
	switch {
	case leaseTerm <= 30:
		return 1.0
	case leaseTerm <= 180:
		return 0.95
	case leaseTerm <= 365:
		return 0.70
	default:
		return 0.50
	}
}

func CalculatePricingBreakdown(input RentCalcInput) (*PricingBreakdown, error) {
	db := database.GetDB().WithContext(nil)
	_ = db

	result := &PricingBreakdown{
		BaseDailyRent:    input.BaseDailyRate,
		TierDiscountRate: GetTierDiscount(input.LeaseTerm),
		RentDays:         input.LeaseTerm,
		PromoDiscountRates: []float64{},
		AppliedPolicies:  []AppliedPolicy{},
	}

	finalRate := input.BaseDailyRate * result.TierDiscountRate

	result.AppliedPolicies = append(result.AppliedPolicies, AppliedPolicy{
		Type:     "tier_discount",
		PlanName: "阶梯折扣",
		Rate:     result.TierDiscountRate,
	})

	if input.MembershipLevelID != nil {
		discountRate, planName, err := getMembershipDiscount(db, *input.MembershipLevelID, input.TenantID)
		if err == nil && discountRate > 0 && discountRate < 1.0 {
			overrideEnabled, err := getInstrumentPromoOverride(db, input.InstrumentID, "discount")
			if err == nil && overrideEnabled {
				result.MembershipDiscountRate = discountRate
				finalRate *= discountRate
				if planName == "" {
					planName = "会员折扣"
				}
				result.AppliedPolicies = append(result.AppliedPolicies, AppliedPolicy{
					Type:     "membership_discount",
					PlanName: planName,
					Rate:     discountRate,
				})
			}
		}
	}

	promoRates, promoPlans, err := getPromoDiscounts(db, input.TenantID, input.OrgID)
	if err == nil {
		for i, rate := range promoRates {
			if rate > 0 && rate < 1.0 {
				finalRate *= rate
				result.PromoDiscountRates = append(result.PromoDiscountRates, rate)
				planName := promoPlans[i]
				if planName == "" {
					planName = "促销活动"
				}
				result.AppliedPolicies = append(result.AppliedPolicies, AppliedPolicy{
					Type:     "promo_campaign",
					PlanName: planName,
					Rate:     rate,
				})
			}
		}
	}

	result.FinalDailyRent = finalRate
	result.TotalAmount = finalRate * float64(input.LeaseTerm)
	result.Deposit = input.Deposit
	result.DepositMethod = input.DepositMethod
	result.DepositRatio = input.DepositRatio
	result.DepositMultiplier = input.DepositMultiplier
	result.TotalPrice = input.TotalPrice
	result.ShippingFee = input.ShippingFee
	result.PricingTiers = input.PricingTiers

	return result, nil
}

func getMembershipDiscount(db *gorm.DB, levelID int, tenantID string) (float64, string, error) {
	var plans []models.PromoPlan
	now := time.Now().Format("2006-01-02")

	if err := db.Where("plan_type = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)",
		"membership_discount", true, now, now).
		Order("scope_type ASC, created_at ASC").
		Find(&plans).Error; err != nil {
		return 1.0, "", err
	}

	var bestRate float64 = 1.0
	var bestPlanName string

	for _, plan := range plans {
		if plan.ScopeType == "merchant" && (plan.ScopeID == nil || *plan.ScopeID != tenantID) {
			continue
		}
		if plan.ScopeType == "system" {
		}

		var detail models.PromoPlanDetail
		if err := db.Where("promo_plan_id = ? AND level_id = ?", plan.ID, levelID).First(&detail).Error; err != nil {
			continue
		}

		if detail.RentDiscount > 0 && detail.RentDiscount < bestRate {
			bestRate = detail.RentDiscount
			bestPlanName = plan.Name
			if plan.ScopeType == "merchant" {
				break
			}
		}
	}

	if bestRate >= 1.0 {
		return 1.0, "", nil
	}
	return bestRate, bestPlanName, nil
}

func getInstrumentPromoOverride(db *gorm.DB, instrumentID string, overrideType string) (bool, error) {
	var override models.InstrumentPromoOverride
	if err := db.Where("instrument_id = ? AND override_type = ?", instrumentID, overrideType).First(&override).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return true, nil
		}
		return true, err
	}
	return override.Enabled, nil
}

func getPromoDiscounts(db *gorm.DB, tenantID string, orgID *string) ([]float64, []string, error) {
	now := time.Now().Format("2006-01-02")
	var plans []models.PromoPlan

	query := db.Where("plan_type = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)",
		"promo_campaign", true, now, now)

	if err := query.Order("scope_type ASC, created_at ASC").Find(&plans).Error; err != nil {
		return nil, nil, err
	}

	var rates []float64
	var planNames []string

	for _, plan := range plans {
		if plan.ScopeType == "merchant" && (plan.ScopeID == nil || *plan.ScopeID != tenantID) {
			continue
		}
		if plan.ScopeType == "site" && (plan.ScopeID == nil || orgID == nil || *plan.ScopeID != *orgID) {
			continue
		}

		rate := getPromoPlanEffectiveRate(db, plan.ID)
		if rate > 0 && rate < 1.0 {
			rates = append(rates, rate)
			planNames = append(planNames, plan.Name)
			if !plan.Stackable {
				break
			}
		}
	}

	return rates, planNames, nil
}

func getPromoPlanEffectiveRate(db *gorm.DB, planID string) float64 {
	var details []models.PromoPlanDetail
	if err := db.Where("promo_plan_id = ? AND rent_discount IS NOT NULL", planID).Find(&details).Error; err != nil {
		return 1.0
	}
	if len(details) == 0 {
		return 1.0
	}
	var minRate float64 = 1.0
	for _, d := range details {
		if d.RentDiscount > 0 && d.RentDiscount < minRate {
			minRate = d.RentDiscount
		}
	}
	return minRate
}

func FormatPricingBreakdownJSON(p *PricingBreakdown) string {
	b, _ := json.Marshal(p)
	return string(b)
}
