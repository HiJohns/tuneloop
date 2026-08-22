package models

// Coupon is a discount code applied at prepay time (#1719 通用化)。
// percent 类型的 Value 为千分比（1000 = 1%，#1728/#1751）——ENO = 10（10‰ = 1%），
// 配置成 1 会按 1‰ 少扣 10 倍。Type waive = full waiver (amount 0).
type Coupon struct {
	ID          string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string  `gorm:"uniqueIndex;size:32" json:"code"`
	Type        string  `gorm:"size:16" json:"type"` // waive / percent
	Value       float64 `json:"value"`
	Active      bool    `json:"active"`
	Description string  `json:"description"`
}
