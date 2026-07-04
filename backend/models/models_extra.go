package models

import "time"

// RepairQuoteStatus constants
const (
	RepairQuotePending    = "pending"
	RepairQuoteAccepted   = "accepted"
	RepairQuoteRejected   = "rejected"
	RepairQuoteSuperseded = "superseded"
)

// RepairQuote stores a repair quote submitted by a technician (v3).
type RepairQuote struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairRequestID string    `gorm:"type:uuid;index;not null" json:"repair_request_id"`
	SiteID          string    `gorm:"type:uuid;index" json:"site_id"`
	WorkerID        string    `gorm:"type:varchar(255);not null" json:"worker_id"`
	QuoteNo         string    `gorm:"type:varchar(30);uniqueIndex" json:"quote_no"`
	MaterialFee     float64   `gorm:"type:decimal(10,2);not null" json:"material_fee"`
	ServiceFee      float64   `gorm:"type:decimal(10,2);not null" json:"service_fee"`
	LogisticsFee    float64   `gorm:"type:decimal(10,2)" json:"logistics_fee"`
	Duration        string    `gorm:"type:varchar(100)" json:"duration"`
	Comment         string    `gorm:"type:text" json:"comment"`
	IsRenegotiation bool      `gorm:"default:false" json:"is_renegotiation"`
	Status          string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}
