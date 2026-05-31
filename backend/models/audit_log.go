package models

import "time"

type AuditLog struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"type:uuid;index;not null" json:"tenant_id"`
	OrgID        *string   `gorm:"type:uuid;index" json:"org_id"`
	UserID       string    `gorm:"type:uuid;index;not null" json:"user_id"`
	ActorRole    string    `gorm:"type:varchar(50)" json:"actor_role"`
	Action       string    `gorm:"type:varchar(50);not null" json:"action"`
	ResourceType string    `gorm:"type:varchar(50);not null" json:"resource_type"`
	ResourceID   string    `gorm:"type:varchar(100)" json:"resource_id"`
	StatusCode   int       `gorm:"type:int" json:"status_code"`
	Status       string    `gorm:"type:varchar(20)" json:"status"`
	ErrorMessage *string   `gorm:"type:text" json:"error_message"`
	Details      *string   `gorm:"type:jsonb" json:"details"`
	RequestBody  *string   `gorm:"type:jsonb" json:"request_body"`
	IPAddress    string    `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent    string    `gorm:"type:varchar(500)" json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
