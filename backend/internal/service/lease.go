package service

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	CreditScoreThreshold = 650
	Discount3Months      = 1.0
	Discount6Months      = 0.98
	Discount12Months     = 0.95
)

type PricingService struct{}

type LevelPricing struct {
	Level       string             `json:"level"`
	MonthlyRent float64            `json:"monthly_rent"`
	Deposit     float64            `json:"deposit"`
	Discounts   map[string]float64 `json:"discounts"`
}

type PricingRequest struct {
	InstrumentID string `json:"instrument_id"`
	Level        string `json:"level"`
	LeaseTerm    int    `json:"lease_term"`
	DepositMode  string `json:"deposit_mode"`
	UserID       string `json:"user_id"`
	CreditScore  int    `json:"credit_score"`
}

type PricingResponse struct {
	FirstMonthRent   float64 `json:"first_month_rent"`
	Deposit          float64 `json:"deposit"`
	DepositWaived    bool    `json:"deposit_waived"`
	DepositWaivedAmt float64 `json:"deposit_waived_amt"`
	TotalAmount      float64 `json:"total_amount"`
	DiscountInfo     string  `json:"discount_info"`
	DepositInfo      string  `json:"deposit_info"`
}

func NewPricingService() *PricingService {
	return &PricingService{}
}

func (s *PricingService) CalculatePrice(ctx context.Context, req *PricingRequest) (*PricingResponse, error) {
	levelPricing := s.getLevelPricing(req.Level)

	discount := s.getDiscount(req.LeaseTerm)
	monthlyRent := levelPricing.MonthlyRent * discount

	deposit := levelPricing.Deposit
	depositWaived := false
	depositWaivedAmt := float64(0)
	depositInfo := "标准押金"

	if req.DepositMode == "free" && req.CreditScore >= CreditScoreThreshold {
		depositWaived = true
		depositWaivedAmt = deposit
		deposit = 0
		depositInfo = fmt.Sprintf("信用分%d达标，押金已免除", req.CreditScore)
	}

	resp := &PricingResponse{
		FirstMonthRent:   monthlyRent,
		Deposit:          deposit,
		DepositWaived:    depositWaived,
		DepositWaivedAmt: depositWaivedAmt,
		TotalAmount:      monthlyRent + deposit,
		DepositInfo:      depositInfo,
	}

	if req.LeaseTerm == 12 {
		resp.DiscountInfo = "12个月租期享95折"
	}

	return resp, nil
}

func (s *PricingService) getLevelPricing(level string) *LevelPricing {
	pricing := map[string]*LevelPricing{
		"entry": {
			Level:       "entry",
			MonthlyRent: 300,
			Deposit:     2000,
			Discounts: map[string]float64{
				"3":  1.0,
				"6":  0.98,
				"12": 0.95,
			},
		},
		"professional": {
			Level:       "professional",
			MonthlyRent: 800,
			Deposit:     5000,
			Discounts: map[string]float64{
				"3":  1.0,
				"6":  0.98,
				"12": 0.95,
			},
		},
		"master": {
			Level:       "master",
			MonthlyRent: 2000,
			Deposit:     10000,
			Discounts: map[string]float64{
				"3":  1.0,
				"6":  0.98,
				"12": 0.95,
			},
		},
	}

	if p, ok := pricing[level]; ok {
		return p
	}
	return pricing["entry"]
}

func (s *PricingService) getDiscount(term int) float64 {
	switch term {
	case 3:
		return Discount3Months
	case 6:
		return Discount6Months
	case 12:
		return Discount12Months
	default:
		return 1.0
	}
}

func (s *PricingService) GetInstrumentPricing(instrumentID string) map[string]interface{} {
	return map[string]interface{}{
		"entry": map[string]interface{}{
			"monthly_rent":     300,
			"deposit":          2000,
			"service_coverage": []string{"基础清洁", "免费调音 1 次/年"},
		},
		"professional": map[string]interface{}{
			"monthly_rent":     800,
			"deposit":          5000,
			"service_coverage": []string{"深度清洁", "免费调音 2 次/年", "免费维修"},
		},
		"master": map[string]interface{}{
			"monthly_rent":     2000,
			"deposit":          10000,
			"service_coverage": []string{"专家精调", "无限次调音", "免费维修", "上门保养"},
		},
	}
}

type InstrumentPricing struct {
	Entry        LevelServiceCoverage `json:"entry"`
	Professional LevelServiceCoverage `json:"professional"`
	Master       LevelServiceCoverage `json:"master"`
}

type LevelServiceCoverage struct {
	MonthlyRent     float64  `json:"monthly_rent"`
	Deposit         float64  `json:"deposit"`
	ServiceCoverage []string `json:"service_coverage"`
}

func ParsePricingJSON(pricingStr string) map[string]LevelPricing {
	var result map[string]LevelPricing
	json.Unmarshal([]byte(pricingStr), &result)
	return result
}
