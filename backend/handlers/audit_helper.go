package handlers

import (
	"encoding/json"
	"log"
	"time"

	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// writeAuditLog #2062: 统一审计写入。
//
// audit_logs.details 列类型为 **jsonb**（models/audit_log.go），必须写入合法 JSON
// 文本——历史上有 3 处写入纯文本且未检查错误，Postgres 报
// `invalid input syntax for type json (SQLSTATE 22P02)` 后被静默吞掉，审计从未落库。
//
// fire-and-forget：marshal/写库失败仅记日志，**不阻断业务主流程**
// （对齐 face_review.go / `writeTechProfileAudit` 既有容错口径）。
func writeAuditLog(db *gorm.DB, tenantID, userID, action, resourceType, resourceID string, details map[string]interface{}) {
	b, err := json.Marshal(details)
	if err != nil {
		log.Printf("[Audit] details marshal failed action=%s: %v", action, err)
		return
	}
	d := string(b)
	if err := db.Create(&models.AuditLog{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      &d,
		CreatedAt:    time.Now(),
	}).Error; err != nil {
		log.Printf("[Audit] write failed action=%s resource=%s: %v", action, resourceID, err)
	}
}
