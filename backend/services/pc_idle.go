package services

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
)

// #2456: PC 管理后台静置超时（idle timeout）策略参数。
//
// 会话事实源是 tuneloop 本地 pc_sessions 表（Decision Lock D1）：只有 PC 入口
// （OAuth 回调 / 账密登录）写入，H5 与小程序不写。因此本文件只提供"阈值解析"
// 与"会话键派生"，不含任何端判定，避免把 idle 策略误套到顾客/小程序流（D5）。
const (
	// DefaultPCIdleTimeoutMinutes 是静置超时的默认阈值（分钟）。
	DefaultPCIdleTimeoutMinutes = 60
	// MaxPCIdleTimeoutMinutes 是配置上限；超过按上限截断（避免误配成无限期）。
	MaxPCIdleTimeoutMinutes = 1440
	// PCActivityTouchThreshold 是服务端活动写入的最小间隔（节流，D3）。
	PCActivityTouchThreshold = 60 * time.Second
)

// PCSessionIdleTimeoutMinutes 解析环境变量 PC_IDLE_TIMEOUT_MINUTES：
//   - 未设置 → 默认 60
//   - 0        → 禁用（不校验，返回 0）
//   - 负数/非法 → 回落默认 60
//   - > 1440   → 截断到 1440
func PCSessionIdleTimeoutMinutes() int {
	raw := strings.TrimSpace(os.Getenv("PC_IDLE_TIMEOUT_MINUTES"))
	if raw == "" {
		return DefaultPCIdleTimeoutMinutes
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes < 0 {
		return DefaultPCIdleTimeoutMinutes
	}
	if minutes > MaxPCIdleTimeoutMinutes {
		return MaxPCIdleTimeoutMinutes
	}
	return minutes
}

// PCSessionIdleTimeout 返回静置超时时长；0 表示禁用（调用方必须跳过校验）。
func PCSessionIdleTimeout() time.Duration {
	minutes := PCSessionIdleTimeoutMinutes()
	if minutes <= 0 {
		return 0
	}
	return time.Duration(minutes) * time.Minute
}

// HashRefreshToken 返回 refresh token 的 sha256 十六进制摘要，用作 pc_sessions
// 的稳定会话键（IAM 刷新不轮换 refresh_token，见 #2456 Input Truth）。
func HashRefreshToken(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(sum[:])
}
