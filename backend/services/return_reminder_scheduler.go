package services

import (
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReturnReminderScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewReturnReminderScheduler() *ReturnReminderScheduler {
	return &ReturnReminderScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *ReturnReminderScheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		s.checkReturnReminders()
		for range s.ticker.C {
			s.checkReturnReminders()
		}
	}()
	log.Println("[ReturnReminderScheduler] started")
}

func (s *ReturnReminderScheduler) Stop() {
	s.ticker.Stop()
	s.done <- true
	log.Println("[ReturnReminderScheduler] stopped")
}

func (s *ReturnReminderScheduler) checkReturnReminders() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ReturnReminderScheduler] panic recovered: %v", r)
		}
	}()

	today := time.Now().Truncate(24 * time.Hour)
	reminderDate := today.Add(5 * 24 * time.Hour)
	reminderDateStr := reminderDate.Format("2006-01-02")

	var orders []models.Order
	if err := s.db.Where("status = ? AND end_date = ? AND deleted_at IS NULL",
		models.OrderStatusInLease, reminderDateStr).Find(&orders).Error; err != nil {
		log.Printf("[ReturnReminderScheduler] query error: %v", err)
		return
	}

	for _, order := range orders {
		notification := models.Notification{
			ID:        uuid.New().String(),
			TenantID:  order.TenantID,
			OrgID:     order.OrgID,
			UserID:    order.UserID,
			Type:      "return_reminder",
			Title:     "归还提醒",
			Content:   "您的乐器租赁期即将结束，请在 5 天内归还乐器，以免产生逾期费用。",
			RefID:     order.ID,
			RefType:   "order",
			Status:    "unread",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.db.Create(&notification).Error; err != nil {
			log.Printf("[ReturnReminderScheduler] failed to create notification for order %s: %v", order.ID, err)
		}
	}

	if len(orders) > 0 {
		log.Printf("[ReturnReminderScheduler] created %d return reminders", len(orders))
	}
}
