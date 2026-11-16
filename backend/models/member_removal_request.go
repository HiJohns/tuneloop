package models

import "time"

// MemberRemovalRequest 成员移除审批单（#2385 决策 D3 / #2394 实施）。
//
// 业务背景：网点管理员不能直接开除成员，必须提交移除申请，由商户管理员批准后
// 执行解绑（IAM 关系 + 本地成员行）。
//
// 状态机：
//
//	pending_approval ──approve──> approved ──execute──> executed
//	       │                        └──execute failed──> failed
//	       ├──reject──> rejected
//	       └──cancel──> cancelled
type MemberRemovalRequest struct {
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	MerchantID   string     `gorm:"type:uuid;not null;index" json:"merchant_id"`
	SiteID       *string    `gorm:"type:uuid;index" json:"site_id,omitempty"`
	TargetUserID string     `gorm:"type:uuid;not null;index" json:"target_user_id"`
	RequesterID  string     `gorm:"type:uuid;not null;index" json:"requester_id"`
	Reason       string     `gorm:"type:varchar(200);not null" json:"reason"`
	Status       string     `gorm:"type:varchar(20);not null;default:'pending_approval';index" json:"status"`
	DecidedBy    *string    `gorm:"type:uuid" json:"decided_by,omitempty"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	RejectReason *string    `gorm:"type:varchar(200)" json:"reject_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (MemberRemovalRequest) TableName() string { return "member_removal_requests" }
