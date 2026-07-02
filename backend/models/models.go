package models

import (
	"github.com/google/uuid"
	"github.com/lib/pq"
	"time"
)

type User struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IAMSub        string     `gorm:"type:varchar(255);not null;-:migration" json:"iam_sub"`
	TenantID      string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID         string     `gorm:"type:uuid;index;not null" json:"org_id"`
	Username      string     `gorm:"type:varchar(255)" json:"username"`
	Name          string     `gorm:"type:varchar(255)" json:"name"`
	Phone         string     `gorm:"type:varchar(50)" json:"phone"`
	Email         string     `gorm:"type:varchar(255)" json:"email"`
	CreditScore   int        `gorm:"default:600" json:"credit_score"`
	DepositMode   string     `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	IsShadow      bool       `gorm:"default:true" json:"is_shadow"`
	IsSystemAdmin bool       `gorm:"default:false" json:"is_system_admin"`
	Status        string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Position             string     `gorm:"type:varchar(100)" json:"position"`
	Role                 string     `gorm:"type:varchar(50)" json:"role"`
	ForcePasswordChange  bool       `gorm:"default:false" json:"force_password_change"`
	WxOpenid             string     `gorm:"type:varchar(128);index" json:"wx_openid"`
	MembershipLevelID    *int       `gorm:"type:int" json:"membership_level_id"`
	TotalSpending        float64    `gorm:"type:decimal;default:0" json:"total_spending"`
	PrepaidPoints        float64    `gorm:"type:decimal;default:0" json:"prepaid_points"`
	PromoPoints          float64    `gorm:"type:decimal;default:0" json:"promo_points"`
	OnboardingCompleted  bool       `gorm:"default:false" json:"onboarding_completed"`
	DeletedAt            *time.Time `gorm:"index" json:"deleted_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
	OrgID           *string          `gorm:"type:uuid;index" json:"org_id"`
	CategoryID      *string          `gorm:"type:uuid;index" json:"category_id"`
	CategoryName    string           `gorm:"type:varchar(100)" json:"category_name"`
	// Brand 字段已删除 - 遗留字段
	Level           string           `gorm:"type:varchar(20)" json:"level"`      // deprecated, use LevelID instead
	LevelName       string           `gorm:"type:varchar(50)" json:"level_name"` // deprecated
	LevelID         *uuid.UUID       `gorm:"type:uuid;index" json:"level_id"`
	InstrumentLevel *InstrumentLevel `gorm:"foreignKey:LevelID" json:"instrument_level,omitempty"`
	// Model 字段已删除 - 遗留字段
	SN              string           `gorm:"type:varchar(100)" json:"sn"`
	Site            string           `gorm:"type:varchar(255)" json:"site"` // legacy, 建议用 SiteID
	SiteID          *uuid.UUID       `gorm:"type:uuid;index" json:"site_id"`
	CurrentSiteID   *uuid.UUID       `gorm:"type:uuid;index" json:"current_site_id"`
	Description     string           `gorm:"type:text" json:"description"`
	Images          string           `gorm:"type:jsonb;default:'[]'" json:"images"`
	Video           string           `gorm:"type:varchar(500)" json:"video"`
	Poster          string           `gorm:"type:text" json:"poster"`
	Specifications  string           `gorm:"type:jsonb;default:'{}'" json:"specifications"`
	Pricing         string           `gorm:"type:jsonb;default:'{}'" json:"pricing"`
	BaseDailyRate   *float64         `gorm:"type:decimal(10,2)" json:"base_daily_rate"`
	PricingOverrides string          `gorm:"type:jsonb;default:'{}'" json:"pricing_overrides"`
	StockStatus     string           `gorm:"type:varchar(20);default:'available'" json:"stock_status"`
	RepairStatus    string           `gorm:"type:varchar(20);default:null" json:"repair_status"`
	RepairWorkerID  *string          `gorm:"type:uuid;index" json:"repair_worker_id"`
	Properties      string           `gorm:"type:jsonb;default:'{}'" json:"properties"`
	MinMembershipLevel *int          `gorm:"type:int" json:"min_membership_level"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const (
	StockStatusAvailable   = "available"
	StockStatusRented      = "rented"
	StockStatusMaintenance = "maintenance"
	StockStatusArchived    = "archived"
	StockStatusLost        = "lost"
	StockStatusSold        = "sold"
)

const (
	OrderStatusReserved         = "reserved"
	OrderStatusPaid             = "paid"
	OrderStatusPendingShipment  = "pending_shipment"
	OrderStatusInTransit        = "in_transit"
	OrderStatusShipped          = "shipped"
	OrderStatusInLease          = "in_lease"
	OrderStatusReturning        = "returning"
	OrderStatusReturned          = "returned"
	OrderStatusCompleted         = "completed"
	OrderStatusCancelled         = "cancelled"
	OrderStatusDepositRefunding  = "deposit_refunding"
	OrderStatusDamageAppealing   = "damage_appealing"
	OrderStatusExpired          = "expired"
	OrderStatusTransferred      = "transferred"
)

const (
	LeaseStatusActive         = "active"
	LeaseStatusReturnRequested = "return_requested"
	LeaseStatusCompleted      = "completed"
	LeaseStatusCancelled      = "cancelled"
)

const (
	MerchantTypeFull      = "full"
	MerchantTypeControlled = "controlled"
)

const (
	ForwardingStatusPending   = "pending"
	ForwardingStatusInTransit = "in_transit"
	ForwardingStatusReceived  = "received"
	ForwardingStatusReady     = "ready"
	ForwardingStatusLastMile  = "last_mile"
	ForwardingStatusDelivered = "delivered"
	ForwardingStatusCompleted = "completed"
	ForwardingStatusLost      = "lost"
	ForwardingStatusCancelled = "cancelled"
	ForwardingStatusException = "exception"
)

const (
	ForwardingDirectionOutbound = "outbound"
	ForwardingDirectionReturn   = "return"
)

// Notification 通知消息表
type Notification struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID      string    `gorm:"type:uuid;index" json:"org_id"`
	UserID     string    `gorm:"type:uuid;not null;index" json:"user_id"`
	Type       string    `gorm:"type:varchar(20);not null;index" json:"type"`
	Title      string    `gorm:"type:varchar(255);not null" json:"title"`
	Content    string    `gorm:"type:text" json:"content"`
	RefID      string    `gorm:"type:uuid;index" json:"ref_id"`
	RefType    string    `gorm:"type:varchar(50)" json:"ref_type"`
	ActionType string    `gorm:"type:varchar(20);default:'info'" json:"action_type"`
	ActionData string    `gorm:"type:jsonb" json:"action_data,omitempty"`
	Status     string    `gorm:"type:varchar(20);default:'unread';index" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// InstrumentPhotoSpec 乐器拍照要求规范表
