// CalculateEndDate returns the end date given start date and number of days.
// endDate = startDate + days - 1, meaning startDate 00:00 to endDate 23:59:59.
export function calculateEndDate(startDate, days) {
  const d = new Date(startDate)
  d.setDate(d.getDate() + days - 1)
  return d
}

// CalculateDays returns the number of days between startDate and endDate.
// startDate 00:00 to endDate 23:59:59. Minimum 1 day.
export function calculateDays(startDate, endDate) {
  const end = new Date(endDate)
  end.setHours(23, 59, 59, 0)
  const start = new Date(startDate)
  return Math.max(1, Math.ceil((end.getTime() - start.getTime()) / 86400000))
}
