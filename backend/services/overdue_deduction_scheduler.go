package services

import (
	"fmt"
	"log"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DefaultOverdueDiscountRate = 1.5
)

type OverdueDeductionScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewOverdueDeductionScheduler() *OverdueDeductionScheduler {
	return &OverdueDeductionScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *OverdueDeductionScheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 5, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}
		time.Sleep(time.Until(next))
		s.deductOverdue()

		for range s.ticker.C {
			s.deductOverdue()
		}
	}()
	log.Println("[OverdueDeductionScheduler] started")
}

func (s *OverdueDeductionScheduler) Stop() {
	s.ticker.Stop()
	s.done <- true
	log.Println("[OverdueDeductionScheduler] stopped")
}

func (s *OverdueDeductionScheduler) deductOverdue() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OverdueDeductionScheduler] panic: %v", r)
		}
	}()

	today := time.Now().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")

	var orders []models.Order
	if err := s.db.Where("status = ? AND end_date < ? AND deleted_at IS NULL",
		models.OrderStatusInLease, todayStr).Find(&orders).Error; err != nil {
		log.Printf("[OverdueDeductionScheduler] query error: %v", err)
		return
	}

	for _, order := range orders {
		s.processOverdueOrder(order, today)
	}

	if len(orders) > 0 {
		log.Printf("[OverdueDeductionScheduler] processed %d overdue orders", len(orders))
	}
}

func (s *OverdueDeductionScheduler) processOverdueOrder(order models.Order, today time.Time) {
	var existing models.OverdueCharge
	if err := s.db.Where("order_id = ? AND charge_date = ?", order.ID, today.Format("2006-01-02")).
		First(&existing).Error; err == nil {
		return
	}

	overdueRate := s.getOverdueRate(order)
	overdueAmount := order.MonthlyRent / 30 * overdueRate

	var user models.User
	if err := s.db.Where("id = ?", order.UserID).First(&user).Error; err != nil {
		log.Printf("[OverdueDeductionScheduler] user %s not found for order %s", order.UserID, order.ID)
		return
	}

	deducted := 0.0
	remaining := overdueAmount
	status := "failed"
	var failureReason *string

	if user.PrepaidPoints >= overdueAmount {
		deducted = overdueAmount
		remaining = 0
		status = "success"

		if err := s.db.Model(&user).Updates(map[string]interface{}{
			"prepaid_points": gorm.Expr("prepaid_points - ?", overdueAmount),
			"updated_at":     time.Now(),
		}).Error; err != nil {
			log.Printf("[OverdueDeductionScheduler] failed to deduct prepaid for user %s: %v", user.ID, err)
			reason := "扣款失败: " + err.Error()
			failureReason = &reason
			status = "failed"
		} else {
			pt := models.PointsTransaction{
				ID:                  uuid.New().String(),
				UserID:              user.ID,
				TenantID:            order.TenantID,
				Type:                "overdue_deduction",
				Amount:              -overdueAmount,
				BalanceAfterPrepaid: user.PrepaidPoints - overdueAmount,
				BalanceAfterPromo:   user.PromoPoints,
				OrderID:             &order.ID,
				Description:         "逾期自动扣款",
				CreatedAt:           time.Now(),
			}
			if err := s.db.Create(&pt).Error; err != nil {
				log.Printf("[OverdueDeductionScheduler] failed to record transaction: %v", err)
			}
		}
	} else if user.PrepaidPoints > 0 {
		deducted = user.PrepaidPoints
		remaining = overdueAmount - user.PrepaidPoints
		status = "partial"

		newBalance := 0.0
		if err := s.db.Model(&user).Updates(map[string]interface{}{
			"prepaid_points": 0,
			"updated_at":     time.Now(),
		}).Error; err != nil {
			log.Printf("[OverdueDeductionScheduler] partial deduction failed for user %s: %v", user.ID, err)
			reason := "部分扣款失败: " + err.Error()
			failureReason = &reason
			status = "failed"
		} else {
			pt := models.PointsTransaction{
				ID:                  uuid.New().String(),
				UserID:              user.ID,
				TenantID:            order.TenantID,
				Type:                "overdue_deduction",
				Amount:              -deducted,
				BalanceAfterPrepaid: newBalance,
				BalanceAfterPromo:   user.PromoPoints,
				OrderID:             &order.ID,
				Description:         "逾期部分扣款（预付点不足）",
				CreatedAt:           time.Now(),
			}
			if err := s.db.Create(&pt).Error; err != nil {
				log.Printf("[OverdueDeductionScheduler] failed to record transaction: %v", err)
			}
		}
	} else {
		reason := "预付点余额为0，无法自动扣款"
		failureReason = &reason
		status = "failed"
	}

	charge := models.OverdueCharge{
		ID:                 uuid.New().String(),
		OrderID:            order.ID,
		ChargeDate:         today.Format("2006-01-02"),
		Amount:             overdueAmount,
		DeductedFromPrepaid: deducted,
		RemainingBalance:   remaining,
		Status:             status,
		FailureReason:      failureReason,
		CreatedAt:          time.Now(),
	}

	if err := s.db.Create(&charge).Error; err != nil {
		log.Printf("[OverdueDeductionScheduler] failed to save overdue charge: %v", err)
	}

	if status == "failed" || status == "partial" {
		alert := models.Notification{
			ID:        uuid.New().String(),
			TenantID:  order.TenantID,
			OrgID:     order.OrgID,
			UserID:    order.UserID,
			Type:      "overdue_alert",
			Title:     "逾期扣款失败",
			Content:   "您的逾期租金自动扣款失败，欠款金额 " + formatFloat(remaining) + " 元，请尽快处理以免产生更多费用。",
			RefID:     order.ID,
			RefType:   "order",
			Status:    "unread",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.db.Create(&alert).Error; err != nil {
			log.Printf("[OverdueDeductionScheduler] failed to create alert: %v", err)
		}

		var siteManagers []models.User
		siteQuery := s.db.Where("tenant_id = ? AND org_id = ? AND role IN ?", order.TenantID, order.OrgID, []string{"site_admin", "merchant_admin"})
		if err := siteQuery.Find(&siteManagers).Error; err == nil {
			for _, mgr := range siteManagers {
				mgrAlert := models.Notification{
					ID:        uuid.New().String(),
					TenantID:  order.TenantID,
					OrgID:     order.OrgID,
					UserID:    mgr.ID,
					Type:      "overdue_alert",
					Title:     "逾期扣款失败通知",
					Content:   "以下订单逾期扣款失败，欠款 " + formatFloat(remaining) + " 元。订单ID: " + order.ID,
					RefID:     order.ID,
					RefType:   "order",
					Status:    "unread",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := s.db.Create(&mgrAlert).Error; err != nil {
					log.Printf("[OverdueDeductionScheduler] failed to create manager alert: %v", err)
				}
			}
		}
	}
}

func (s *OverdueDeductionScheduler) getOverdueRate(order models.Order) float64 {
	var detail models.PromoPlanDetail
	if err := s.db.Joins("JOIN promo_plans ON promo_plans.id = promo_plan_details.promo_plan_id").
		Where("promo_plans.plan_type = 'overdue_discount' AND promo_plans.is_active = ?", true).
		First(&detail).Error; err == nil && detail.OverdueDiscount > 0 {
		return detail.OverdueDiscount
	}
	return DefaultOverdueDiscountRate
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