type InstrumentPhotoSpec struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	CategoryID       string    `gorm:"type:uuid;index;not null" json:"category_id"`
	PhotoRequirements string   `gorm:"type:jsonb;default:'[]'" json:"photo_requirements"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Order struct {
	ID                string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID             string     `gorm:"type:uuid;index" json:"org_id"`
	UserID            string     `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID      string     `gorm:"type:uuid;not null" json:"instrument_id"`
	Level             string     `gorm:"type:varchar(20);not null" json:"level"`
	LeaseTerm         int        `gorm:"not null" json:"lease_term"`
	DepositMode       string     `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	MonthlyRent       float64    `gorm:"type:decimal(10,2);not null" json:"monthly_rent"`
	Deposit           float64    `gorm:"type:decimal(10,2);default:0" json:"deposit"`
	ShippingFee        float64    `gorm:"type:decimal(10,2);default:0" json:"shipping_fee"`
	AccumulatedMonths int        `gorm:"default:0" json:"accumulated_months"`
	Status            string     `gorm:"type:varchar(20);default:'reserved';index" json:"status"`
	StartDate         *string    `gorm:"type:date" json:"start_date"`
	EndDate           *string    `gorm:"type:date" json:"end_date"`
	TrackingNumber    *string    `gorm:"type:varchar(100);index" json:"tracking_number"`
	CourierCompany    *string    `gorm:"type:varchar(100)" json:"courier_company"`
	ShippedAt         *time.Time `gorm:"type:timestamp" json:"shipped_at"`
	DeliveredAt       *time.Time `gorm:"type:timestamp" json:"delivered_at"`
	ReturnedAt        *time.Time `gorm:"type:timestamp" json:"returned_at"`
	DepositRefunded     bool       `gorm:"column:deposit_refunded;default:false" json:"deposit_refunded"`
	PricingBreakdown    *string    `gorm:"type:jsonb" json:"pricing_breakdown"`
	CashPaid            float64    `gorm:"type:decimal(10,2);not null;default:0" json:"cash_paid"`
	PrepaidPointsUsed   float64    `gorm:"type:decimal(10,2);not null;default:0" json:"prepaid_points_used"`
	GiftPointsUsed      float64    `gorm:"type:decimal(10,2);not null;default:0" json:"gift_points_used"`
	PointsPolicySnapshot *string   `gorm:"type:jsonb" json:"points_policy_snapshot"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Settlement struct {
	ID                 string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID            string    `gorm:"type:uuid;not null;index" json:"order_id"`
	ActualRentDays     int       `gorm:"not null;default:0" json:"actual_rent_days"`
	ActualRentAmount   float64   `gorm:"type:decimal(10,2);not null;default:0" json:"actual_rent_amount"`
	OriginalRentAmount float64   `gorm:"type:decimal(10,2);not null;default:0" json:"original_rent_amount"`
	GiftPointsRefunded float64   `gorm:"type:decimal(10,2);not null;default:0" json:"gift_points_refunded"`
	CashRefundable     float64   `gorm:"type:decimal(10,2);not null;default:0" json:"cash_refundable"`
	PrepaidRefunded    float64   `gorm:"type:decimal(10,2);not null;default:0" json:"prepaid_refunded"`
	RefundMethod       string    `gorm:"type:varchar(20);not null;default:'prepaid'" json:"refund_method"`
	RefundStatus       string    `gorm:"type:varchar(20);not null;default:'pending'" json:"refund_status"`
	OverdueChargesTotal float64  `gorm:"type:decimal(10,2);not null;default:0" json:"overdue_charges_total"`
	Breakdown          string    `gorm:"type:jsonb;not null;default:'{}'" json:"breakdown"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type OverdueCharge struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID          string    `gorm:"type:uuid;not null;index" json:"order_id"`
	ChargeDate       string    `gorm:"type:date;not null;index" json:"charge_date"`
	Amount           float64   `gorm:"type:decimal(10,2);not null" json:"amount"`
	DeductedFromPrepaid float64 `gorm:"type:decimal(10,2);not null;default:0" json:"deducted_from_prepaid"`
	RemainingBalance float64   `gorm:"type:decimal(10,2);not null;default:0" json:"remaining_balance"`
	Status           string    `gorm:"type:varchar(20);not null;default:'success';index" json:"status"`
	FailureReason    *string   `gorm:"type:varchar(500)" json:"failure_reason"`
	CreatedAt        time.Time `json:"created_at"`
}

type OrderLog struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID      string    `gorm:"type:uuid;not null;index" json:"order_id"`
	Event        string    `gorm:"type:varchar(50);not null" json:"event"`
	OperatorID   *string   `gorm:"type:varchar(255)" json:"operator_id"`
	OperatorName *string   `gorm:"type:varchar(255)" json:"operator_name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Site struct {
	ID                string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID            string     `gorm:"type:uuid;index" json:"org_id"`
	OrganizationCode string    `gorm:"type:varchar(255);index" json:"organization_code"`
	ParentID         *uuid.UUID `gorm:"type:uuid;index" json:"parent_id"`
	ManagerID     *uuid.UUID `gorm:"column:manager_id;type:uuid;index" json:"manager_id"`
	Name          string     `gorm:"type:varchar(255);not null" json:"name"`
	Address       string     `gorm:"type:varchar(500)" json:"address"`
	Type          string     `gorm:"type:varchar(50)" json:"type"`
	Latitude      float64    `gorm:"type:decimal(10,6)" json:"latitude"`
	Longitude     float64    `gorm:"type:decimal(10,6)" json:"longitude"`
	Phone         string     `gorm:"type:varchar(50)" json:"phone"`
	BusinessHours string     `gorm:"type:varchar(100)" json:"business_hours"`
	Status        string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ManagerPending bool     `gorm:"default:false" json:"manager_pending"`
	DeletedAt     *time.Time `gorm:"index" json:"deleted_at"`
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
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `gorm:"index" json:"completed_at,omitempty"`
}

