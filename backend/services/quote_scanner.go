package services

import (
	"log"
	"regexp"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

var (
	phonePattern  = regexp.MustCompile(`1[3-9]\d{9}`)
	emailPattern  = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	wechatPattern = regexp.MustCompile(`wxid_[a-zA-Z0-9]+|微信号?\s*[:：]?\s*[a-zA-Z][a-zA-Z0-9_-]{5,}`)
)

// ScanResult holds the scan outcome.
type ScanResult struct {
	HasSensitive bool
	Matched      []string
}

// ScanQuoteComment checks a comment for phone, email, and WeChat patterns.
func ScanQuoteComment(comment string) ScanResult {
	var matched []string

	if phonePattern.MatchString(comment) {
		matched = append(matched, "phone")
	}
	if emailPattern.MatchString(comment) {
		matched = append(matched, "email")
	}
	if wechatPattern.MatchString(comment) {
		matched = append(matched, "wechat")
	}

	return ScanResult{
		HasSensitive: len(matched) > 0,
		Matched:      matched,
	}
}

// HandleSensitiveQuote creates a warning when a quote comment contains sensitive info.
// Returns true if the quote should be closed.
func HandleSensitiveQuote(repairRequestID, workerID, comment string) bool {
	result := ScanQuoteComment(comment)
	if !result.HasSensitive {
		return false
	}

	db := database.GetDB()

	// Create warning
	w := models.Warning{
		ID:          uuid.New().String(),
		Reason:      "sensitive_content_in_quote",
		Category:    "quote",
		Level:       models.WarningSeverityMedium,
		ObjectType:  "repair_request",
		ObjectID:    repairRequestID,
		Description: "报价评论含敏感信息: " + comment[:min(100, len(comment))],
		Status:      models.WarningStatusOpen,
		CreatedAt:   time.Now(),
	}
	if err := db.Create(&w).Error; err != nil {
		log.Printf("[QuoteScanner] Failed to create warning: %v", err)
	}

	log.Printf("[QuoteScanner] Sensitive content detected in quote %s: %v", repairRequestID, result.Matched)

	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
