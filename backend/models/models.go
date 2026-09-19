package models

import (
	"github.com/google/uuid"
	"github.com/lib/pq"
	"time"
)

type User struct {
	ID                   string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IAMSub               string     `gorm:"type:varchar(255);not null;-:migration" json:"iam_sub"`
	TenantID             string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID                string     `gorm:"type:uuid;index;not null" json:"org_id"`
	Username             string     `gorm:"type:varchar(255)" json:"username"`
	Nickname             string     `gorm:"type:varchar(64)" json:"nickname"`
	Name                 string     `gorm:"type:varchar(255)" json:"name"`
	Phone                string     `gorm:"type:varchar(50)" json:"phone"`
	Email                string     `gorm:"type:varchar(255)" json:"email"`
	CreditScore          int        `gorm:"default:600" json:"credit_score"`
	DepositMode          string     `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"` // 已弃用，仅用于兼容旧数据，勿写入新值
	IsShadow             bool       `gorm:"default:true" json:"is_shadow"`
	IsSystemAdmin        bool       `gorm:"default:false" json:"is_system_admin"`
	Status               string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Position             string     `gorm:"type:varchar(100)" json:"position"`
	Role                 string     `gorm:"type:varchar(50)" json:"role"`
	ForcePasswordChange  bool       `gorm:"default:false" json:"force_password_change"`
	WxOpenid             string     `gorm:"type:varchar(128);index" json:"wx_openid"`
	WxUnionid            string     `gorm:"type:varchar(128)" json:"wx_unionid"`
	AvatarURL            string     `gorm:"type:varchar(500)" json:"avatar"`
	IsProfileCompleted   bool       `gorm:"default:false" json:"is_profile_completed"`
	MembershipLevelID    *int       `gorm:"type:int" json:"membership_level_id"`
	TotalSpending        Cents      `gorm:"type:bigint;default:0" json:"total_spending"`
	PrepaidPoints        Cents      `gorm:"type:bigint;default:0" json:"-"` // deprecated (#1531)
	// PromoPoints removed in #1983 (stage 2): points balance = SUM(point_batches.remaining_cents).
	OnboardingCompleted  bool       `gorm:"default:false" json:"onboarding_completed"`
	IdPhotoFront         *string    `gorm:"type:varchar(500)" json:"id_photo_front"`
	IdPhotoBack          *string    `gorm:"type:varchar(500)" json:"id_photo_back"`
	IdPhotoOther         *string    `gorm:"type:varchar(500)" json:"id_photo_other"`
	IdPhotoOtherType     *string    `gorm:"type:varchar(50)" json:"id_photo_other_type"`  // #1807: 第三证件类型（student/teacher/work/other）
	IdPhotoOtherVerified bool       `gorm:"default:false" json:"id_photo_other_verified"` // #1924: 第二证件审核通过（类型由审核员指定）
	RealName             *string    `gorm:"type:varchar(64)" json:"real_name"`
	IdCardNo             *string    `gorm:"type:varchar(18)" json:"id_card_no"`
	IdCardExpire         *string    `gorm:"type:varchar(20)" json:"id_card_expire"`     // #1807: 身份证有效期（YYYY-MM-DD 或「长期」），员工审核时按证件照填写
	IdCardAuthority      *string    `gorm:"type:varchar(100)" json:"id_card_authority"` // #1807: 签发机关（员工审核填写）
	IdCardAddress        *string    `gorm:"type:varchar(200)" json:"id_card_address"`   // #1807: 证件住址（员工审核填写）
	FaceVerified         bool       `gorm:"default:false" json:"face_verified"`
	FaceVerifiedAt       *time.Time `gorm:"column:face_verified_at" json:"face_verified_at"`
	FaceVerifyMethod     *string    `gorm:"type:varchar(10)" json:"face_verify_method"` // #1789 T1: tencent=自动比对 / manual=人工审核（信息变更时清除）
	RefCode              string     `gorm:"type:varchar(16)" json:"ref_code"`
	DeletedAt            *time.Time `gorm:"index" json:"deleted_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// FaceCaptureBatch (#1789 T1): 实名核身自拍采集批次（人工审核用）。
// 状态机：pending（待审核）→ approved（通过）/ rejected（驳回可重新采集）。
type FaceCaptureBatch struct {
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string     `gorm:"type:uuid;not null;index:idx_face_capture_batches_user" json:"user_id"`
	Status       string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`    // pending/approved/rejected
	Kind         string     `gorm:"type:varchar(20);not null;default:'registration'" json:"kind"` // #1924: registration / second_doc
	RejectReason *string    `gorm:"type:text" json:"reject_reason,omitempty"`
	SubmittedAt  time.Time  `gorm:"not null;default:now()" json:"submitted_at"`
	ReviewedBy   *string    `gorm:"type:varchar(255)" json:"reviewed_by,omitempty"` // 平台员工（本地 users 缓存 name/ID）
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Category struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Icon      string    `json:"icon"`
	ParentID  *string   `gorm:"type:uuid" json:"parent_id"`
	Level     int       `gorm:"default:1" json:"level"`
	Sort      int       `gorm:"default:1" json:"sort"`
	Visible   bool      `gorm:"default:true" json:"visible"`
	CreatedAt time.Time `json:"created_at"`
}

type Instrument struct {
	ID           string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string  `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID        *string `gorm:"type:uuid;index" json:"org_id"`
	CategoryID   *string `gorm:"type:uuid;index" json:"category_id"`
	CategoryName string  `gorm:"type:varchar(100)" json:"category_name"`
	// Brand 字段已删除 - 遗留字段
	Level           string           `gorm:"type:varchar(20)" json:"level"`      // deprecated, use LevelID instead
	LevelName       string           `gorm:"type:varchar(50)" json:"level_name"` // deprecated
	LevelID         *uuid.UUID       `gorm:"type:uuid;index" json:"level_id"`
	InstrumentLevel *InstrumentLevel `gorm:"foreignKey:LevelID" json:"instrument_level,omitempty"`
	// Model 字段已删除 - 遗留字段
	SN                 string     `gorm:"type:varchar(100)" json:"sn"`
	Site               string     `gorm:"type:varchar(255)" json:"site"` // legacy, 建议用 SiteID
	SiteID             *uuid.UUID `gorm:"type:uuid;index" json:"site_id"`
	CurrentSiteID      *uuid.UUID `gorm:"type:uuid;index" json:"current_site_id"`
	Description        string     `gorm:"type:text" json:"description"`
	Images             string     `gorm:"type:jsonb;default:'[]'" json:"images"`
	Video              string     `gorm:"type:varchar(500)" json:"video"`
	Poster             string     `gorm:"type:text" json:"poster"`
	CoverImage         string     `gorm:"type:text" json:"cover_image"`
	Deposit            *Cents     `gorm:"type:bigint;default:0" json:"deposit"`
	Specifications     string     `gorm:"type:jsonb;default:'{}'" json:"specifications"`
	Pricing            string     `gorm:"type:jsonb;default:'{}'" json:"pricing"`
	TotalPrice         *Cents     `gorm:"type:bigint" json:"total_price"`
	BaseDailyRate      *Cents     `gorm:"type:bigint" json:"base_daily_rate"`
	PricingOverrides   string     `gorm:"type:jsonb;default:'{}'" json:"pricing_overrides"`
	StockStatus        string     `gorm:"type:varchar(20);default:'available'" json:"stock_status"`
	RepairStatus       string     `gorm:"type:varchar(20)" json:"repair_status"`
	RepairWorkerID     *string    `gorm:"type:uuid;index" json:"repair_worker_id"`
	Properties         string     `gorm:"type:jsonb;default:'{}'" json:"properties"`
	MinMembershipLevel *int       `gorm:"type:int" json:"min_membership_level"`
	SortOrder          int        `gorm:"default:0;index" json:"sort_order"` // #1797: 子分类内排序（0=未排序，退化 created_at 序）
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Referral struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ReferrerID string    `gorm:"type:uuid;not null" json:"referrer_id"`
	RefereeID  string    `gorm:"type:uuid;not null;uniqueIndex" json:"referee_id"`
	RefCode    string    `gorm:"size:16;not null" json:"ref_code"`
	Status     string    `gorm:"size:20;not null;default:registered" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
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
	OrderStatusReserved              = "reserved"
	OrderStatusPaid                  = "paid"
	OrderStatusPendingShipment       = "pending_shipment"
	OrderStatusInTransit             = "in_transit"
	OrderStatusShipped               = "shipped"
	OrderStatusInLease               = "in_lease"
	OrderStatusReturning             = "returning"
	OrderStatusReturned              = "returned"
	OrderStatusCompleted             = "completed"
	OrderStatusCancelled             = "cancelled"
	OrderStatusDepositRefunding      = "deposit_refunding"
	OrderStatusDamageAppealing       = "damage_appealing"
	OrderStatusPendingDamageResponse = "pending_damage_response"
	OrderStatusExpired               = "expired"
	OrderStatusTransferred           = "transferred"
)