type BrandConfig struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TenantID     string    `gorm:"type:uuid;index" json:"tenant_id"`
	ClientID     string    `gorm:"type:varchar(100)uniqueIndexnot null" json:"client_id"`
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
	OrderID        string    `gorm:"type:uuiduniqueIndexnot null" json:"order_id"`
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
	ClientID     string    `gorm:"type:varchar(100)uniqueIndexnot null" json:"client_id"`
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
	Status            string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	TransactionDate string    `gorm:"type:date;not null" json:"transaction_date"`
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserInstrument represents an instrument owned by a user (for repair requests).
type UserInstrument struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         string    `gorm:"type:varchar(255);index;not null" json:"user_id"`
	SN             string    `gorm:"type:varchar(255);index;not null" json:"sn"`
	InstrumentType string    `gorm:"type:varchar(100)" json:"instrument_type"`
	Brand          string    `gorm:"type:varchar(100)" json:"brand"`
	Model          string    `gorm:"type:varchar(100)" json:"model"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RepairRequestStatus constants
const (
	RepairReqStatusPendingShip  = "pending_ship"
	RepairReqStatusPendingAssessment = "pending_assessment"
	RepairReqStatusShipping     = "shipping"
	RepairReqStatusInspecting   = "inspecting"
	RepairReqStatusQuoted       = "quoted"
	RepairReqStatusPendingPay   = "pending_payment"
	RepairReqStatusPendingCancel = "pending_cancel"
	RepairReqStatusRepairing    = "repairing"
	RepairReqStatusReturnPend   = "return_pending"
	RepairReqStatusReturned     = "returned"
	RepairReqStatusClosed       = "closed"
	RepairReqStatusAppealing    = "appealing"
)

// RepairRequest represents a customer repair request.
type RepairRequest struct {
	ID                   string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID             string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	SiteID               string    `gorm:"type:uuid;index;not null" json:"site_id"`
	UserID               string    `gorm:"type:varchar(255);index;not null" json:"user_id"`
	UserInstrumentID     string    `gorm:"type:uuid;index" json:"user_instrument_id"`
	Status               string    `gorm:"type:varchar(20);default:'pending_ship'" json:"status"`
	Description          string    `gorm:"type:text" json:"description"`
	Photos               string    `gorm:"type:jsonb;default:'[]'" json:"photos"`
	VideoURL             string    `gorm:"type:varchar(500)" json:"video_url"`
	QuoteAmount          *float64  `json:"quote_amount"`
	InspectionFee        *float64  `json:"inspection_fee"`
	ShippingFee          *float64  `json:"shipping_fee"`
	TrackingCompany      string    `gorm:"type:varchar(100)" json:"tracking_company"`
	TrackingNumber       string    `gorm:"type:varchar(100)" json:"tracking_number"`
	ReturnCompany        string    `gorm:"type:varchar(100)" json:"return_company"`
	ReturnTrackingNumber string    `gorm:"type:varchar(100)" json:"return_tracking_number"`
	WorkerID             *string   `gorm:"type:varchar(255)" json:"worker_id"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt             *time.Time `json:"closed_at"`
}

