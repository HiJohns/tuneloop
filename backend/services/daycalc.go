package services

import (
	"math"
	"time"
)

// CalculateEndDate returns the end date given start date and number of days.
// endDate = startDate + days - 1, meaning startDate 00:00 to endDate 23:59:59.
func CalculateEndDate(startDate time.Time, days int) time.Time {
	return startDate.AddDate(0, 0, days-1)
}

// CalculateDays returns the number of days between startDate and endDate.
// startDate 00:00 to endDate 23:59:59. Minimum 1 day.
func CalculateDays(startDate, endDate time.Time) int {
	endOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, endDate.Location())
	days := int(math.Ceil(endOfDay.Sub(startDate).Hours() / 24))
	if days < 1 {
		days = 1
	}
	return days
}

// CalculateLeaseDays is the single authoritative rule for ACTUAL LEASE DAYS
// used by every settlement money path (#1738 P3 口径统一):
// ceil(elapsed hours / 24), minimum 1 day (#1665: 当天收货当天归还不足
// 24h 计 1 天，不按自然日翻倍). Deliberately NOT calendar-day based —
// use CalculateDays for natural-day displays instead.
func CalculateLeaseDays(startDate, endDate time.Time) int {
	hours := endDate.Sub(startDate).Hours()
	days := int(math.Ceil(hours / 24))
	if days < 1 {
		days = 1
	}
	return days
}
