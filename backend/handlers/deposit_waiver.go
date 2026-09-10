package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deposit-free eligibility rules (#1867):
//   - verified real-name identity (face_verified)
//   - third certificate type is student or teacher (#1807 field)
//   - internal credit score >= DEPOSIT_WAIVER_MIN_CREDIT (default 600)
//   - a recommendation letter must be attached to the order
//
// External credit-score providers are out of scope (docs/features.md keeps
// the credit model as simulated/internal until an integration is agreed).

const defaultDepositWaiverMinCredit = 600

// depositWaiverMinCredit reads the configurable credit-score threshold.
func depositWaiverMinCredit() int {
	if v := os.Getenv("DEPOSIT_WAIVER_MIN_CREDIT"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return defaultDepositWaiverMinCredit
}

// DepositWaiverEligibility describes whether a user may apply for a
// deposit-free order, with machine-readable reasons for the frontend.
type DepositWaiverEligibility struct {
	Eligible       bool     `json:"eligible"`
	Reasons        []string `json:"reasons"`
	IdentityType   *string  `json:"identity_type"`
	FaceVerified   bool     `json:"face_verified"`
	CreditScore    int      `json:"credit_score"`
	MinCreditScore int      `json:"min_credit_score"`
}

// EvaluateDepositWaiverEligibility loads the user row and evaluates the
// #1867 rules. Returns the eligibility descriptor; the user row must exist
// (callers ensure the local user beforehand).
func EvaluateDepositWaiverEligibility(db *gorm.DB, userID string) DepositWaiverEligibility {
	minCredit := depositWaiverMinCredit()
	result := DepositWaiverEligibility{
		Eligible:       true,
		Reasons:        []string{},
		MinCreditScore: minCredit,
	}

	var user models.User
	if err := db.Select("face_verified, id_photo_other_type, credit_score").
		First(&user, "id = ?", userID).Error; err != nil {
		result.Eligible = false
		result.Reasons = append(result.Reasons, "user_not_found")
		return result
	}

	result.FaceVerified = user.FaceVerified
	result.IdentityType = user.IdPhotoOtherType
	result.CreditScore = user.CreditScore

	if !user.FaceVerified {
		result.Eligible = false
		result.Reasons = append(result.Reasons, "face_not_verified")
	}
	if user.IdPhotoOtherType == nil ||
		(*user.IdPhotoOtherType != "student" && *user.IdPhotoOtherType != "teacher") {
		result.Eligible = false
		result.Reasons = append(result.Reasons, "identity_not_student_or_teacher")
	}
	if user.CreditScore < minCredit {
		result.Eligible = false
		result.Reasons = append(result.Reasons, "credit_below_threshold")
	}
	return result
}

// checkDepositWaiverEligibility is the order-creation gate: returns the
// business code 40301 + message when the user does not qualify (#1867).
// Callers respond with HTTP 403 and body code 40301.
func checkDepositWaiverEligibility(db *gorm.DB, userID string) (int, string) {
	e := EvaluateDepositWaiverEligibility(db, userID)
	if e.Eligible {
		return 0, ""
	}
	if len(e.Reasons) == 1 && e.Reasons[0] == "user_not_found" {
		return 40301, "deposit-free verification failed: user record not found"
	}
	return 40301, fmt.Sprintf(
		"deposit-free rental requires a verified student/faculty identity and credit score >= %d",
		e.MinCreditScore)
}

// GetDepositWaiverEligibility exposes the eligibility state to the frontend
// so the checkout can gate the deposit-free toggle with concrete reasons.
// The JWT carries the IAM sub — resolve the local user row without writes.
func GetDepositWaiverEligibility(c *gin.Context) {
	ctx := c.Request.Context()
	iamSub := middleware.GetUserID(ctx)
	if iamSub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40001, "message": "login required"})
		return
	}
	db := database.GetDB().WithContext(ctx)

	var user models.User
	if err := db.Where("iam_sub = ?", iamSub).First(&user).Error; err != nil {
		// No local profile yet — the onboarding flow must complete first.
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": DepositWaiverEligibility{
			Eligible:       false,
			Reasons:        []string{"profile_not_ready"},
			MinCreditScore: depositWaiverMinCredit(),
		}})
		return
	}
	result := EvaluateDepositWaiverEligibility(db, user.ID)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}
