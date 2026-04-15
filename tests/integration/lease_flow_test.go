package integration

import (
	"testing"
	"tuneloop-backend/models"
)

// TestLeaseFlow 测试租赁完整流程
func TestLeaseFlow(t *testing.T) {
	// 1. 创建乐器（库存 = 1）
	instrument := models.Instrument{
		Name:       "测试钢琴",
		CategoryID: "test-category-id",
		Level:      "entry",
		Status:     "active",
	}
	
	// 2. 订单创建后库存扣减
	// 预期: inventory_available = 0
	
	// 3. 确认归还后库存恢复
	// 预期: inventory_available = 1
}

// TestOrderStatusFlow 测试订单状态流转
func TestOrderStatusFlow(t *testing.T) {
	// 测试状态: pending -> paid -> in_lease -> completed
}
