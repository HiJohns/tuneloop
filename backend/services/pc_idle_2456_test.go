package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #2456 R7：PC_IDLE_TIMEOUT_MINUTES 解析规则（默认 60 / 0 禁用 / 非法回落 / 上限截断）。
func TestPCSessionIdleTimeoutConfig(t *testing.T) {
	invalid := func(t *testing.T) {
		t.Helper()
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "abc")
		require.Equal(t, DefaultPCIdleTimeoutMinutes, PCSessionIdleTimeoutMinutes())
		t.Logf("INVALID_FALLBACK=%d", PCSessionIdleTimeoutMinutes())
	}

	t.Run("default_when_unset", func(t *testing.T) {
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "")
		require.Equal(t, DefaultPCIdleTimeoutMinutes, PCSessionIdleTimeoutMinutes())
		require.Equal(t, 60*time.Minute, PCSessionIdleTimeout())
	})

	t.Run("one_minute", func(t *testing.T) {
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "1")
		require.Equal(t, 1, PCSessionIdleTimeoutMinutes())
		require.Equal(t, time.Minute, PCSessionIdleTimeout())
	})

	t.Run("zero_disables", func(t *testing.T) {
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "0")
		require.Equal(t, 0, PCSessionIdleTimeoutMinutes())
		require.Equal(t, time.Duration(0), PCSessionIdleTimeout())
		t.Logf("ZERO_DISABLED=%d", 1)
	})

	t.Run("invalid_falls_back", invalid)

	t.Run("negative_falls_back", func(t *testing.T) {
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "-5")
		require.Equal(t, DefaultPCIdleTimeoutMinutes, PCSessionIdleTimeoutMinutes())
	})

	t.Run("clamped_to_max", func(t *testing.T) {
		t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "99999")
		require.Equal(t, MaxPCIdleTimeoutMinutes, PCSessionIdleTimeoutMinutes())
		t.Logf("CLAMP_MAX=%d", PCSessionIdleTimeoutMinutes())
	})
}

// #2456 R5/R3：会话键必须稳定且对令牌敏感（refresh_hash = sha256(refresh_token)）。
func TestHashRefreshTokenStable(t *testing.T) {
	first := HashRefreshToken("rt-sample")
	second := HashRefreshToken("rt-sample")
	other := HashRefreshToken("rt-other")

	require.Equal(t, first, second)
	require.NotEqual(t, first, other)
	require.Len(t, first, 64)
}
