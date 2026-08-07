package models

import "time"

// Guarantor is a saved contact used as a deposit guarantor for
// deposit-free orders (#1557). Belongs to one user, reusable across orders.
type Guarantor struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;index" json:"user_id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Phone     string    `gorm:"type:varchar(50);not null" json:"phone"`
	Company   string    `gorm:"type:varchar(200)" json:"company"`
	Title     string    `gorm:"type:varchar(100)" json:"title"`
	Address   string    `gorm:"type:varchar(500)" json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

func (Guarantor) TableName() string { return "guarantors" }

// OrderGuarantor links an order to the guarantors used for its
// deposit-free application (#1557).
type OrderGuarantor struct {
	OrderID     string `gorm:"type:uuid;primaryKey" json:"order_id"`
	GuarantorID string `gorm:"type:uuid;primaryKey" json:"guarantor_id"`
}

func (OrderGuarantor) TableName() string { return "order_guarantors" }
