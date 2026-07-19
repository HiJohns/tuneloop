package services

import (
	"encoding/json"
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
		// Run immediately on start, then hourly
		s.processOverdue()

		for range s.ticker.C {
			s.processOverdue()
		}
	}()
	log.Println("[OverdueDeductionScheduler] started")
}

func (s *OverdueDeductionScheduler) Stop() {
	s.ticker.Stop()
	s.done <- true
	log.Println("[OverdueDeductionScheduler] stopped")
}

func (s *OverdueDeductionScheduler) processOverdue() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OverdueDeductionScheduler] panic: %v", r)
		}
	}()

	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	todayStr := today.Format("2006-01-02")
	yesterdayStr := yesterday.Format("2006-01-02")

	// Phase 1: Status transition — in_lease → expired (always run)
	s.transitionToExpired(todayStr)

	// Phase 2: Deduction — only at ~01:00, charge for yesterday
	if now.Hour() == 1 {
		s.deductYesterday(yesterdayStr)
	}
}

// transitionToExpired moves in_lease orders past their end_date to expired status.
func (s *OverdueDeductionScheduler) transitionToExpired(todayStr string) {
	result := s.db.Model(&models.Order{}).
		Where("status = ? AND end_date < ?", models.OrderStatusInLease, todayStr).
		Update("status", models.OrderStatusExpired)
	if result.Error != nil {
		log.Printf("[OverdueDeductionScheduler] status transition error: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[OverdueDeductionScheduler] transitioned %d orders in_lease → expired", result.RowsAffected)
	}
}

// deductYesterday charges overdue fee for yesterday on expired orders.
func (s *OverdueDeductionScheduler) deductYesterday(yesterdayStr string) {
	var orders []models.Order
	if err := s.db.Where("status = ? AND end_date < ?",
		models.OrderStatusExpired, yesterdayStr).Find(&orders).Error; err != nil {
		log.Printf("[OverdueDeductionScheduler] query error: %v", err)
		return
	}

	if len(orders) == 0 {
		return
	}

	for _, order := range orders {
		s.chargeDay(order, yesterdayStr)
	}

	log.Printf("[OverdueDeductionScheduler] charged %d overdue orders for %s", len(orders), yesterdayStr)
}

// chargeDay creates an overdue_charge for a specific date on a single order.
// Deducts from prepaid_points if available; otherwise marks as failed/partial.
func (s *OverdueDeductionScheduler) chargeDay(order models.Order, dateStr string) {
	// Idempotency check: skip if already charged for this date
	var existing models.OverdueCharge
	if err := s.db.Where("order_id = ? AND charge_date = ?", order.ID, dateStr).
		First(&existing).Error; err == nil {
		return
	}

	overdueRate := s.getOverdueRate(order)
	dailyRate := 0.0
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb map[string]interface{}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
			if v, ok := pb["final_daily_rent"].(float64); ok && v > 0 {
				dailyRate = v
			} else if v, ok := pb["base_daily_rent"].(float64); ok {
				dailyRate = v
			}
		}
	}
	if dailyRate <= 0 {
		dailyRate = order.MonthlyRent / 30
	}
	overdueAmount := dailyRate * overdueRate

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
		ChargeDate:         dateStr,
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
				if mgr.Email != "" {
					log.Printf("[OverdueDeductionScheduler] email notification for %s would be sent (stub): overdue order %s, amount %.2f", mgr.Email, order.ID, remaining)
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
