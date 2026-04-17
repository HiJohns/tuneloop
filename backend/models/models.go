package models

import (
	"github.com/google/uuid"
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
	TenantID  string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Icon      string    `json:"icon"`
	ParentID  *string   `gorm:"type:uuid" json:"parent_id"`
	Level     int       `gorm:"default:1" json:"level"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Visible   bool      `gorm:"default:true" json:"visible"`
	CreatedAt time.Time `json:"created_at"`
}

type Instrument struct {
	ID              string           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID        string           `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID           string           `gorm:"type:uuid;index" json:"org_id"`
	CategoryID      string           `gorm:"type:uuid;index" json:"category_id"`
	CategoryName    string           `gorm:"type:varchar(100)" json:"category_name"`
	Name            string           `gorm:"type:varchar(255)" json:"name"`
	Brand           string           `gorm:"type:varchar(100)" json:"brand"`
	Level           string           `gorm:"type:varchar(20)" json:"level"`      // deprecated, use LevelID instead
	LevelName       string           `gorm:"type:varchar(50)" json:"level_name"` // deprecated
	LevelID         *uuid.UUID       `gorm:"type:uuid;index" json:"level_id"`
	InstrumentLevel *InstrumentLevel `gorm:"foreignKey:LevelID" json:"instrument_level,omitempty"`
	Model           string           `gorm:"type:varchar(100)" json:"model"`
	SN              string           `gorm:"type:varchar(100)" json:"sn"`
	Site            string           `gorm:"type:varchar(255)" json:"site"`
	SiteID          *uuid.UUID       `gorm:"type:uuid;index" json:"site_id"`
	CurrentSiteID   *uuid.UUID       `gorm:"type:uuid;index" json:"current_site_id"`
	Description     string           `gorm:"type:text" json:"description"`
	Images          string           `gorm:"type:jsonb;default:'[]'" json:"images"`
	Video           string           `gorm:"type:varchar(500)" json:"video"`
	Specifications  string           `gorm:"type:jsonb;default:'{}'" json:"specifications"`
	Pricing         string           `gorm:"type:jsonb;default:'{}'" json:"pricing"`
	StockStatus     string           `gorm:"type:varchar(20);default:'available'" json:"stock_status"`
	Properties      string           `gorm:"type:jsonb;default:'{}'" json:"properties"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type Order struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID             string    `gorm:"type:uuid;index" json:"org_id"`
	UserID            string    `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID      string    `gorm:"type:uuid;not null" json:"instrument_id"`
	Level             string    `gorm:"type:varchar(20);not null" json:"level"`
	LeaseTerm         int       `gorm:"not null" json:"lease_term"`
	DepositMode       string    `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	MonthlyRent       float64   `gorm:"type:decimal(10,2);not null" json:"monthly_rent"`
	Deposit           float64   `gorm:"type:decimal(10,2);default:0" json:"deposit"`
	AccumulatedMonths int       `gorm:"default:0" json:"accumulated_months"`
	Status            string    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	StartDate         *string   `gorm:"type:date" json:"start_date"`
	EndDate           *string   `gorm:"type:date" json:"end_date"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Site struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID      string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID         string     `gorm:"type:uuid;index" json:"org_id"`
	ParentID      *uuid.UUID `gorm:"type:uuid;index" json:"parent_id"`
	ManagerID     *uuid.UUID `gorm:"column:manager_id;type:uuid;index" json:"manager_id"`
	Name          string     `gorm:"type:varchar(255);not null" json:"name"`
	Address       string     `gorm:"type:varchar(500)" json:"address"`
	Type          string     `gorm:"type:varchar(50)" json:"type"`
	Latitude      float64    `gorm:"type:decimal(10,6)" json:"latitude"`
	Longitude     float64    `gorm:"type:decimal(10,6)" json:"longitude"`
	Phone         string     `gorm:"type:varchar(50)" json:"phone"`
	BusinessHours string     `gorm:"type:varchar(100)" json:"business_hours"`
	Status        string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

const (
	TicketStatusPending    = "PENDING"
	TicketStatusProcessing = "PROCESSING"
	TicketStatusCompleted  = "COMPLETED"
)

type MaintenanceTicket struct {
	ID                 string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID           string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID              string     `gorm:"type:uuid;index" json:"org_id"`
	OrderID            string     `gorm:"type:uuid;not null" json:"order_id"`
	InstrumentID       string     `gorm:"type:uuid;not null" json:"instrument_id"`
	UserID             string     `gorm:"type:uuid;not null;index" json:"user_id"`
	ProblemDescription string     `gorm:"type:text" json:"problem_description"`
	Images             string     `gorm:"type:jsonb;default:'[]'" json:"images"`
	ServiceType        string     `gorm:"type:varchar(20)" json:"service_type"`
	Status             string     `gorm:"type:varchar(20);default:'PENDING';index" json:"status"`
	AssignedSiteID     string     `gorm:"type:uuid" json:"assigned_site_id"`
	TechnicianID       string     `gorm:"type:uuid;index" json:"technician_id"`
	ProgressNotes      string     `gorm:"type:text" json:"progress_notes"`
	RepairReport       string     `gorm:"type:text" json:"repair_report"`
	RepairPhotos       string     `gorm:"type:jsonb;default:'[]'" json:"repair_photos"`
	EstimatedCost      float64    `gorm:"type:decimal(10,2);default:0" json:"estimated_cost"`
	AcceptedAt         *time.Time `json:"accepted_at"`
	CompletionNotes    string     `gorm:"type:text" json:"completion_notes"`
	CompletionPhotos   string     `gorm:"type:jsonb;default:'[]'" json:"completion_photos"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `gorm:"index" json:"completed_at,omitempty"`
}

type BrandConfig struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TenantID     string    `gorm:"type:uuid;index" json:"tenant_id"`
	ClientID     string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"client_id"`
	PrimaryColor string    `gorm:"type:varchar(20);default:'#6366F1'" json:"primary_color"`
	LogoURL      string    `gorm:"type:varchar(500)" json:"logo_url"`
	BrandName    string    `gorm:"type:varchar(100)" json:"brand_name"`
	SupportPhone string    `gorm:"type:varchar(50)" json:"support_phone"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OwnershipCertificate struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID          string    `gorm:"type:uuid;index" json:"org_id"`
	OrderID        string    `gorm:"type:uuid;uniqueIndex;not null" json:"order_id"`
	UserID         string    `gorm:"type:uuid;index" json:"user_id"`
	InstrumentID   string    `gorm:"type:uuid;index" json:"instrument_id"`
	TransferDate   time.Time `json:"transfer_date"`
	CertificateURL string    `gorm:"type:varchar(500)" json:"certificate_url"`
	CreatedAt      time.Time `json:"created_at"`
}

type Technician struct {
	ID       string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID string `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID    string `gorm:"type:uuid;index" json:"org_id"`
	SiteID   string `gorm:"type:uuid;index" json:"site_id"`
	Name     string `gorm:"type:varchar(100)" json:"name"`
	Phone    string `gorm:"type:varchar(50)" json:"phone"`
}

type SiteImage struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	SiteID    string    `gorm:"type:uuid;not null" json:"site_id"`
	URL       string    `gorm:"type:varchar(500);not null" json:"url"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type InventoryTransfer struct {
	ID          string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID    string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID       string     `gorm:"type:uuid;index" json:"org_id"`
	AssetID     string     `gorm:"type:uuid;index;not null" json:"asset_id"`
	FromSiteID  string     `gorm:"type:uuid;not null" json:"from_site_id"`
	ToSiteID    string     `gorm:"type:uuid;not null" json:"to_site_id"`
	Reason      string     `gorm:"type:text" json:"reason"`
	Status      string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedBy   string     `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Client struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	ClientID     string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"client_id"`
	ClientSecret string    `gorm:"type:varchar(255)" json:"client_secret"`
	Name         string    `gorm:"type:varchar(100)" json:"name"`
	RedirectURIs string    `gorm:"type:text" json:"redirect_uris"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Lease struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID      string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	UserID        string    `gorm:"type:uuid;index;not null" json:"user_id"`
	InstrumentID  string    `gorm:"type:uuid;index;not null" json:"instrument_id"`
	StartDate     string    `gorm:"type:date;not null" json:"start_date"`
	EndDate       string    `gorm:"type:date;not null" json:"end_date"`
	MonthlyRent   float64   `gorm:"type:decimal(10,2);not null" json:"monthly_rent"`
	DepositAmount float64   `gorm:"type:decimal(10,2);not null" json:"deposit_amount"`
	Status        string    `gorm:"type:varchar(20);default:'active';index" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Deposit struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID        string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	LeaseID         string    `gorm:"type:uuid;index;not null" json:"lease_id"`
	UserID          string    `gorm:"type:uuid;index;not null" json:"user_id"`
	Amount          float64   `gorm:"type:decimal(10,2);not null" json:"amount"`
	Type            string    `gorm:"type:varchar(20);not null" json:"type"`
	Status          string    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	TransactionDate string    `gorm:"type:date;not null" json:"transaction_date"`
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Label represents a normalized tag/label for instruments
type Label struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Name           string    `gorm:"type:varchar(100);not null;index" json:"name"`
	Alias          string    `gorm:"type:jsonb;default:'[]'" json:"alias"`
	AuditStatus    string    `gorm:"type:varchar(20);default:'pending'" json:"audit_status"`
	NormalizedToID *string   `gorm:"type:uuid;index" json:"normalized_to_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Tenant struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// InstrumentLevel represents the skill level for instruments
type InstrumentLevel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Caption   string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"caption"`
	Code      string    `gorm:"type:varchar(20);uniqueIndex;not null" json:"code"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type Property struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Name         string    `gorm:"type:varchar(100);not null" json:"name"`
	PropertyType string    `gorm:"type:varchar(20);not null" json:"property_type"`
	IsRequired   bool      `gorm:"default:false" json:"is_required"`
	Unit         string    `gorm:"type:varchar(50)" json:"unit"`
	Caption      string    `gorm:"type:varchar(100);not null" json:"caption"`
	Status       string    `gorm:"type:varchar(20);default:'active';not null" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PropertyOption struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	PropertyName string    `gorm:"type:varchar(100);index" json:"property_name"`
	Value        string    `gorm:"type:varchar(255);not null" json:"value"`
	Status       string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Alias        *string   `gorm:"type:uuid;index" json:"alias"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type InstrumentProperty struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	InstrumentID string    `gorm:"type:uuid;index;not null" json:"instrument_id"`
	PropertyName string    `gorm:"type:varchar(100);index" json:"property_name"`
	Value        string    `gorm:"type:varchar(255)" json:"value"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// MaintenanceWorker 维修师傅表
type MaintenanceWorker struct {
	ID        string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID     string     `gorm:"type:uuid;index" json:"org_id"`
	SiteID    *string    `gorm:"type:uuid;index" json:"site_id"`
	Name      string     `gorm:"type:varchar(100);not null" json:"name"`
	Phone     string     `gorm:"type:varchar(50)" json:"phone"`
	JoinDate  *time.Time `json:"join_date"`
	Status    string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

// MaintenanceSession 维修会话表
type MaintenanceSession struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID            string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID               string     `gorm:"type:uuid;index" json:"org_id"`
	MaintenanceTicketID string     `gorm:"type:uuid;not null" json:"maintenance_ticket_id"`
	WorkerID            *string    `gorm:"type:uuid;index" json:"worker_id"`
	Status              string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	StartTime           *time.Time `json:"start_time"`
	EndTime             *time.Time `json:"end_time"`
	ProgressNotes       string     `gorm:"type:text" json:"progress_notes"`
	CompletionNotes     string     `gorm:"type:text" json:"completion_notes"`
	InspectionResult    string     `gorm:"type:varchar(20)" json:"inspection_result"`
	InspectionComment   string     `gorm:"type:text" json:"inspection_comment"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// MaintenanceSessionRecord 维修记录表
type MaintenanceSessionRecord struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	SessionID  string    `gorm:"type:uuid;index;not null" json:"session_id"`
	RecordType string    `gorm:"type:varchar(20)" json:"record_type"`
	Content    string    `gorm:"type:text" json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}
