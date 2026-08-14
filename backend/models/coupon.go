package models

// Coupon is a membership-fee discount code applied at prepay time
// (#1664). Type waive = full waiver (amount 0), percent = the value is a
// percentage of the base membership fee (ENO → 1%).
type Coupon struct {
	ID          string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string  `gorm:"uniqueIndex;size:32" json:"code"`
	Type        string  `gorm:"size:16" json:"type"` // waive / percent
	Value       float64 `json:"value"`
	Active      bool    `json:"active"`
	Description string  `json:"description"`
}