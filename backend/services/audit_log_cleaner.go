package services

import (
	"log"
	"time"

	"gorm.io/gorm"

	"tuneloop-backend/database"
)

type AuditLogCleaner struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewAuditLogCleaner() *AuditLogCleaner {
	return &AuditLogCleaner{
		db:     database.GetDB(),
		done:   make(chan bool),
	}
}

func (c *AuditLogCleaner) Start() {
	c.ticker = time.NewTicker(24 * time.Hour)
	go func() {
		c.clean()
		for {
			select {
			case <-c.ticker.C:
				if err := c.clean(); err != nil {
					log.Printf("[AuditLogCleaner] clean error: %v", err)
				}
			case <-c.done:
				c.ticker.Stop()
				return
			}
		}
	}()
	log.Println("[AuditLogCleaner] started - runs daily")
}

func (c *AuditLogCleaner) Stop() {
	c.done <- true
}

func (c *AuditLogCleaner) clean() error {
	result := c.db.Exec("DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '1 year'")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("[AuditLogCleaner] deleted %d expired audit log records", result.RowsAffected)
	}
	return nil
}
