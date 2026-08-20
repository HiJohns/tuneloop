package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Cents 表示货币金额，单位：分（1 元 = 100 分）。#1725 金额整型化：
// 内部一律整数运算，杜绝浮点精度隐患（0.06...05 之类）。
//
// P1 过渡期（#1726）：DB 列仍为 DECIMAL(10,2)（元），Scan/Value 负责
// 元 ↔ 分 转换；API JSON 输出元（兼容旧前端）。P3（#1728）DB 改 BIGINT
// 分、API 输出分后，Scan/Value 与 MarshalJSON 随之切换。
type Cents int64

// ToYuan 返回元（float64，仅用于展示层/边界转换，计算勿用）。
func (c Cents) ToYuan() float64 {
	return float64(c) / 100
}

// FromYuan 把元（float64）转换为分（四舍五入到分）。
func FromYuan(yuan float64) Cents {
	return Cents(math.Round(yuan*100 + 0.5) - 0.5)
}

// MarshalJSON：P1 输出元（12.34）——API 契约暂不变，前端无需改动。
func (c Cents) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.ToYuan())
}

// UnmarshalJSON：接受元（数字，如 12.34 或 12）→ ×100；或分（整数）。
// P1 期间前端/调用方提交元；分输入兼容 JSONB 内部整数金额。
func (c *Cents) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*c = 0
		return nil
	}
	if strings.Contains(s, ".") || strings.ContainsAny(s, "eE") {
		var yuan float64
		if err := json.Unmarshal(b, &yuan); err != nil {
			return fmt.Errorf("cents: invalid yuan amount %s: %w", s, err)
		}
		*c = FromYuan(yuan)
		return nil
	}
	var cents int64
	if err := json.Unmarshal(b, &cents); err != nil {
		return fmt.Errorf("cents: invalid cents amount %s: %w", s, err)
	}
	*c = Cents(cents)
	return nil
}

// Scan：DB → Cents。P1 期间 DB 为 DECIMAL(10,2)（元），×100 转分。
// 兼容 P2 后 BIGINT（分）直读。
func (c *Cents) Scan(v interface{}) error {
	if v == nil {
		*c = 0
		return nil
	}
	switch val := v.(type) {
	case int64:
		*c = Cents(val)
		return nil
	case float64:
		*c = FromYuan(val)
		return nil
	case []byte:
		s := string(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			*c = FromYuan(f)
			return nil
		}
		return fmt.Errorf("cents: cannot scan %q", s)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			*c = FromYuan(f)
			return nil
		}
		return fmt.Errorf("cents: cannot scan %q", val)
	default:
		return fmt.Errorf("cents: unsupported scan type %T", v)
	}
}

// Value：Cents → DB。P1 期间 DB 为 DECIMAL(10,2)，写元（float64）。
func (c Cents) Value() (driver.Value, error) {
	return c.ToYuan(), nil
}

// ToCentsPtr 把 *float64（元）转为 *Cents（分），nil 透传。
func ToCentsPtr(yuan *float64) *Cents {
	if yuan == nil {
		return nil
	}
	c := FromYuan(*yuan)
	return &c
}

// ToYuanPtr 把 *Cents（分）转为 *float64（元），nil 透传。
func ToYuanPtr(cents *Cents) *float64 {
	if cents == nil {
		return nil
	}
	y := cents.ToYuan()
	return &y
}

// Mul 整数乘法（Cents × 无单位量，如天数）。
func (c Cents) Mul(n int64) Cents {
	return c * Cents(n)
}

// Percent 计算百分比：c × pct / 100（整数运算，先乘后除防精度损失）。
func (c Cents) Percent(pct int64) Cents {
	return c * Cents(pct) / 100
}
