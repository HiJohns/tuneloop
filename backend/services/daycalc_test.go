package services

import (
	"testing"
	"time"
)

// #1738 P3: CalculateLeaseDays is the single lease-day rule for settlement
// money paths. Three canonical cases: same-day short rental, cross-midnight
// under 24h, and multi-day.
func TestCalculateLeaseDays(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	cases := []struct {
		name string
		start time.Time
		end   time.Time
		want  int
	}{
		{
			// 当日收货当日归还（#1665 口径）：不足 24h 计 1 天
			name:  "same day under 24h",
			start: time.Date(2026, 8, 21, 10, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 21, 18, 0, 0, 0, loc),
			want:  1,
		},
		{
			// 跨天但不足 24h：仍按实际时长 ceil = 1 天
			name:  "cross midnight under 24h",
			start: time.Date(2026, 8, 20, 20, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 21, 8, 0, 0, 0, loc),
			want:  1,
		},
		{
			// 跨天超过 24h（af3f8cf2 实测场景 29.4h）→ 2 天
			name:  "cross midnight over 24h",
			start: time.Date(2026, 8, 21, 3, 29, 0, 0, loc),
			end:   time.Date(2026, 8, 22, 8, 54, 0, 0, loc),
			want:  2,
		},
		{
			// 整租期边界：恰好 48h → 2 天
			name:  "exactly 48 hours",
			start: time.Date(2026, 8, 20, 12, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 22, 12, 0, 0, 0, loc),
			want:  2,
		},
		{
			// 防御：end ≤ start 时最小 1 天
			name:  "end before start clamps to 1",
			start: time.Date(2026, 8, 22, 12, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 21, 12, 0, 0, 0, loc),
			want:  1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CalculateLeaseDays(tc.start, tc.end); got != tc.want {
				t.Fatalf("CalculateLeaseDays(%v, %v) = %d, want %d", tc.start, tc.end, got, tc.want)
			}
		})
	}
}
