package services

import (
	"encoding/json"
	"math"
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

type TierSegment struct {
	Tier     int     `json:"tier"`
	Days     int     `json:"days"`
	Rate     float64 `json:"rate"`
	Discount float64 `json:"discount"`
	Subtotal float64 `json:"subtotal"`
}

type PricingBreakdown struct {
	BaseDailyRent      float64   `json:"base_daily_rent"`
	RentDays           int       `json:"rent_days"`
	PromoDiscountRates []float64 `json:"promo_discount_rates,omitempty"`
	FinalDailyRent     float64   `json:"final_daily_rent"`
	TotalAmount        float64   `json:"total_amount"`

	Deposit           float64 `json:"deposit"`
	DepositMethod     string  `json:"deposit_method"`
	DepositRatio      float64 `json:"deposit_ratio,omitempty"`
	DepositMultiplier float64 `json:"deposit_multiplier,omitempty"`
	TotalPrice        float64 `json:"total_price,omitempty"`
	ShippingFee       float64 `json:"shipping_fee,omitempty"`

	PricingTiers []PricingTierConfig `json:"pricing_tiers,omitempty"`
	TierSegments []TierSegment       `json:"tier_segments"`

	AppliedPolicies []AppliedPolicy `json:"applied_policies"`
}

type AppliedPolicy struct {
	Type     string  `json:"type"`
	PlanName string  `json:"plan_name"`
	Rate     float64 `json:"rate"`
}

type RentCalcInput struct {
	BaseDailyRate     float64
	LeaseTerm         int
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
	DiscountCode      string // redeemable discount code (#1539)
}

var defaultTiers = []PricingTierConfig{
	{DaysMax: 30, DiscountPercent: 0},
	{DaysMax: 180, DiscountPercent: 5},
	{DaysMax: 365, DiscountPercent: 30},
	{DaysMax: -1, DiscountPercent: 50},
}

func ComputeTierSegments(totalDays int, tiers []PricingTierConfig) []TierSegment {
	if len(tiers) == 0 {
		tiers = defaultTiers
	}
	var segments []TierSegment
	prevMax := 0
	tierIndex := 1
	for _, t := range tiers {
		remaining := totalDays - prevMax
		if remaining <= 0 {
			break
		}
		var segDays int
		if t.DaysMax < 0 {
			segDays = remaining
		} else {
			segDays = t.DaysMax - prevMax
			if segDays > remaining {
				segDays = remaining
			}
		}
		if segDays <= 0 {
			prevMax = t.DaysMax
			continue
		}
		discount := 1.0 - float64(t.DiscountPercent)/100.0
		segments = append(segments, TierSegment{
			Tier:     tierIndex,
			Days:     segDays,
			Discount: discount,
		})
		tierIndex++
		prevMax += segDays
	}
	return segments
}

func CalculatePricingBreakdown(input RentCalcInput) (*PricingBreakdown, error) {
	db := database.GetDB().WithContext(nil)

	result := &PricingBreakdown{
		BaseDailyRent:      input.BaseDailyRate,
		RentDays:           input.LeaseTerm,
		PromoDiscountRates: []float64{},
		AppliedPolicies:    []AppliedPolicy{},
		PricingTiers:       input.PricingTiers,
		TierSegments:       ComputeTierSegments(input.LeaseTerm, input.PricingTiers),
	}

	promoRate := 1.0

	result.AppliedPolicies = append(result.AppliedPolicies, AppliedPolicy{
		Type:     "tier_discount",
		PlanName: "阶梯折扣",
	})

	promoRates, promoPlans, err := getPromoDiscounts(db, input.TenantID, input.OrgID)
	if err == nil {
		for i, rate := range promoRates {
			if rate > 0 && rate < 1.0 {
				promoRate *= rate
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

	cumulativeDiscount := promoRate

	// Discount code (#1539): multiply rent discount factor into the chain.
	if input.DiscountCode != "" {
		if policy := resolveDiscountPolicy(db, input.DiscountCode); policy != nil {
			cumulativeDiscount *= policy.RentDiscount
			result.AppliedPolicies = append(result.AppliedPolicies, AppliedPolicy{
				Type:     "discount_code",
				PlanName: policy.Name,
				Rate:     policy.RentDiscount,
			})
		}
	}

	totalAmount := 0.0
	weightedSum := 0.0
	for i := range result.TierSegments {
		s := &result.TierSegments[i]
		s.Rate = input.BaseDailyRate
		if i < len(input.PricingTiers) && input.PricingTiers[i].DailyRate > 0 {
			s.Rate = input.PricingTiers[i].DailyRate
		}
		s.Discount = s.Discount * cumulativeDiscount
		s.Subtotal = s.Rate * s.Discount * float64(s.Days)
		totalAmount += s.Subtotal
		weightedSum += s.Rate * s.Discount * float64(s.Days)
	}

	effectiveDailyRate := totalAmount / float64(input.LeaseTerm)
	effectiveDailyRate = math.Round(effectiveDailyRate*100) / 100

	result.FinalDailyRent = effectiveDailyRate
	result.TotalAmount = math.Round(totalAmount*100) / 100
	result.Deposit = input.Deposit
	result.DepositMethod = input.DepositMethod
	result.DepositRatio = input.DepositRatio
	result.DepositMultiplier = input.DepositMultiplier
	result.TotalPrice = input.TotalPrice
	result.ShippingFee = input.ShippingFee

	return result, nil
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

// CalculateRenewalPricing computes the cost for extending a lease by additionalDays,
// continuing from the current tier position (consumedDays offset).
func CalculateRenewalPricing(
	baseDailyRate float64,
	pricingTiers []PricingTierConfig,
	consumedDays int,
	additionalDays int,
	cumulativeDiscount float64,
) (renewalCost float64, tierBreakdown []TierSegment) {
	totalDays := consumedDays + additionalDays
	allSegments := ComputeTierSegments(totalDays, pricingTiers)

	var renewalSegments []TierSegment
	remainingToSkip := consumedDays

	for _, seg := range allSegments {
		if remainingToSkip >= seg.Days {
			remainingToSkip -= seg.Days
			continue
		}
		segInRenewal := seg.Days - remainingToSkip
		s := TierSegment{
			Tier:     seg.Tier,
			Days:     segInRenewal,
			Rate:     baseDailyRate,
			Discount: seg.Discount * cumulativeDiscount,
		}
		s.Subtotal = s.Rate * s.Discount * float64(s.Days)
		renewalSegments = append(renewalSegments, s)
		renewalCost += s.Subtotal
		remainingToSkip = 0
	}

	renewalCost = math.Round(renewalCost*100) / 100
	return renewalCost, renewalSegments
}

// resolveDiscountPolicy looks up a discount code and returns its active
// policy, or nil if the code is unknown/inactive/expired/usage-limited
// (#1539).
func resolveDiscountPolicy(db *gorm.DB, code string) *models.DiscountPolicy {
	var dc models.DiscountCode
	if err := db.Where("code = ? AND is_active = ?", code, true).First(&dc).Error; err != nil {
		return nil
	}
	if dc.MaxUses > 0 && dc.UsageCount >= dc.MaxUses {
		return nil
	}
	if dc.ExpiresAt != nil && time.Now().After(*dc.ExpiresAt) {
		return nil
	}
	var policy models.DiscountPolicy
	if err := db.Where("id = ? AND is_active = ?", dc.PolicyID, true).First(&policy).Error; err != nil {
		return nil
	}
	if policy.ValidFrom != nil && time.Now().Before(*policy.ValidFrom) {
		return nil
	}
	if policy.ValidTo != nil && time.Now().After(*policy.ValidTo) {
		return nil
	}
	return &policy
}

// ResolveDiscountCode validates a code and returns both the code record
// and its active policy (or nils). Used at order creation to record
// usage (#1539).
func ResolveDiscountCode(db *gorm.DB, code string) (*models.DiscountCode, *models.DiscountPolicy) {
	policy := resolveDiscountPolicy(db, code)
	if policy == nil {
		return nil, nil
	}
	var dc models.DiscountCode
	if err := db.Where("code = ? AND is_active = ?", code, true).First(&dc).Error; err != nil {
		return nil, nil
	}
	return &dc, policy
}
