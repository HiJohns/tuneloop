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
