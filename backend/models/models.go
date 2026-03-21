package models

import (
	"time"
)

type User struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IAMSub      string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"iam_sub"`
	TenantID    string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID       string    `gorm:"type:uuid;index;not null" json:"org_id"`
	Name        string    `gorm:"type:varchar(255)" json:"name"`
	Phone       string    `gorm:"type:varchar(50)" json:"phone"`
	Email       string    `gorm:"type:varchar(255)" json:"email"`
	CreditScore int       `gorm:"default:600" json:"credit_score"`
	DepositMode string    `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	IsShadow    bool      `gorm:"default:true" json:"is_shadow"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Category struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Icon      string    `json:"icon"`
	ParentID  *string   `gorm:"type:uuid" json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Instrument struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	Brand          string    `gorm:"type:varchar(100)" json:"brand"`
	Level          string    `gorm:"type:varchar(20);not null" json:"level"`
	LevelName      string    `gorm:"type:varchar(50)" json:"level_name"`
	Description    string    `gorm:"type:text" json:"description"`
	Images         string    `gorm:"type:jsonb;default:'[]'" json:"images"`
	Video          string    `gorm:"type:varchar(500)" json:"video"`
	Specifications string    `gorm:"type:jsonb;default:'{}'" json:"specifications"`
	Pricing        string    `gorm:"type:jsonb;default:'{}'" json:"pricing"`
	StockStatus    string    `gorm:"type:varchar(20);default:'available'" json:"stock_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Order struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID            string    `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID      string    `gorm:"type:uuid;not null" json:"instrument_id"`
	Level             string    `gorm:"type:varchar(20);not null" json:"level"`
	LeaseTerm         int       `gorm:"not null" json:"lease_term"`
	DepositMode       string    `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	MonthlyRent       float64   `gorm:"type:decimal(10,2);not null" json:"monthly_rent"`
	Deposit           float64   `gorm:"type:decimal(10,2);default:0" json:"deposit"`
	AccumulatedMonths int       `gorm:"default:0" json:"accumulated_months"`
	Status            string    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	StartDate         string    `gorm:"type:date" json:"start_date"`
	EndDate           string    `gorm:"type:date" json:"end_date"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Site struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name          string    `gorm:"type:varchar(255);not null" json:"name"`
	Address       string    `gorm:"type:varchar(500)" json:"address"`
	Latitude      float64   `gorm:"type:decimal(10,6)" json:"latitude"`
	Longitude     float64   `gorm:"type:decimal(10,6)" json:"longitude"`
	Phone         string    `gorm:"type:varchar(50)" json:"phone"`
	BusinessHours string    `gorm:"type:varchar(100)" json:"business_hours"`
	Status        string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type MaintenanceTicket struct {
	ID                 string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID            string    `gorm:"type:uuid;not null" json:"order_id"`
	InstrumentID       string    `gorm:"type:uuid;not null" json:"instrument_id"`
	UserID             string    `gorm:"type:uuid;not null;index" json:"user_id"`
	ProblemDescription string    `gorm:"type:text" json:"problem_description"`
	Images             string    `gorm:"type:jsonb;default:'[]'" json:"images"`
	ServiceType        string    `gorm:"type:varchar(20)" json:"service_type"`
	Status             string    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	AssignedSiteID     string    `gorm:"type:uuid" json:"assigned_site_id"`
	EstimatedCost      float64   `gorm:"type:decimal(10,2);default:0" json:"estimated_cost"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type BrandConfig struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ClientID     string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"client_id"`
	PrimaryColor string    `gorm:"type:varchar(20);default:'#6366F1'" json:"primary_color"`
	LogoURL      string    `gorm:"type:varchar(500)" json:"logo_url"`
	BrandName    string    `gorm:"type:varchar(100)" json:"brand_name"`
	SupportPhone string    `gorm:"type:varchar(50)" json:"support_phone"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