const (
	LeaseStatusActive          = "active"
	LeaseStatusReturnRequested = "return_requested"
	LeaseStatusCompleted       = "completed"
	LeaseStatusCancelled       = "cancelled"
)

const (
	MerchantTypeFull       = "full"
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
	ActionData *string   `gorm:"type:jsonb" json:"action_data,omitempty"`
	Status     string    `gorm:"type:varchar(20);default:'unread';index" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// InstrumentPhotoSpec 乐器拍照要求规范表
type InstrumentPhotoSpec struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	CategoryID        string    `gorm:"type:uuid;index;not null" json:"category_id"`
	PhotoRequirements string    `gorm:"type:jsonb;default:'[]'" json:"photo_requirements"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Order struct {
	ID                   string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID             string `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID                string `gorm:"type:uuid;index" json:"org_id"`
	UserID               string `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID         string `gorm:"type:uuid;not null" json:"instrument_id"`
	Level                string `gorm:"type:varchar(20);not null" json:"level"`
	LeaseTerm            int    `gorm:"not null" json:"lease_term"`
	DepositMode          string `gorm:"type:varchar(20);default:'standard'" json:"deposit_mode"`
	MonthlyRent          Cents  `gorm:"type:bigint;not null" json:"monthly_rent"`
	Deposit              Cents  `gorm:"type:bigint;default:0" json:"deposit"`
	DepositWaived        bool   `gorm:"column:deposit_waived;not null;default:false" json:"deposit_waived"`
	RecommendationLetter string `gorm:"column:recommendation_letter;type:varchar(500);not null;default:''" json:"recommendation_letter"` // #1867: deposit-free application letter URL
	ShippingFee          Cents  `gorm:"type:bigint;default:0" json:"shipping_fee"`
	AccumulatedMonths    int    `gorm:"default:0" json:"accumulated_months"`
	// #1965 业务订单号：`YL<YYYYMMDD>-<NNN>`（当日第 N 单），唯一索引
	OrderNo                 string     `gorm:"type:varchar(24);uniqueIndex" json:"order_no"`
	Status                  string     `gorm:"type:varchar(40);default:'reserved';index" json:"status"`
	StartDate               *string    `gorm:"type:date" json:"start_date"`
	EndDate                 *string    `gorm:"type:date" json:"end_date"`
	TrackingNumber          *string    `gorm:"type:varchar(100);index" json:"tracking_number"`
	CourierCompany          *string    `gorm:"type:varchar(100)" json:"courier_company"`
	ShippedAt               *time.Time `gorm:"type:timestamp" json:"shipped_at"`
	DeliveredAt             *time.Time `gorm:"type:timestamp" json:"delivered_at"`
	ReturnedAt              *time.Time `gorm:"type:timestamp" json:"returned_at"`
	DepositRefunded         bool       `gorm:"column:deposit_refunded;default:false" json:"deposit_refunded"`
	PricingBreakdown        *string    `gorm:"type:jsonb" json:"pricing_breakdown"`
	CashPaid                Cents      `gorm:"type:bigint;not null;default:0" json:"cash_paid"`
	PrepaidPointsUsed       Cents      `gorm:"type:bigint;not null;default:0" json:"prepaid_points_used"`
	GiftPointsUsed          Cents      `gorm:"type:bigint;not null;default:0" json:"gift_points_used"`
	PointsPolicySnapshot    *string    `gorm:"type:jsonb" json:"points_policy_snapshot"`
	RequestSnapshot         *string    `gorm:"type:jsonb" json:"request_snapshot"`
	PricingConfigSnapshot   *string    `gorm:"type:jsonb" json:"pricing_config_snapshot"`
	CouponCode              *string    `gorm:"type:varchar(32)" json:"coupon_code,omitempty"`
	CouponDiscount          Cents      `gorm:"type:bigint;not null;default:0" json:"coupon_discount"` // #1744: 优惠码折扣金额（分）
	InvoiceApplied          bool       `gorm:"not null;default:false" json:"invoice_applied"`
	InvoiceAppliedAt        *time.Time `gorm:"type:timestamptz" json:"invoice_applied_at"`
	CurrentPaymentSessionID *string    `gorm:"type:uuid" json:"current_payment_session_id,omitempty"`
	PaymentDeadline         *time.Time `json:"payment_deadline"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type MerchantSettlementConfig struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	ReceiverType     string    `gorm:"type:varchar(20);not null;default:'merchant'" json:"receiver_type"`
	ReceiverAccount  string    `gorm:"type:varchar(128);not null" json:"receiver_account"`
	ProfitShareRatio float64   `gorm:"type:decimal(5,2);not null;default:0" json:"profit_share_ratio"`
	IsEnabled        bool      `gorm:"not null;default:true" json:"is_enabled"`
	PaymentTimeout   *int      `gorm:"type:integer" json:"payment_timeout"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Settlement struct {
	ID                  string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID             string    `gorm:"type:uuid;not null;index" json:"order_id"`
	ActualRentDays      int       `gorm:"not null;default:0" json:"actual_rent_days"`
	ActualRentAmount    Cents     `gorm:"type:bigint;not null;default:0" json:"actual_rent_amount"`
	OriginalRentAmount  Cents     `gorm:"type:bigint;not null;default:0" json:"original_rent_amount"`
	GiftPointsRefunded  Cents     `gorm:"type:bigint;not null;default:0" json:"gift_points_refunded"`
	CashRefundable      Cents     `gorm:"type:bigint;not null;default:0" json:"cash_refundable"`
	PrepaidRefunded     Cents     `gorm:"type:bigint;not null;default:0" json:"prepaid_refunded"`
	RefundMethod        string    `gorm:"type:varchar(20);not null;default:'prepaid'" json:"refund_method"`
	RefundStatus        string    `gorm:"type:varchar(20);not null;default:'pending'" json:"refund_status"`
	OverdueChargesTotal Cents     `gorm:"type:bigint;not null;default:0" json:"overdue_charges_total"`
	Breakdown           string    `gorm:"type:jsonb;not null;default:'{}'" json:"breakdown"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SettlementCalculation (#1738 P2): append-only audit trail of every
// settlement computation (preview via GET calculate, confirm via POST).
// Stores the order input snapshot and the computed result (cents JSONB)
// so any displayed amount can be traced to a persisted calculation.
type SettlementCalculation struct {
	ID            string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID       string  `gorm:"type:uuid;index;not null" json:"order_id"`
	TenantID      string  `gorm:"type:uuid;index" json:"tenant_id"`
	Trigger       string  `gorm:"type:varchar(20);not null" json:"trigger"` // preview | confirm
	InputSnapshot *string `gorm:"type:jsonb" json:"input_snapshot,omitempty"`
	// Result is TEXT (not JSONB): #1738 audit contract requires the stored
	// bytes to be byte-identical to what the handler responded — JSONB
	// normalization (key reorder + whitespace) would break that guarantee.
	Result     *string   `gorm:"type:text" json:"result,omitempty"`
	ActualDays int       `gorm:"not null;default:0" json:"actual_days"`
	CreatedAt  time.Time `json:"created_at"`
}

type OverdueCharge struct {
	ID                  string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID             string    `gorm:"type:uuid;not null;index" json:"order_id"`
	ChargeDate          string    `gorm:"type:date;not null;index" json:"charge_date"`
	Amount              Cents     `gorm:"type:bigint;not null" json:"amount"`
	DeductedFromDeposit Cents     `gorm:"type:bigint;not null;default:0" json:"deducted_from_deposit"`
	DeductedFromPrepaid Cents     `gorm:"type:bigint;not null;default:0" json:"deducted_from_prepaid"`
	RemainingBalance    float64   `gorm:"type:decimal(10,2);not null;default:0" json:"remaining_balance"`
	Status              string    `gorm:"type:varchar(20);not null;default:'success';index" json:"status"`
	FailureReason       *string   `gorm:"type:varchar(500)" json:"failure_reason"`
	CreatedAt           time.Time `json:"created_at"`
}

type OrderLog struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID      string    `gorm:"type:uuid;not null;index" json:"order_id"`
	Event        string    `gorm:"type:varchar(50);not null" json:"event"`
	OperatorID   *string   `gorm:"type:varchar(255)" json:"operator_id"`
	OperatorName *string   `gorm:"type:varchar(255)" json:"operator_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// PaymentSession 统一支付会话表（替代 OrderPaymentRecord + OrderRefundRecord）
type PaymentSession struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string    `gorm:"type:uuid;index:idx_payment_sessions_tenant;not null" json:"tenant_id"`
	UserID         string    `gorm:"type:uuid;index:idx_payment_sessions_user;not null" json:"user_id"`
	Type           string    `gorm:"type:varchar(20);not null;default:'payment'" json:"type"`
	Status         string    `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Amount         Cents     `gorm:"type:bigint;not null" json:"amount"`
	Breakdown      *string   `gorm:"type:jsonb" json:"breakdown,omitempty"`
	WalletSnapshot *string   `gorm:"type:jsonb" json:"wallet_snapshot,omitempty"`
	OutTradeNo     *string   `gorm:"type:varchar(32);uniqueIndex" json:"out_trade_no"`
	TransactionID  *string   `gorm:"type:varchar(64)" json:"transaction_id"`
	Method         *string   `gorm:"type:varchar(20)" json:"method"`
	FailReason     *string   `gorm:"type:text" json:"fail_reason"`
	RawResponse    *string   `gorm:"type:jsonb" json:"raw_response"`
	RefundFromID   *string   `gorm:"type:uuid" json:"refund_from_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SessionOrderLink 支付会话与订单的多对多关联（支持合并付款）
type SessionOrderLink struct {
	SessionID string `gorm:"type:uuid;primaryKey;not null" json:"session_id"`
	OrderID   string `gorm:"type:varchar(32);primaryKey;not null" json:"order_id"`
}

// OrderPaymentRecord 支付记录表（包含完整审计字段）— 已废弃，由 PaymentSession 替代
type OrderPaymentRecord struct {
	ID             string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string     `gorm:"type:uuid;index:idx_payment_tenant;not null" json:"tenant_id"`
	OrgID          *string    `gorm:"type:uuid" json:"org_id,omitempty"`
	UserID         string     `gorm:"type:uuid;index:idx_payment_user;not null" json:"user_id"`
	OrderID        *string    `gorm:"type:uuid;index:idx_payment_order" json:"order_id,omitempty"`
	SessionID      *string    `gorm:"type:uuid" json:"session_id,omitempty"` // two-phase registration session (#1663) — survives the callback (RawResponse is overwritten by the callback result)
	OrderType      string     `gorm:"type:varchar(20);not null" json:"order_type"`
	OutTradeNo     *string    `gorm:"type:varchar(32);uniqueIndex" json:"out_trade_no"`
	TransactionID  *string    `gorm:"type:varchar(64)" json:"transaction_id"`
	OpenID         string     `gorm:"column:openid;type:varchar(128)" json:"openid"` // payer.openid from payment callback (#1731)
	Amount         Cents      `gorm:"type:bigint;not null" json:"amount"`
	Type           string     `gorm:"type:varchar(20);not null;default:'payment'" json:"type"`
	Status         string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Method         *string    `gorm:"type:varchar(20)" json:"method"`
	PrepayID       *string    `gorm:"type:varchar(64)" json:"prepay_id"`
	CodeURL        *string    `gorm:"type:text" json:"code_url"`
	FailReason     *string    `gorm:"type:text" json:"fail_reason"`
	RawResponse    *string    `gorm:"type:jsonb" json:"raw_response"`
	RemindedAt     *time.Time `gorm:"type:timestamp" json:"reminded_at,omitempty"`           // #1749 L-04D: 催缴幂等标记
	Days           *int       `gorm:"type:integer" json:"days,omitempty"`                    // #1802 T1: 续费天数独立持久化（RawResponse 会被微信回调覆盖）
	CouponCode     *string    `gorm:"type:varchar(32)" json:"coupon_code,omitempty"`         // #1853: 本笔支付优惠码（逐笔入库）
	CouponDiscount Cents      `gorm:"type:bigint;not null;default:0" json:"coupon_discount"` // #1853: 本笔支付折扣（分，无码 0）
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// OrderRefundRecord 退款记录表（支持部分退款，与支付记录一对多）
type OrderRefundRecord struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID        string    `gorm:"type:uuid;not null" json:"tenant_id"`
	PaymentRecordID *string   `gorm:"type:uuid;index" json:"payment_record_id,omitempty"`
	OutRefundNo     *string   `gorm:"type:varchar(32);uniqueIndex" json:"out_refund_no"`
	RefundID        *string   `gorm:"type:varchar(64)" json:"refund_id"`
	Amount          Cents     `gorm:"type:bigint;not null" json:"amount"`
	Reason          *string   `gorm:"type:varchar(200)" json:"reason"`
	Status          string    `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	FailReason      *string   `gorm:"type:text" json:"fail_reason"`
	RawResponse     *string   `gorm:"type:jsonb" json:"raw_response"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Site struct {
	ID               string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID            string     `gorm:"type:uuid;index" json:"org_id"`
	OrganizationCode string     `gorm:"type:varchar(255);index" json:"organization_code"`
	ParentID         *uuid.UUID `gorm:"type:uuid;index" json:"parent_id"`
	ManagerID        *uuid.UUID `gorm:"column:manager_id;type:uuid;index" json:"manager_id"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	Address          string     `gorm:"type:varchar(500)" json:"address"`
	Type             string     `gorm:"type:varchar(50)" json:"type"`
	Latitude         float64    `gorm:"type:decimal(10,6)" json:"latitude"`
	Longitude        float64    `gorm:"type:decimal(10,6)" json:"longitude"`
	Phone            string     `gorm:"type:varchar(50)" json:"phone"`
	ContactName      string     `gorm:"type:varchar(255)" json:"contact_name"` // #1935: 中转网点联系人（audit Bug4：模型缺列致 Update 500）
	PostalCode       string     `gorm:"type:varchar(20)" json:"postal_code"`
	BusinessHours    string     `gorm:"type:varchar(100)" json:"business_hours"`
	Status           string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ManagerPending   bool       `gorm:"default:false" json:"manager_pending"`
	DeletedAt        *time.Time `gorm:"index" json:"deleted_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	EstimatedCost      Cents      `gorm:"type:bigint;default:0" json:"estimated_cost"`
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
	MonthlyRent   Cents     `gorm:"type:bigint;not null" json:"monthly_rent"`
	DepositAmount Cents     `gorm:"type:bigint;not null" json:"deposit_amount"`
	Status        string    `gorm:"type:varchar(20);default:'active';index" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Deposit struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID        string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	LeaseID         string    `gorm:"type:uuid;index;not null" json:"lease_id"`
	UserID          string    `gorm:"type:uuid;index;not null" json:"user_id"`
	Amount          Cents     `gorm:"type:bigint;not null" json:"amount"`
	Type            string    `gorm:"type:varchar(20);not null" json:"type"`
	Status          string    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	TransactionDate string    `gorm:"type:date;not null" json:"transaction_date"`
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
// v3: added transit_processing, transit_in, transit_out
//
//	deprecated inspecting, quoted, pending_cancel
const (
	RepairReqStatusPendingShip       = "pending_ship"
	RepairReqStatusPendingAssessment = "pending_assessment"
	RepairReqStatusShipping          = "shipping"
	RepairReqStatusInspecting        = "inspecting" // Deprecated: v3, use pending_assessment
	RepairReqStatusQuoted            = "quoted"     // Deprecated: v3, use pending_assessment
	RepairReqStatusPendingPay        = "pending_payment"
	RepairReqStatusPendingCancel     = "pending_cancel" // Deprecated: v3
	RepairReqStatusRepairing         = "repairing"
	RepairReqStatusReturnPend        = "return_pending"
	RepairReqStatusReturned          = "returned"
	RepairReqStatusClosed            = "closed"
	RepairReqStatusAppealing         = "appealing"
	RepairReqStatusTransitProcessing = "transit_processing"
	RepairReqStatusTransitIn         = "transit_in"
	RepairReqStatusTransitOut        = "transit_out"
	// #1942 维修服务（type='service'）状态
	RepairReqStatusPendingQuote  = "pending_quote"
	RepairReqStatusPaid          = "paid"
	RepairReqStatusAdjustPending = "adjust_pending"
	RepairReqStatusDoneRepair    = "done_repair"
)

// RepairRequest represents a customer repair request.
type RepairRequest struct {
	ID                   string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID             string     `gorm:"type:uuid;index" json:"tenant_id"`
	SiteID               string     `gorm:"type:uuid;index" json:"site_id"`
	UserID               string     `gorm:"type:varchar(255);index;not null" json:"user_id"`
	UserInstrumentID     string     `gorm:"type:uuid;index" json:"user_instrument_id"`
	Status               string     `gorm:"type:varchar(20);default:'pending_ship'" json:"status"`
	MerchantType         string     `gorm:"type:varchar(10);default:'full'" json:"merchant_type"` // v3: full / controlled
	TransitSiteID        *string    `gorm:"type:uuid;index" json:"transit_site_id"`               // v3: selected transit site (controlled path)
	ControlledSiteID     *string    `gorm:"type:uuid;index" json:"controlled_site_id"`            // v3: accepted quote's controlled site
	AcceptedQuoteID      *string    `gorm:"type:uuid;index" json:"accepted_quote_id"`             // v3
	CheckFeeSnapshot     *Cents     `json:"check_fee_snapshot"`                                   // v3: system check_fee at payment time
	PaidAmount           *Cents     `gorm:"type:bigint" json:"paid_amount"`                       // v3: total amount paid
	ExpireAt             *time.Time `json:"expire_at"`                                            // v3: pending_assessment expiry
	ReminderSent         bool       `gorm:"default:false" json:"reminder_sent"`                   // v3: 24h reminder sent flag
	Description          string     `gorm:"type:text" json:"description"`
	Photos               string     `gorm:"type:jsonb;default:'[]'" json:"photos"`
	VideoURL             string     `gorm:"type:varchar(500)" json:"video_url"`
	QuoteAmount          *Cents     `json:"quote_amount"`   // Deprecated: v3, use repair_quotes
	InspectionFee        *Cents     `json:"inspection_fee"` // Deprecated: v3, use check_fee_snapshot
	ShippingFee          *Cents     `json:"shipping_fee"`
	TrackingCompany      string     `gorm:"type:varchar(100)" json:"tracking_company"`
	TrackingNumber       string     `gorm:"type:varchar(100)" json:"tracking_number"`
	ReturnCompany        string     `gorm:"type:varchar(100)" json:"return_company"`
	ReturnTrackingNumber string     `gorm:"type:varchar(100)" json:"return_tracking_number"`
	WorkerID             *string    `gorm:"type:varchar(255)" json:"worker_id"`
	// #1942 维修服务（单项服务商品）：type='service' 分支字段
	Type                string     `gorm:"type:varchar(20);default:'warranty';index" json:"type"`
	RepairCode          *string    `gorm:"type:varchar(6);uniqueIndex" json:"repair_code"` // 6 位唯一编码（数字+大写字母），warranty 为 NULL
	TechnicianID        *string    `gorm:"type:uuid;index" json:"technician_id"`           // 师傅指派
	QuoteRepairCents    *Cents     `gorm:"type:bigint" json:"quote_repair_cents"`          // 报价：修理费
	QuoteLogisticsCents *Cents     `gorm:"type:bigint" json:"quote_logistics_cents"`       // 报价：物流费预估
	QuoteStatus         string     `gorm:"type:varchar(20);default:''" json:"quote_status"`
	AdjustedQuoteCents  *Cents     `gorm:"type:bigint" json:"adjusted_quote_cents"`  // 加价后新总价
	IncurredRepairCents *Cents     `gorm:"type:bigint" json:"incurred_repair_cents"` // 到此为止修理费
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ClosedAt            *time.Time `json:"closed_at"`
}

// RepairLogisticsFee 维修服务分段物流费（#1942，RS-05）：每段发运时经手员工实填
type RepairLogisticsFee struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairID    string    `gorm:"type:uuid;index;not null" json:"repair_id"`
	Leg         int       `gorm:"not null" json:"leg"`
	AmountCents Cents     `gorm:"type:bigint;not null;default:0" json:"amount_cents"`
	FilledBy    string    `gorm:"type:varchar(255)" json:"filled_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// RepairReview 维修服务评价（#1942，RS-09）：评分/留言/拍照，PC 后台可见
type RepairReview struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairID  string    `gorm:"type:uuid;uniqueIndex;not null" json:"repair_id"`
	UserID    string    `gorm:"type:uuid;index;not null" json:"user_id"`
	Rating    int       `gorm:"not null" json:"rating"`
	Message   string    `gorm:"type:text" json:"message"`
	Photos    string    `gorm:"type:jsonb;default:'[]'" json:"photos"`
	CreatedAt time.Time `json:"created_at"`
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
	ID                  string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID              string    `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID            string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Type                string    `gorm:"type:varchar(20);not null;index" json:"type"`
	Amount              Cents     `gorm:"type:bigint;not null" json:"amount"`
	BalanceAfterPrepaid Cents     `gorm:"type:bigint;not null;default:0" json:"balance_after_prepaid"`
	BalanceAfterPromo   float64   `gorm:"type:decimal(10,2);not null;default:0" json:"balance_after_promo"`
	OrderID             *string   `gorm:"type:uuid;index" json:"order_id"`
	Description         string    `gorm:"type:varchar(500)" json:"description"`
	CreatedAt           time.Time `json:"created_at"`
}

// PointBatch 乐币批次（#1947 Sub-D）：发放建批次，按 FIFO 消费；expires_at NULL=不过期。
type PointBatch struct {
	ID             string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         string     `gorm:"type:uuid;not null;index" json:"user_id"`
	SourceType     string     `gorm:"type:varchar(20);not null;index" json:"source_type"` // signup|referral|fission|purchase|activity|migration|manual
	SourceRef      string     `gorm:"type:varchar(64)" json:"source_ref"`
	AmountCents    Cents      `gorm:"type:bigint;not null" json:"amount_cents"`
	RemainingCents Cents      `gorm:"type:bigint;not null" json:"remaining_cents"`
	AcquiredAt     time.Time  `gorm:"type:timestamptz;not null" json:"acquired_at"`
	ExpiresAt      *time.Time `gorm:"type:timestamptz;index" json:"expires_at"` // NULL=不过期
	ExpiredCents   Cents      `gorm:"type:bigint;not null;default:0" json:"expired_cents"`
	ExpiredAt      *time.Time `gorm:"type:timestamptz" json:"expired_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// PointBatchConsumption 批次扣减留痕（#1947 Sub-D）：一次消费可跨多批次。
type PointBatchConsumption struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TransactionID string    `gorm:"type:uuid;not null;index" json:"transaction_id"`
	BatchID       string    `gorm:"type:uuid;not null;index" json:"batch_id"`
	AmountCents   Cents     `gorm:"type:bigint;not null" json:"amount_cents"`
	ConsumedAt    time.Time `gorm:"type:timestamptz;not null" json:"consumed_at"`
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

// InstrumentLevel represents the skill level for instruments
type InstrumentLevel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Caption   string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"caption"`
	Code      string    `gorm:"type:varchar(20);uniqueIndex;not null" json:"code"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type Property struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Name              string    `gorm:"type:varchar(100);not null" json:"name"`
	PropertyType      string    `gorm:"type:varchar(20);not null" json:"property_type"`
	IsRequired        bool      `gorm:"default:false" json:"is_required"`
	Unit              string    `gorm:"type:varchar(50)" json:"unit"`
	Caption           string    `gorm:"type:varchar(100);not null" json:"caption"`
	ScopeType         string    `gorm:"type:varchar(20);default:'global'" json:"scope_type"`
	RelatedCategoryID *string   `gorm:"type:uuid;index" json:"related_category_id"`
	RelatedPropertyID *string   `gorm:"type:uuid;index" json:"related_property_id"`
	Description       string    `gorm:"type:text" json:"description"`
	Status            string    `gorm:"type:varchar(20);default:'active';not null" json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type PropertyOption struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	PropertyName     string    `gorm:"type:varchar(100);index" json:"property_name"`
	Value            string    `gorm:"type:varchar(255);not null" json:"value"`
	Status           string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Alias            *string   `gorm:"type:uuid;index" json:"alias"`
	ScopeCategoryID  *string   `gorm:"type:uuid;index" json:"scope_category_id"`
	ScopeParentValue *string   `gorm:"type:varchar(255)" json:"scope_parent_value"`
	SubmitterID      string    `gorm:"type:varchar(255)" json:"submitter_id"`
	SiteID           *string   `gorm:"type:uuid" json:"site_id"`
	MerchantID       string    `gorm:"type:uuid" json:"merchant_id"`
	InstrumentID     string    `gorm:"type:uuid" json:"instrument_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
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

// RepairRecord stores individual repair session records (comments + photos)
type RepairRecord struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InstrumentID string    `gorm:"type:uuid;not null;index" json:"instrument_id"`
	WorkerID     string    `gorm:"type:varchar(255);not null" json:"worker_id"`
	Comment      string    `gorm:"type:text" json:"comment"`
	Photos       string    `gorm:"type:jsonb;default:'[]'" json:"photos"`
	CreatedAt    time.Time `json:"created_at"`
}

// LeaseSession 租赁会话表
type LeaseSession struct {
	ID              string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID        string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID           *string    `gorm:"type:uuid;index" json:"org_id"`
	OrderID         string     `gorm:"type:uuid;not null;index" json:"order_id"`
	UserID          string     `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID    string     `gorm:"type:uuid;not null" json:"instrument_id"`
	StartDate       time.Time  `gorm:"type:date" json:"start_date"`
	EndDate         time.Time  `gorm:"type:date" json:"end_date"`
	ActualEndDate   *time.Time `gorm:"type:date" json:"actual_end_date,omitempty"`
	Status          string     `gorm:"type:varchar(20);default:'active';index" json:"status"`
	DeliveryAddress *string    `gorm:"type:jsonb" json:"delivery_address,omitempty"`
	ReturnMethod    string     `gorm:"type:varchar(20)" json:"return_method"`
	ReturnTracking  string     `gorm:"type:varchar(100)" json:"return_tracking"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ForwardingSession 转发会话表
type ForwardingSession struct {
	ID               string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID            string `gorm:"type:uuid;index" json:"org_id"`
	LeaseSessionID   string `gorm:"type:uuid;not null;index" json:"lease_session_id"`
	OrderID          string `gorm:"type:uuid;index" json:"order_id"`
	MerchantID       string `gorm:"type:uuid;index" json:"merchant_id"`
	ForwardingSiteID string `gorm:"type:uuid;index" json:"forwarding_site_id"`
	Direction        string `gorm:"type:varchar(20);not null" json:"direction"`
	Status           string `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	SessionCode      string `gorm:"type:varchar(6);uniqueIndex" json:"session_code"`
	InstrumentID     string `gorm:"type:uuid;index" json:"instrument_id"`
	TrackingNumbers  string `gorm:"type:jsonb" json:"tracking_numbers"`
	// #1934: 段级物流留痕（中转发货回填 / 中转收货拍照）
	TrackingCompany   string    `gorm:"type:varchar(100);not null;default:''" json:"tracking_company"`
	TrackingNumber    string    `gorm:"type:varchar(100);not null;default:''" json:"tracking_number"`
	Photos            *string   `gorm:"type:jsonb" json:"photos"`
	LogisticsFeeCents Cents     `gorm:"type:bigint;not null;default:0" json:"logistics_fee_cents"`
	Notes             string    `gorm:"type:text" json:"notes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TransitShippingFee 受控商户中转物流费分段（#1934，docs/cases/transit.md v1）：
// 承担矩阵 —— 顾客承担 ①(受控→中转, outbound) + ②(中转→顾客, outbound) + ③(顾客→中转, return)；
// 商户承担 ④(中转→受控, return)。订单详情「物流费」= SUM(paid_by='customer')。
type TransitShippingFee struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID    string    `gorm:"type:uuid;index;not null" json:"order_id"`
	Direction  string    `gorm:"type:varchar(20);not null" json:"direction"` // outbound / return
	Segment    int       `gorm:"not null" json:"segment"`                    // 1..4（矩阵编号）
	Amount     Cents     `gorm:"type:bigint;not null;default:0" json:"amount"`
	PaidBy     string    `gorm:"type:varchar(20);not null" json:"paid_by"` // customer / merchant
	RecordedBy string    `gorm:"type:varchar(255);not null" json:"recorded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

func (TransitShippingFee) TableName() string { return "transit_shipping_fees" }

// ElectronicContract 电子合同表
type ElectronicContract struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID          *string   `gorm:"type:uuid;index" json:"org_id"`
	OrderID        string    `gorm:"type:uuid;not null;index" json:"order_id"`
	UserID         string    `gorm:"type:uuid;not null;index" json:"user_id"`
	InstrumentID   string    `gorm:"type:uuid;not null" json:"instrument_id"`
	ContractURL    string    `gorm:"type:varchar(500);not null" json:"contract_url"`
	ContractNumber string    `gorm:"type:varchar(50);unique" json:"contract_number"`
	GeneratedAt    time.Time `json:"generated_at"`
	Status         string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// DamageReport 定损报告表
type DamageReport struct {
	ID                string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID             string     `gorm:"type:uuid;index" json:"org_id"`
	LeaseID           string     `gorm:"type:uuid;not null;index" json:"lease_id"`
	InstrumentID      string     `gorm:"type:uuid;not null" json:"instrument_id"`
	UserID            string     `gorm:"type:uuid;not null;index" json:"user_id"`
	DamageAmount      *Cents     `gorm:"type:bigint" json:"damage_amount,omitempty"`
	DamageDescription string     `gorm:"type:text" json:"damage_description"`
	AssessedBy        *string    `gorm:"type:uuid;index" json:"assessed_by"`
	AssessedAt        *time.Time `json:"assessed_at"`
	DepositDeducted   Cents      `gorm:"type:bigint;default:0" json:"deposit_deducted"`
	Status            string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	// 验收字段（#1708 并入）：所有验收（含 good 无损坏）统一
	// 写入 damage_reports（原定损评估表已合并废弃）。
	Condition             string     `gorm:"type:varchar(20)" json:"condition"`
	Notes                 string     `gorm:"type:text" json:"notes"`
	ScanTime              *time.Time `json:"scan_time"`
	OverdueDays           int        `gorm:"default:0" json:"overdue_days"`
	OverdueFee            Cents      `gorm:"type:bigint;default:0" json:"overdue_fee"`
	AdditionalShippingFee Cents      `gorm:"type:bigint;default:0" json:"additional_shipping_fee"` // #1801: 归还时追加物流费
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// AppealStatus constants
const (
	AppealStatusPending   = "pending"
	AppealStatusReviewing = "reviewing" // v3: transit site employee reviewing/desensitizing
	AppealStatusForwarded = "forwarded" // v3: forwarded to controlled site admin
	AppealStatusClosed    = "closed"
)

// Appeal 申诉记录表
type Appeal struct {
	ID               string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID            string     `gorm:"type:uuid;index" json:"org_id"`
	SiteID           string     `gorm:"type:uuid;index" json:"site_id"`
	Category         string     `gorm:"type:varchar(30)" json:"category"`
	ObjectType       string     `gorm:"type:varchar(30)" json:"object_type"`
	ObjectID         string     `gorm:"type:uuid;index" json:"object_id"`
	AppellantID      string     `gorm:"type:varchar(255)" json:"appellant_id"`
	Description      string     `gorm:"type:text" json:"description"`
	Images           string     `gorm:"type:jsonb;default:'[]'" json:"images"`
	DamageReportID   *string    `gorm:"type:uuid;index" json:"damage_report_id,omitempty"`
	UserID           *string    `gorm:"type:uuid;index" json:"user_id,omitempty"`
	AppealReason     *string    `gorm:"type:text" json:"appeal_reason,omitempty"`
	ReviewerID       string     `gorm:"type:varchar(255)" json:"reviewer_id,omitempty"`                                      // v3: transit site employee reviewing
	DesensitizedDesc string     `gorm:"type:text;column:desensitized_description" json:"desensitized_description,omitempty"` // v3: stripped of user contact info
	ForwardedTo      string     `gorm:"type:varchar(255)" json:"forwarded_to,omitempty"`                                     // v3: controlled site admin
	Status           string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	SubmittedAt      time.Time  `json:"submitted_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Resolution       string     `gorm:"type:varchar(20)" json:"resolution"`
	FinalAmount      *Cents     `gorm:"type:bigint" json:"final_amount,omitempty"`
	ManagerComment   string     `gorm:"type:text" json:"manager_comment"`
	ResolvedBy       *string    `gorm:"type:uuid" json:"resolved_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ClosedAt         *time.Time `json:"closed_at"`
}

// OrderStatusHistory 订单状态历史表
type OrderStatusHistory struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID      *string   `gorm:"type:uuid;index" json:"org_id"`
	OrderID    string    `gorm:"type:uuid;not null;index" json:"order_id"`
	StatusFrom string    `gorm:"type:varchar(40)" json:"status_from"`
	StatusTo   string    `gorm:"type:varchar(40)" json:"status_to"`
	Notes      string    `gorm:"type:text" json:"notes"`
	ChangedBy  *string   `gorm:"type:uuid;index" json:"changed_by"`
	ChangedAt  time.Time `json:"changed_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (OrderStatusHistory) TableName() string {
	return "order_status_history"
}

// Merchant represents a merchant/organization entity aligned with IAM Organization
type Merchant struct {
	ID                 string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID           string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID              string    `gorm:"type:uuid;index;not null" json:"org_id"`
	Name               string    `gorm:"type:varchar(255);not null" json:"name"`
	Code               string    `gorm:"type:varchar(100)" json:"code"`
	ContactName        string    `gorm:"type:varchar(255)" json:"contact_name"`
	ContactEmail       string    `gorm:"type:varchar(255)" json:"contact_email"`
	ContactPhone       string    `gorm:"type:varchar(50)" json:"contact_phone"`
	Phone              string    `gorm:"type:varchar(50)" json:"phone"`
	Address            string    `gorm:"type:text" json:"address"`
	AdminUID           string    `gorm:"type:uuid;index" json:"admin_uid"`
	AdminPending       bool      `gorm:"default:false" json:"admin_pending"`
	Status             string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	MerchantType       string    `gorm:"type:varchar(20);default:'full'" json:"merchant_type"`
	TransitAddress     string    `gorm:"type:text" json:"transit_address"`
	TransitPhone       string    `gorm:"type:varchar(50)" json:"transit_phone"`
	TransitContactName string    `gorm:"type:varchar(255)" json:"transit_contact_name"`
	RebateOptIn        bool      `gorm:"default:true" json:"rebate_opt_in"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// SiteMember represents the many-to-many relationship between users and sites
type SiteMember struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	SiteID    string    `gorm:"type:uuid;not null;index:idx_site_members_unique" json:"site_id"`
	UserID    string    `gorm:"type:uuid;not null;index:idx_site_members_unique" json:"user_id"`
	Role      string    `gorm:"type:varchar(20);default:'Staff'" json:"role"`
	Status    string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	IamTaskID string    `gorm:"type:varchar(255)" json:"iam_task_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MerchantMember is the merchant-level counterpart of SiteMember.
// IAM binding org for a merchant is its own tenant_id.
type MerchantMember struct {
	ID           string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string         `gorm:"type:uuid;not null;index:idx_mm_tenant" json:"tenant_id"`
	MerchantID   string         `gorm:"type:uuid;not null;index:idx_mm_merchant" json:"merchant_id"`
	UserID       string         `gorm:"type:uuid;not null;index:idx_mm_unique" json:"user_id"`
	Role         string         `gorm:"type:varchar(20);default:'site_member'" json:"role"`
	Status       string         `gorm:"type:varchar(20);default:'active'" json:"status"`
	CusPermCodes pq.StringArray `gorm:"type:text[];default:'{}'" json:"cus_perm_codes"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (MerchantMember) TableName() string { return "merchant_members" }

// ConfirmationSession handles user invitation confirmation flow
type ConfirmationSession struct {
	ID             string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       string     `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID          string     `gorm:"type:uuid;index;not null" json:"org_id"`
	UserID         string     `gorm:"type:uuid;not null;index" json:"user_id"`
	ConfirmType    string     `gorm:"type:varchar(20);not null" json:"confirm_type"`
	ConfirmTarget  string     `gorm:"type:varchar(255);not null" json:"confirm_target"`
	MerchantID     string     `gorm:"type:uuid;index" json:"merchant_id"`
	ActionType     string     `gorm:"type:varchar(50);not null" json:"action_type"`
	ActionTargetID string     `gorm:"type:uuid" json:"action_target_id"`
	IAMSessionID   string     `gorm:"type:varchar(255);index" json:"iam_session_id"`
	CallbackURL    string     `gorm:"type:varchar(500)" json:"callback_url"`
	Status         string     `gorm:"type:varchar(20);default:'waiting';index" json:"status"`
	Message        string     `gorm:"type:text" json:"message"`
	Token          string     `gorm:"type:varchar(100);uniqueIndex" json:"token"`
	ExpiresAt      time.Time  `gorm:"not null" json:"expires_at"`
	ConfirmedAt    *time.Time `json:"confirmed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// InstrumentPhotoBatch stores photo batch metadata for instruments
type InstrumentPhotoBatch struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InstrumentID string    `gorm:"type:uuid;index;not null" json:"instrument_id"`
	BatchType    string    `gorm:"type:varchar(20);not null;index" json:"batch_type"`
	StoragePath  string    `gorm:"type:varchar(500);not null" json:"storage_path"`
	OperatorID   string    `gorm:"type:uuid;index" json:"operator_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// Role defines a role template with cus_perm codes (IAM cache)
type Role struct {
	ID            string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID      string         `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_code" json:"tenant_id"`
	IAMTemplateID string         `gorm:"type:varchar(100)" json:"iam_template_id"`
	Name          string         `gorm:"type:varchar(100);not null" json:"name"`
	Code          string         `gorm:"type:varchar(50);not null;uniqueIndex:idx_tenant_code" json:"code"`
	CusPermCodes  pq.StringArray `gorm:"type:text[];default:'{}'" json:"cus_perm_codes"`
	IsSystem      bool           `gorm:"default:false" json:"is_system"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type PricingTemplate struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code            string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	Description     string    `gorm:"type:text" json:"description"`
	ConfigSchema    string    `gorm:"type:jsonb;not null;default:'{}'" json:"config_schema"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	IsSystemDefault bool      `gorm:"default:false" json:"is_system_default"`
	CreatedAt       time.Time `json:"created_at"`
}

type MerchantPricingConfig struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   string    `gorm:"type:uuid;uniqueIndex;not null" json:"tenant_id"`
	TemplateID string    `gorm:"type:uuid;not null" json:"template_id"`
	Config     string    `gorm:"type:jsonb;not null;default:'{}'" json:"config"`
	UpdatedAt  time.Time `json:"updated_at"`
	UpdatedBy  string    `gorm:"type:varchar(255)" json:"updated_by"`
}

type InstrumentMedia struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID        string    `gorm:"type:uuid;index" json:"org_id"`
	InstrumentID *string   `gorm:"type:uuid;index" json:"instrument_id"`
	ObjectType   string    `gorm:"type:varchar(30)" json:"object_type"`
	ObjectID     *string   `gorm:"type:uuid;index" json:"object_id"`
	BatchID      string    `gorm:"type:uuid;index;not null" json:"batch_id"`
	BatchType    string    `gorm:"type:varchar(20);not null" json:"batch_type"`
	FileName     string    `gorm:"type:varchar(255);not null" json:"file_name"`
	FileType     string    `gorm:"type:varchar(10);not null" json:"file_type"`
	FileSize     int64     `gorm:"default:0" json:"file_size"`
	StorageKey   string    `gorm:"type:varchar(500);not null" json:"storage_key"`
	IsDisplay    bool      `gorm:"default:false" json:"is_display"`
	SortOrder    int       `gorm:"default:0" json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// UserAddress stores user's shipping addresses
type UserAddress struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        string    `gorm:"type:uuid;index;not null" json:"user_id"`
	RecipientName string    `gorm:"type:varchar(100)" json:"recipient_name"`
	Phone         string    `gorm:"type:varchar(50)" json:"phone"`
	Province      string    `gorm:"type:varchar(50)" json:"province"`
	City          string    `gorm:"type:varchar(50)" json:"city"`
	District      string    `gorm:"type:varchar(50)" json:"district"`
	Detail        string    `gorm:"type:varchar(500)" json:"detail"`
	PostalCode    string    `gorm:"type:varchar(20)" json:"postal_code"`
	IsDefault     bool      `gorm:"default:false" json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SystemSetting struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null;uniqueIndex:idx_setting_tenant_key" json:"tenant_id"`
	SettingKey   string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_setting_tenant_key" json:"setting_key"`
	SettingValue string    `gorm:"type:text;not null;default:''" json:"setting_value"`
	UpdatedAt    time.Time `json:"updated_at"`
	UpdatedBy    string    `gorm:"type:varchar(255)" json:"updated_by"`
}

// MediaAsset tracks every physical file written under uploads/media/ for
// orphan detection and periodic cleanup. instrument_media remains the
// authoritative source for business media; MediaAsset is a unified index only.
type MediaAsset struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	StorageKey       string    `gorm:"type:varchar(500);not null;uniqueIndex:uq_media_assets_storage_key" json:"storage_key"`
	SourceType       string    `gorm:"type:varchar(30);not null" json:"source_type"`
	SourceID         string    `gorm:"type:varchar(100)" json:"source_id"`
	IsReferenced     bool      `gorm:"not null;default:true" json:"is_referenced"`
	RefCount         int       `gorm:"not null;default:1" json:"ref_count"`
	FileSize         int64     `gorm:"type:bigint" json:"file_size"`
	FileType         string    `gorm:"type:varchar(10)" json:"file_type"`
	CreatedAt        time.Time `json:"created_at"`
	LastReferencedAt time.Time `json:"last_referenced_at"`
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
	ID                   string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID              string    `gorm:"type:uuid;index;not null" json:"order_id"`
	TransitSiteID        string    `gorm:"type:uuid;index;not null" json:"transit_site_id"`
	ControlledSiteID     string    `gorm:"type:uuid;index;not null" json:"controlled_site_id"`
	Status               string    `gorm:"type:varchar(20);default:'dispatching'" json:"status"`
	UnpackPhotos         string    `gorm:"type:jsonb;default:'[]'" json:"unpack_photos"`
	RepackCompany        string    `gorm:"type:varchar(100)" json:"repack_company"`
	RepackTrackingNumber string    `gorm:"type:varchar(100)" json:"repack_tracking_number"`
	TransitOrderNumber   string    `gorm:"type:varchar(50)" json:"transit_order_number"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// RepairTransitStatus constants
// v3: added pending_activation/active/received/relayed; deprecated inbound/transiting/outbound/sent_back
const (
	RepairTransitInbound           = "inbound"    // Deprecated: v3 direction=in, status=active
	RepairTransitTransiting        = "transiting" // Deprecated: v3
	RepairTransitOutbound          = "outbound"   // Deprecated: v3 direction=out, status=active
	RepairTransitSentBack          = "sent_back"  // Deprecated: v3
	RepairTransitPendingActivation = "pending_activation"
	RepairTransitActive            = "active"
	RepairTransitReceived          = "received"
	RepairTransitRelayed           = "relayed"
)

// RepairTransitOrderDirection constants
const (
	RepairTransitDirIn  = "in"
	RepairTransitDirOut = "out"
)

// RepairTransitOrder links a repair request with its transit workflow.
type RepairTransitOrder struct {
	ID                   string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RepairRequestID      string    `gorm:"type:uuid;index" json:"repair_request_id"`
	TransitSiteID        string    `gorm:"type:uuid;index;not null" json:"transit_site_id"`
	ControlledSiteID     string    `gorm:"type:uuid;index" json:"controlled_site_id"`
	Direction            string    `gorm:"type:varchar(10)" json:"direction"` // v3: in/out
	Status               string    `gorm:"type:varchar(20);default:'pending_activation'" json:"status"`
	TransitServiceFee    *Cents    `gorm:"type:bigint" json:"transit_service_fee"`   // v3
	TransitLogisticsFee  *Cents    `gorm:"type:bigint" json:"transit_logistics_fee"` // v3
	Note                 string    `gorm:"type:text" json:"note"`                    // v3
	UnpackPhotos         string    `gorm:"type:jsonb;default:'[]'" json:"unpack_photos"`
	RepackCompany        string    `gorm:"type:varchar(100)" json:"repack_company"`
	RepackTrackingNumber string    `gorm:"type:varchar(100)" json:"repack_tracking_number"`
	TransitOrderNumber   string    `gorm:"type:varchar(50)" json:"transit_order_number"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// WarningStatus constants
const (
	WarningStatusOpen         = "open"
	WarningStatusAcknowledged = "acknowledged"
	WarningStatusResolved     = "resolved"
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
	// #1898: last time a warning e-mail was sent (cooldown/repeat window).
	LastNotifiedAt *time.Time `gorm:"type:timestamptz" json:"last_notified_at,omitempty"`
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

// InvoiceApplication represents a customer's invoice request grouped by merchant.
// One application = one merchant's set of orders from a single submission.
type InvoiceApplication struct {
	ID       string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID   string `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID string `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Status   string `gorm:"type:varchar(20);not null;default:'pending'" json:"status"` // pending | replied
	// #1941 发票信息（按商户分组，每个分组=一张发票）
	InvoiceType string     `gorm:"type:varchar(20);not null;default:'普通'" json:"invoice_type"` // 普通 | 专用
	Title       string     `gorm:"type:varchar(255);not null;default:''" json:"title"`         // 发票抬头
	TaxNumber   string     `gorm:"type:varchar(50)" json:"tax_number"`                         // 税号（个人抬头可空）
	TotalAmount Cents      `gorm:"type:bigint;not null;default:0" json:"total_amount"`
	OrderCount  int        `gorm:"not null;default:0" json:"order_count"`
	Reply       *string    `gorm:"type:text" json:"reply"`
	InvoiceFile *string    `gorm:"type:text" json:"invoice_file"`
	RepliedAt   *time.Time `gorm:"type:timestamptz" json:"replied_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Orders is a computed join field, not stored in DB
	Orders       []Order `gorm:"-" json:"orders,omitempty"`
	MerchantName string  `gorm:"-" json:"merchant_name,omitempty"`
}

// InstrumentLossRecord 乐器丢失记录（#1948，LS-00）：
// 员工裁量制——责任方/比例/赔偿额由员工填写，系统按 LS-03 分场景结算；
// 冲正/恢复留痕见 LS-05/LS-05a。
type InstrumentLossRecord struct {
	ID                string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID          string     `gorm:"type:uuid;index" json:"tenant_id"`
	InstrumentID      string     `gorm:"type:uuid;index;not null" json:"instrument_id"`
	OrderID           *string    `gorm:"type:uuid;index" json:"order_id"`                    // 关联租约订单（纯库存为 NULL）
	ResponsibleParty  string     `gorm:"type:varchar(20);not null" json:"responsible_party"` // user|logistics|platform|site
	UserRatio         int        `gorm:"not null;default:0" json:"user_ratio"`               // 0-100
	CompensationCents Cents      `gorm:"type:bigint;not null;default:0" json:"compensation_cents"`
	UserBurdenCents   Cents      `gorm:"type:bigint;not null;default:0" json:"user_burden_cents"` // 员工可覆盖
	Description       string     `gorm:"type:text" json:"description"`
	Photos            string     `gorm:"type:jsonb;default:'[]'" json:"photos"`
	SettledAt         *time.Time `json:"settled_at"` // 租约结算完成时间（纯库存 NULL）
	SettleBreakdown   string     `gorm:"type:jsonb;default:'{}'" json:"settle_breakdown"`
	CreatedBy         string     `gorm:"type:varchar(255)" json:"created_by"`
	// 恢复留痕（LS-05）
	RestoredAt         *time.Time `json:"restored_at"`
	RestoredDamaged    bool       `gorm:"default:false" json:"restored_damaged"`
	RestoreDescription string     `gorm:"type:text" json:"restore_description"`
	RestorePhotos      string     `gorm:"type:jsonb;default:'[]'" json:"restore_photos"`
	// ⛔ 冲正留痕 —— **已作废（2026-09-18）**：LS-05a 取消找回冲正，找回仅「恢复上架」。
	// 列保留以兼容历史行，**新流程不再写入**（#1979）。
	ReversedAt          *time.Time `json:"reversed_at"`
	ReversedAmountCents Cents      `gorm:"type:bigint;default:0" json:"reversed_amount_cents"`
	DeductedDamageCents Cents      `gorm:"type:bigint;default:0" json:"deducted_damage_cents"`
	DeductedIdleCents   Cents      `gorm:"type:bigint;default:0" json:"deducted_idle_cents"`
	ReverseNote         string     `gorm:"type:text" json:"reverse_note"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TechnicianProfile 维修师傅档案（#1974 T1，2026-09-18 设计变更）：
// 师傅**直属商户**（tenant 级，不再挂靠网点）；档案含个人照片/介绍/专长年限。
type TechnicianProfile struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     string    `gorm:"type:uuid;index;not null" json:"user_id"`
	TenantID   string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	Photo      string    `gorm:"type:varchar(500)" json:"photo"`
	Bio        string    `gorm:"type:text" json:"bio"`
	Experience string    `gorm:"type:jsonb;default:'[]'" json:"experience"` // [{craft, years}]
	Status     string    `gorm:"type:varchar(20);default:'active';index" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OrderNoPrefix 业务订单号前缀（#1965）：`YL<YYYYMMDD>-<NNN>`（当日第 N 单）
const OrderNoPrefix = "YL"
