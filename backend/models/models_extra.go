package models

import "time"

// RepairQuote stores a repair quote submitted by a technician for assessment. (Issue #1148)
type RepairQuote struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairRequestID string    `gorm:"type:uuid;index;not null" json:"repair_request_id"`
	WorkerID        string    `gorm:"type:varchar(255);not null" json:"worker_id"`
	QuoteAmount     float64   `gorm:"type:decimal(10,2);not null" json:"quote_amount"`
	Timeframe       string    `gorm:"type:varchar(100)" json:"timeframe"`
	Comment         string    `gorm:"type:text" json:"comment"`
	Status          string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}
