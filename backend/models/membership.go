package models

import "time"

type MembershipLevel struct {
	ID        int     `gorm:"primaryKey" json:"id"`
	Name      string  `gorm:"type:varchar(50);not null" json:"name"`
	MinAmount float64 `gorm:"type:decimal;not null" json:"min_amount"`
}

// GiftPolicy defines per-membership-level gift point rules (#1605, L-05).
// pay_ratio: gift points may cover up to floor(payable × pay_ratio) at
// payment time (initial rent + renewal). refund_ratio: on refund completion,
// rebate points = floor(cash_paid C1 × refund_ratio). level_id=0 is the
// default fallback row for unconfigured levels.
type GiftPolicy struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LevelID     int       `gorm:"not null;uniqueIndex" json:"level_id"`
	PayRatio    float64   `gorm:"type:decimal(5,4);not null;default:0.3" json:"pay_ratio"`
	RefundRatio float64   `gorm:"type:decimal(5,4);not null;default:0" json:"refund_ratio"`
	IsActive    bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (GiftPolicy) TableName() string { return "gift_policies" }

type RebateConfig struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LevelID   int       `gorm:"not null;uniqueIndex" json:"level_id"`
	RentRatio float64   `gorm:"type:decimal(5,4);not null;default:0.01" json:"rent_ratio"`
	IsActive  bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (RebateConfig) TableName() string { return "rebate_config" }

// MembershipGiftRatio defines per-membership-level gift point ratios (#1536).
// Used by: self spend loyalty (#1542), referral registration bonus (#1534),
// referral spend commission (#1535).
type MembershipGiftRatio struct {
	ID                 string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LevelID            int       `gorm:"not null;uniqueIndex" json:"level_id"`
	SelfSpendRatio     float64   `gorm:"type:decimal(5,4);not null;default:0" json:"self_spend_ratio"`
	ReferralRegPoints  float64   `gorm:"type:decimal(10,2);not null;default:0" json:"referral_reg_points"`
	ReferralSpendRatio float64   `gorm:"type:decimal(5,4);not null;default:0" json:"referral_spend_ratio"`
	IsActive           bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (MembershipGiftRatio) TableName() string { return "membership_gift_ratios" }

type PromoPlan struct {
	ID        string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PlanType  string     `gorm:"type:varchar(20);not null;default:'promo_campaign'" json:"plan_type"`
	ScopeType string     `gorm:"type:varchar(20);not null" json:"scope_type"`
	ScopeID   *string    `gorm:"type:uuid" json:"scope_id"`
	Name      string     `gorm:"type:varchar(100);not null" json:"name"`
	StartDate *string    `gorm:"type:date" json:"start_date"`
	EndDate   *string    `gorm:"type:date" json:"end_date"`
	Stackable bool       `gorm:"not null;default:false" json:"stackable"`
	IsActive  bool       `gorm:"not null;default:true" json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type PromoPlanDetail struct {
	ID               string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PromoPlanID      string  `gorm:"type:uuid;not null;index" json:"promo_plan_id"`
	LevelID          int     `gorm:"not null" json:"level_id"`
	RentDiscount     float64 `gorm:"type:decimal(5,4)" json:"rent_discount"`
	DepositDiscount  float64 `gorm:"type:decimal(5,4)" json:"deposit_discount"`
	OverdueDiscount  float64 `gorm:"type:decimal(5,4)" json:"overdue_discount"`
}

type PointsPolicy struct {
	ID          string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ScopeType   string  `gorm:"type:varchar(20);not null" json:"scope_type"`
	ScopeID     *string `gorm:"type:uuid" json:"scope_id"`
	MaxPayRatio float64 `gorm:"type:decimal(5,4)" json:"max_pay_ratio"`
	ValidDays   int     `json:"valid_days"`
	IsActive    bool    `gorm:"not null;default:true" json:"is_active"`
}

type InstrumentPromoOverride struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	InstrumentID   string    `gorm:"type:uuid;not null;index" json:"instrument_id"`
	OverrideType   string    `gorm:"type:varchar(20);not null" json:"override_type"`
	Enabled        bool      `gorm:"not null;default:true" json:"enabled"`
	UpdatedAt      time.Time `json:"updated_at"`
}
