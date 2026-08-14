package models

import "time"

// RegistrationSession holds a two-phase registration attempt (#1663/#1664):
// the user submits the form first (pending), pays the membership fee
// (paid), then the server creates the account from form_data (completed).
type RegistrationSession struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OpenID        string     `gorm:"column:openid;type:varchar(128);index" json:"openid"`
	ExchangeToken string     `gorm:"type:varchar(128)" json:"-"`
	FormData      string     `gorm:"type:jsonb" json:"form_data"`
	CouponCode    string     `gorm:"type:varchar(32)" json:"coupon_code"`
	Amount        float64    `gorm:"type:numeric(10,2)" json:"amount"`
	Status        string     `gorm:"size:16;default:'pending'" json:"status"` // pending/paid/completed/failed
	Error         string     `gorm:"type:text" json:"error"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}