// RepairRequestRecord stores logs for a repair request.
type RepairRequestRecord struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairRequestID string    `gorm:"type:uuid;index;not null" json:"repair_request_id"`
	WorkerID        string    `gorm:"type:varchar(255)" json:"worker_id"`
	Comment         string    `gorm:"type:text" json:"comment"`
	Photos          string    `gorm:"type:jsonb;default:'[]'" json:"photos"`
	RecordType      string    `gorm:"type:varchar(20)" json:"record_type"`
	CreatedAt       time.Time `json:"created_at"`
}

type PointsTransaction struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID            string    `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID          string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Type              string    `gorm:"type:varchar(20);not null;index" json:"type"`
	Amount            float64   `gorm:"type:decimal(10,2);not null" json:"amount"`
	BalanceAfterPrepaid float64 `gorm:"type:decimal(10,2);not null;default:0" json:"balance_after_prepaid"`
	BalanceAfterPromo float64   `gorm:"type:decimal(10,2);not null;default:0" json:"balance_after_promo"`
	OrderID           *string   `gorm:"type:uuid;index" json:"order_id"`
	Description       string    `gorm:"type:varchar(500)" json:"description"`
	CreatedAt         time.Time `json:"created_at"`
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
	ID          string    `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RepairQuote stores a repair quote submitted by a technician for assessment.
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

