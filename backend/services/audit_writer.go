package services

import (
	"log"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type AuditRecord struct {
	TenantID     string
	OrgID        *string
	UserID       string
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   string
	StatusCode   int
	Status       string
	ErrorMessage string
	Details      string
	RequestBody  string
	IPAddress    string
	UserAgent    string
}

type AuditWriter struct {
	records chan *AuditRecord
	done    chan bool
}

const auditChannelCap = 1024

func NewAuditWriter() *AuditWriter {
	w := &AuditWriter{
		records: make(chan *AuditRecord, auditChannelCap),
		done:    make(chan bool),
	}
	go w.loop()
	return w
}

func (w *AuditWriter) WriteSync(rec *AuditRecord) error {
	return w.save(rec)
}

func (w *AuditWriter) Write(rec *AuditRecord) {
	select {
	case w.records <- rec:
	default:
		log.Printf("[CRITICAL] Audit log channel full (cap=%d), dropping record: %s %s/%s",
			auditChannelCap, rec.Action, rec.ResourceType, rec.ResourceID)
	}
}

func (w *AuditWriter) Stop() {
	w.done <- true
}

func (w *AuditWriter) loop() {
	for {
		select {
		case rec := <-w.records:
			if err := w.save(rec); err != nil {
				log.Printf("[AuditWriter] failed to save audit record: %v", err)
			}
		case <-w.done:
			w.drain()
			return
		}
	}
}

func (w *AuditWriter) drain() {
	for {
		select {
		case rec := <-w.records:
			if err := w.save(rec); err != nil {
				log.Printf("[AuditWriter] drain failed to save audit record: %v", err)
			}
		default:
			return
		}
	}
}

func (w *AuditWriter) save(rec *AuditRecord) error {
	db := database.GetDB()
	entry := models.AuditLog{
		TenantID:     rec.TenantID,
		OrgID:        rec.OrgID,
		UserID:       rec.UserID,
		ActorRole:    rec.ActorRole,
		Action:       rec.Action,
		ResourceType: rec.ResourceType,
		ResourceID:   rec.ResourceID,
		StatusCode:   rec.StatusCode,
		Status:       rec.Status,
		IPAddress:    rec.IPAddress,
		UserAgent:    rec.UserAgent,
	}
	if rec.ErrorMessage != "" {
		entry.ErrorMessage = &rec.ErrorMessage
	}
	if rec.Details != "" {
		entry.Details = &rec.Details
	}
	if rec.RequestBody != "" {
		entry.RequestBody = &rec.RequestBody
	}
	return db.Create(&entry).Error
}
