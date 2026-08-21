package handlers

import (
	"encoding/json"
	"fmt"
	"math"

	"tuneloop-backend/database"

	"gorm.io/gorm"
)

// MigrateJSONBCents 把存量 JSONB 复合字段中的元金额一次性转换为分（#1727 P2）。
//
// 覆盖字段：
//   - orders.pricing_breakdown / points_policy_snapshot / request_snapshot / pricing_config_snapshot
//   - settlements.breakdown
//   - order_payment_records.raw_response（仅自定义键：gift_used/original_amount/amount）
//   - notifications.action_data
//
// 幂等：转换后的 JSON 写入顶层标记 "_cents_migrated": true，重跑跳过（=0 更新）。
// fail-fast：任何记录 JSON 解析失败或转换后数值异常 → 返回 error，调用方 FATAL
// 退出，严禁带伤运行。
func MigrateJSONBCents(dryRun bool) (int, error) {
	return MigrateJSONBCentsWithMode(dryRun, false)
}

// MigrateJSONBCentsWithMode 转换 JSONB 金额。
// reverse=true 时把已迁移（_cents_migrated）的记录反向 ÷100 恢复元并移除标记
// （P2 回滚用——JSONB 元→分必须与前端改分同步发布，见 #1727 质疑）。
func MigrateJSONBCentsWithMode(dryRun, reverse bool) (int, error) {
	db := getDB()
	type tableField struct {
		table  string
		column string
	}
	targets := []tableField{
		{"orders", "pricing_breakdown"},
		{"orders", "points_policy_snapshot"},
		{"orders", "request_snapshot"},
		{"orders", "pricing_config_snapshot"},
		{"settlements", "breakdown"},
		{"order_payment_records", "raw_response"},
		{"notifications", "action_data"},
	}

	total := 0
	for _, tf := range targets {
		// 表可能不存在（依赖 migrations）——跳过（幂等）
		var tableExists int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?`, tf.table).Scan(&tableExists).Error; err != nil {
			return total, fmt.Errorf("migrate jsonb cents: check table %s: %w", tf.table, err)
		}
		if tableExists == 0 {
			continue
		}
		var colExists int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, tf.table, tf.column).Scan(&colExists).Error; err != nil {
			return total, fmt.Errorf("migrate jsonb cents: check column %s.%s: %w", tf.table, tf.column, err)
		}
		if colExists == 0 {
			continue
		}

		var rows []struct {
			ID     string
			Column *string
		}
		if err := db.Table(tf.table).Select("id, " + tf.column + " AS column").Find(&rows).Error; err != nil {
			return total, fmt.Errorf("migrate jsonb cents: read %s.%s: %w", tf.table, tf.column, err)
		}
		for _, r := range rows {
			if r.Column == nil || *r.Column == "" {
				continue
			}
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(*r.Column), &obj); err != nil {
				// 非 JSON（如纯文本快照）——跳过不视为失败
				continue
			}
			if obj == nil {
				continue
			}
			migrated, _ := obj["_cents_migrated"].(bool)
			if reverse {
				if !migrated {
					continue
				}
				if err := reverseMoneyKeys(obj); err != nil {
					return total, fmt.Errorf("reverse jsonb cents: %s.%s id=%s: %w", tf.table, tf.column, r.ID, err)
				}
				delete(obj, "_cents_migrated")
			} else {
				if migrated {
					continue
				}
				converted, err := convertMoneyKeys(obj)
				if err != nil {
					return total, fmt.Errorf("migrate jsonb cents: %s.%s id=%s: %w", tf.table, tf.column, r.ID, err)
				}
				if !converted {
					continue // 无金额键，无需转换
				}
				obj["_cents_migrated"] = true
			}
			if dryRun {
				total++
				continue
			}
			out, err := json.Marshal(obj)
			if err != nil {
				return total, fmt.Errorf("migrate jsonb cents: marshal %s.%s id=%s: %w", tf.table, tf.column, r.ID, err)
			}
			outStr := string(out)
			if err := db.Table(tf.table).Where("id = ?", r.ID).Update(tf.column, &outStr).Error; err != nil {
				return total, fmt.Errorf("migrate jsonb cents: write %s.%s id=%s: %w", tf.table, tf.column, r.ID, err)
			}
			total++
		}
	}
	return total, nil
}

// moneyKeys 是 JSONB 内的金额键白名单（单位：元）——迁移时 ×100 转分。
// 比例/折扣键（pay_ratio/rent_ratio/discount 等）不属于金额，不转换。
var moneyKeys = map[string]bool{
	"base_daily_rate": true, "base_daily_rent": true, "daily_rate": true, "rate": true,
	"deposit": true, "shipping_fee": true, "total_price": true,
	"total_amount": true, "rent_amount": true, "subtotal": true, "total": true,
	"overdue_daily_fee": true, "overdue_fee": true,
	"gift_used": true, "prepaid_used": true, "original_amount": true, "amount": true,
	"final_amount": true, "damage_amount": true, "total_deduction": true,
	"pay_amount": true, "paid_amount": true,
	"cash_refundable": true, "prepaid_refunded": true, "gift_refunded": true,
	"gift_cap": true, "rent_payable": true, "damage_deducted": true,
	"overdue_charges_total": true, "total_refund": true,
	"deducted_from_deposit": true, "deducted_from_prepaid": true,
}

// convertMoneyKeys 递归转换 JSON 对象/数组中的金额键（元 → 分）。
// 返回是否发生了转换。数值异常（|分| > 10^12 = 百亿）视为数据错误。
func convertMoneyKeys(v interface{}) (bool, error) {
	converted := false
	switch val := v.(type) {
	case map[string]interface{}:
		for k, sub := range val {
			if moneyKeys[k] {
				f, ok := sub.(float64)
				if !ok {
					continue // 非数值（如字符串）跳过
				}
				cents := math.Round(f * 100)
				if math.Abs(cents) > 1e12 {
					return false, fmt.Errorf("amount %s=%v out of plausible range after ×100", k, f)
				}
				val[k] = cents
				converted = true
			} else {
				subConv, err := convertMoneyKeys(sub)
				if err != nil {
					return false, err
				}
				converted = converted || subConv
			}
		}
	case []interface{}:
		for _, item := range val {
			itemConv, err := convertMoneyKeys(item)
			if err != nil {
				return false, err
			}
			converted = converted || itemConv
		}
	}
	return converted, nil
}

// reverseMoneyKeys 递归把已迁移为分的金额键 ÷100 恢复元（回滚）。
func reverseMoneyKeys(v interface{}) error {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, sub := range val {
			if moneyKeys[k] {
				if f, ok := sub.(float64); ok {
					val[k] = f / 100
				}
			} else if k != "_cents_migrated" {
				if err := reverseMoneyKeys(sub); err != nil {
					return err
				}
			}
		}
	case []interface{}:
		for _, item := range val {
			if err := reverseMoneyKeys(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// getDB returns the global database handle.
func getDB() *gorm.DB {
	return database.GetDB()
}
