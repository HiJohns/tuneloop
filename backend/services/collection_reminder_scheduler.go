package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// CollectionReminderScheduler (#1749 L-04D)：每日维护事务——扫描超过 24h
// 未完成补缴的订单，向商户管理员 + 网点管理员发送催缴报警通知（线下催缴）。
// 周期 ≤1h（对齐 L-04D「周期 1h 内扫描」）。
type CollectionReminderScheduler struct {
	db     *gorm.DB
	ticker *time.Ticker
	done   chan bool
}

func NewCollectionReminderScheduler() *CollectionReminderScheduler {
	return &CollectionReminderScheduler{
		db:   database.GetDB(),
		done: make(chan bool),
	}
}

func (s *CollectionReminderScheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		s.process()
		for {
			select {
			case <-s.ticker.C:
				if err := s.process(); err != nil {
					log.Printf("[CollectionReminderScheduler] error: %v", err)
				}
			case <-s.done:
				s.ticker.Stop()
				return
			}
		}
	}()
	log.Println("[CollectionReminderScheduler] started - hourly scan for shortfall payments overdue 24h")
}

func (s *CollectionReminderScheduler) Stop() {
	s.done <- true
}

// process 扫描 payment_shortfall pending 记录（created_at < now-24h 且未提醒），
// 发送催缴通知并标记 reminded_at（幂等：同一订单仅报警一次）。不改订单状态
// （L-04D 关键规则：催缴仅通知，订单保持等待补缴）。
func (s *CollectionReminderScheduler) process() error {
	cutoff := time.Now().Add(-24 * time.Hour)

	var overdueRecords []models.OrderPaymentRecord
	if err := s.db.
		Where("order_type = ? AND status = ? AND created_at < ? AND reminded_at IS NULL",
			"payment_shortfall", "pending", cutoff).
		Find(&overdueRecords).Error; err != nil {
		return err
	}

	reminded := 0
	for _, rec := range overdueRecords {
		if err := s.remindOrder(rec); err != nil {
			log.Printf("[CollectionReminderScheduler] failed to remind order %s: %v", orderIDOf(rec), err)
			continue
		}
		reminded++
	}

	if len(overdueRecords) > 0 {
		log.Printf("[CollectionReminderScheduler] reminded %d/%d overdue shortfall orders", reminded, len(overdueRecords))
	}
	return nil
}

func orderIDOf(rec models.OrderPaymentRecord) string {
	if rec.OrderID == nil {
		return ""
	}
	return *rec.OrderID
}

// remindOrder 为单个逾期补缴订单发催缴报警（商户管理员 + 网点管理员），
// 日志留痕，标记 reminded_at（幂等）。
func (s *CollectionReminderScheduler) remindOrder(rec models.OrderPaymentRecord) error {
	orderID := orderIDOf(rec)
	if orderID == "" {
		return fmt.Errorf("shortfall record %s has no order_id", rec.ID)
	}

	// 顾客信息（姓名/手机）从本地 users 缓存读取
	var order models.Order
	if err := s.db.Where("id = ?", orderID).First(&order).Error; err != nil {
		return fmt.Errorf("load order: %w", err)
	}
	customerName := ""
	customerPhone := ""
	var customer models.User
	if err := s.db.Where("id = ?", order.UserID).First(&customer).Error; err == nil {
		customerName = customer.Name
		customerPhone = customer.Phone
	}
	outTradeNo := ""
	if rec.OutTradeNo != nil {
		outTradeNo = *rec.OutTradeNo
	}
	overdueHours := int(time.Since(rec.CreatedAt).Hours())

	content := fmt.Sprintf(
		"顾客 %s（%s）的补缴已超时 %d 小时：订单 %s，需补缴 ¥%.2f。请线下联系顾客催缴。",
		customerName, customerPhone, overdueHours, orderID[:min(8, len(orderID))], rec.Amount.ToYuan(),
	)
	ad := map[string]interface{}{
		"order_id":         orderID,
		"shortfall_amount": int64(rec.Amount),
		"customer_name":    customerName,
		"customer_phone":   customerPhone,
		"out_trade_no":     outTradeNo,
	}
	adJSON, _ := json.Marshal(ad)
	adStr := string(adJSON)

	// 网点管理员 + 网点员工（site_admin/site_member）——通知锚定订单乐器所属网点
	var siteID string
	var inst models.Instrument
	if err := s.db.Where("id = ?", order.InstrumentID).First(&inst).Error; err == nil {
		if inst.SiteID != nil {
			siteID = inst.SiteID.String()
		} else if inst.CurrentSiteID != nil {
			siteID = inst.CurrentSiteID.String()
		}
	}
	if siteID != "" {
		NotifyUsersBySiteWithAction(s.db, order.TenantID, siteID,
			"collect_reminder", "补缴超时催缴", content, orderID, "order",
			[]string{"site_admin", "site_member"}, "collect_reminder", &adStr)
	}
	// 商户管理员
	NotifyMerchantAdmins(s.db, order.TenantID,
		"collect_reminder", "补缴超时催缴", content, orderID, "order",
		"collect_reminder", &adStr)

	// 订单日志留痕
	s.db.Create(&models.OrderLog{
		ID:        uuid.New().String(),
		OrderID:   orderID,
		Event:     "补缴超时催缴",
		CreatedAt: time.Now(),
	})

	// 标记已提醒（幂等）
	now := time.Now()
	if err := s.db.Model(&rec).Update("reminded_at", now).Error; err != nil {
		return fmt.Errorf("mark reminded: %w", err)
	}
	return nil
}