// TransitRoute maps a controlled site to its transit site.
type TransitRoute struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ControlledSiteID string    `gorm:"type:uuid;index;not null" json:"controlled_site_id"`
	TransitSiteID    string    `gorm:"type:uuid;index;not null" json:"transit_site_id"`
	Priority         int       `gorm:"default:0" json:"priority"`
	IsDefault        bool      `gorm:"default:false" json:"is_default"`
	CreatedAt        time.Time `json:"created_at"`
}

// TransitOrderStatus constants
const (
	TransitOrderDispatching = "dispatching"
	TransitOrderArrived     = "arrived"
	TransitOrderRepacked    = "repacked"
	TransitOrderShipped     = "shipped"
)

// TransitOrder links a lease order with its transit workflow.
type TransitOrder struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID             string     `gorm:"type:uuid;index;not null" json:"order_id"`
	TransitSiteID       string     `gorm:"type:uuid;index;not null" json:"transit_site_id"`
	ControlledSiteID    string     `gorm:"type:uuid;index;not null" json:"controlled_site_id"`
	Status              string     `gorm:"type:varchar(20);default:'dispatching'" json:"status"`
	UnpackPhotos        string     `gorm:"type:jsonb;default:'[]'" json:"unpack_photos"`
	RepackCompany       string     `gorm:"type:varchar(100)" json:"repack_company"`
	RepackTrackingNumber string   `gorm:"type:varchar(100)" json:"repack_tracking_number"`
	TransitOrderNumber  string     `gorm:"type:varchar(50)" json:"transit_order_number"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// RepairTransitStatus constants
const (
	RepairTransitInbound    = "inbound"
	RepairTransitTransiting = "transiting"
	RepairTransitOutbound   = "outbound"
	RepairTransitSentBack   = "sent_back"
)

// RepairTransitOrder links a repair request with its transit workflow.
type RepairTransitOrder struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairRequestID     string     `gorm:"type:uuid;index" json:"repair_request_id"`
	TransitSiteID       string     `gorm:"type:uuid;index;not null" json:"transit_site_id"`
	ControlledSiteID    string     `gorm:"type:uuid;index;not null" json:"controlled_site_id"`
	Status              string     `gorm:"type:varchar(20);default:'inbound'" json:"status"`
	UnpackPhotos        string     `gorm:"type:jsonb;default:'[]'" json:"unpack_photos"`
	RepackCompany       string     `gorm:"type:varchar(100)" json:"repack_company"`
	RepackTrackingNumber string   `gorm:"type:varchar(100)" json:"repack_tracking_number"`
	TransitOrderNumber  string     `gorm:"type:varchar(50)" json:"transit_order_number"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// WarningStatus constants
const (
	WarningStatusOpen        = "open"
	WarningStatusAcknowledged = "acknowledged"
	WarningStatusResolved    = "resolved"
)

// WarningSeverity constants
const (
	WarningSeverityLow    = "low"
	WarningSeverityMedium = "medium"
	WarningSeverityHigh   = "high"
)

// Warning represents an alert record in the warning system.
type Warning struct {
	ID          string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SiteID      string     `gorm:"type:uuid;index" json:"site_id"`
	MerchantID  string     `gorm:"type:uuid;index" json:"merchant_id"`
	Reason      string     `gorm:"type:varchar(50);not null" json:"reason"`
	Category    string     `gorm:"type:varchar(30)" json:"category"`
	Level       string     `gorm:"type:varchar(10);default:'low'" json:"level"`
	ObjectType  string     `gorm:"type:varchar(30)" json:"object_type"`
	ObjectID    string     `gorm:"type:uuid;index" json:"object_id"`
	Description string     `gorm:"type:text" json:"description"`
	Status      string     `gorm:"type:varchar(20);default:'open'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy  *string    `gorm:"type:uuid" json:"resolved_by"`
}

// Banner stores WeChat homepage carousel images
type Banner struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string    `gorm:"type:uuid;index" json:"tenant_id"`
	ImageURL  string    `gorm:"type:varchar(500);not null" json:"image_url"`
	LinkURL   string    `gorm:"type:varchar(500)" json:"link_url"`
	Title     string    `gorm:"type:varchar(200)" json:"title"`
	SortOrder int       `gorm:"default:0;index" json:"sort_order"`
	Status    string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	BgColor   string    `gorm:"type:varchar(7);default:'#915F38'" json:"bg_color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
