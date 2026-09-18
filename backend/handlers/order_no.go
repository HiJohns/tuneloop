package handlers

import (
	"fmt"
	"strings"
	"time"

	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// #1965 业务订单号：`YL<YYYYMMDD>-<NNN>`（当日第 N 单，北京日历日）。
// 并发安全：唯一索引兜底，冲突时递增序号重试。

func generateOrderNo(db *gorm.DB, createdAt time.Time) string {
	d := createdAt.In(time.Local).Format("20060102")
	var n int64
	db.Model(&models.Order{}).Where("order_no LIKE ?", models.OrderNoPrefix+d+"-%").Count(&n)
	return fmt.Sprintf("%s%s-%03d", models.OrderNoPrefix, d, n+1)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "duplicate key")
}

// createOrderWithNo 生成 order_no 并创建订单；唯一冲突（并发同号）自动重试。
func createOrderWithNo(db *gorm.DB, order *models.Order) error {
	createdAt := order.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
		order.CreatedAt = createdAt
	}
	for i := 0; i < 5; i++ {
		order.OrderNo = generateOrderNo(db, createdAt)
		err := db.Create(order).Error
		if err == nil {
			return nil
		}
		if isUniqueViolation(err) {
			continue
		}
		return err
	}
	return fmt.Errorf("failed to allocate unique order_no after retries")
}
