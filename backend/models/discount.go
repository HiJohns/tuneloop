package models

import "time"

// DiscountPolicy defines the discount terms referenced by discount codes
// (#1539). One policy can back many codes.
type DiscountPolicy struct {
	ID               string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name             string     `gorm:"type:varchar(100);not null" json:"name"`
	RentDiscount     float64    `gorm:"type:decimal(5,4);default:1" json:"rent_discount"`     // 0.9 = 9折
	DepositDiscount  float64    `gorm:"type:decimal(5,4);default:1" json:"deposit_discount"`  // 0.9 = 9折
	ShippingDiscount float64    `gorm:"type:decimal(5,4);default:1" json:"shipping_discount"` // 0.9 = 9折
	MaxAmount        Cents      `gorm:"type:bigint;default:0" json:"max_amount"`              // 0 = no cap
	ValidFrom        *time.Time `json:"valid_from"`
	ValidTo          *time.Time `json:"valid_to"`
	IsActive         bool       `gorm:"not null;default:true" json:"is_active"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (DiscountPolicy) TableName() string { return "discount_policies" }

// DiscountCode is a redeemable code linked to a discount policy (#1539).
type DiscountCode struct {
	ID         string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code       string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	PolicyID   string     `gorm:"type:uuid;index;not null" json:"policy_id"`
	MaxUses    int        `gorm:"default:0" json:"max_uses"` // 0 = unlimited
	UsageCount int        `gorm:"default:0" json:"usage_count"`
	ExpiresAt  *time.Time `json:"expires_at"`
	IsActive   bool       `gorm:"not null;default:true" json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (DiscountCode) TableName() string { return "discount_codes" }

// DiscountCodeUsage records each redemption of a discount code (#1539).
type DiscountCodeUsage struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CodeID         string    `gorm:"type:uuid;index;not null" json:"code_id"`
	OrderID        string    `gorm:"type:uuid;index" json:"order_id"`
	UserID         string    `gorm:"type:uuid;index" json:"user_id"`
	DiscountAmount float64   `gorm:"type:decimal(10,2);not null;default:0" json:"discount_amount"`
	CreatedAt      time.Time `json:"created_at"`
}

func (DiscountCodeUsage) TableName() string { return "discount_code_usages" }
