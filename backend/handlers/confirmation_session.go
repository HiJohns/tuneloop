package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConfirmationSessionHandler handles confirmation session lifecycle
type ConfirmationSessionHandler struct {
	// Can hold dependencies like notification service
}

// NewConfirmationSessionHandler creates a new handler
func NewConfirmationSessionHandler() *ConfirmationSessionHandler {
	return &ConfirmationSessionHandler{}
}

// CreateConfirmationSession POST /api/confirmation-sessions
func (h *ConfirmationSessionHandler) Create(c *gin.Context) {
	var req struct {
		UserID         string `json:"user_id" binding:"required"`
		ConfirmType    string `json:"confirm_type" binding:"required,oneof=email phone"`
		ConfirmTarget  string `json:"confirm_target" binding:"required"`
		MerchantID     string `json:"merchant_id"`
		ActionType     string `json:"action_type" binding:"required,oneof=merchant_admin site_manager site_staff"`
		ActionTargetID string `json:"action_target_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant not authenticated"})
		return
	}

	// Verify user exists
	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", req.UserID, tenantID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query user"})
		}
		return
	}

	// Generate unique token
	token, err := generateSecureToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to generate token"})
		return
	}

	// Calculate expiration (24 hours from now)
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create confirmation session
	session := models.ConfirmationSession{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          orgID,
		UserID:         req.UserID,
		ConfirmType:    req.ConfirmType,
		ConfirmTarget:  req.ConfirmTarget,
		MerchantID:     req.MerchantID,
		ActionType:     req.ActionType,
		ActionTargetID: req.ActionTargetID,
		Status:         "waiting",
		Token:          token,
		ExpiresAt:      expiresAt,
	}

	if err := db.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20100,
		"data": gin.H{
			"id":         session.ID,
			"user_id":    session.UserID,
			"status":     session.Status,
			"token":      session.Token,
			"expires_at": session.ExpiresAt,
		},
	})
}

// GetConfirmationSession GET /api/confirmation-sessions/:id
func (h *ConfirmationSessionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "session id is required"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)
	var session models.ConfirmationSession
	if err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "session not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query session"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":               session.ID,
			"user_id":          session.UserID,
			"confirm_type":     session.ConfirmType,
			"confirm_target":   session.ConfirmTarget,
			"merchant_id":      session.MerchantID,
			"action_type":      session.ActionType,
			"action_target_id": session.ActionTargetID,
			"status":           session.Status,
			"message":          session.Message,
			"expires_at":       session.ExpiresAt,
			"confirmed_at":     session.ConfirmedAt,
			"created_at":       session.CreatedAt,
		},
	})
}

// ConfirmConfirmationSession POST /api/confirmation-sessions/:id/confirm
func (h *ConfirmationSessionHandler) Confirm(c *gin.Context) {
	id := c.Param("id")
	token := c.Query("token")

	if id == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "session id and token are required"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)
	var session models.ConfirmationSession

	// Find session with token
	if err := db.Where("id = ? AND tenant_id = ? AND token = ?", id, tenantID, token).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "invalid session or token"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to verify session"})
		}
		return
	}

	// Check if already expired
	if time.Now().After(session.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "confirmation link has expired"})
		return
	}

	// Check if already confirmed
	if session.Status == "confirmed" {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "already confirmed"})
		return
	}

	// Update session status
	now := time.Now()
	session.Status = "confirmed"
	session.ConfirmedAt = &now

	if err := db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to confirm session"})
		return
	}

	// Execute the action
	if err := h.executeAction(&session); err != nil {
		// #2542: the session was already persisted as "confirmed" above; a failed
		// action must not leave that receipt behind.
		failSession(db, &session, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to execute action: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "successfully confirmed and action executed",
	})
}

// RejectConfirmationSession POST /api/confirmation-sessions/:id/reject
func (h *ConfirmationSessionHandler) Reject(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "token is required"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify session exists and matches token
	var session models.ConfirmationSession
	if err := db.Where("id = ? AND tenant_id = ? AND token = ?", id, tenantID, req.Token).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "invalid session or token"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to verify session"})
		}
		return
	}

	// Update status to rejected
	session.Status = "rejected"
	if err := db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reject session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "successfully rejected",
	})
}

// generateSecureToken generates a cryptographically secure random token of specified length
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IAMConfirmationCallback GET /api/iam/confirmation-callback
func (h *ConfirmationSessionHandler) IAMConfirmationCallback(c *gin.Context) {
	sessionID := c.Query("session")
	result := c.Query("result")
	confirmType := c.Query("confirm_type")

	if sessionID == "" || result == "" {
		c.Redirect(http.StatusFound, "/confirmation-result?status=error&message=missing_parameters")
		return
	}

	log.Printf("[IAMCallback] Received callback: session=%s, result=%s, confirm_type=%s", sessionID, result, confirmType)

	db := database.GetDB()

	var session models.ConfirmationSession
	if err := db.Where("iam_session_id = ? OR id = ?", sessionID, sessionID).First(&session).Error; err != nil {
		log.Printf("[IAMCallback] Session not found: %s", sessionID)
		c.Redirect(http.StatusFound, "/confirmation-result?status=error&message=session_not_found")
		return
	}

	now := time.Now()

	switch result {
	case "accept":
		session.Status = "confirmed"
		session.ConfirmedAt = &now
		session.Message = "Confirmed via IAM callback"

		if err := db.Save(&session).Error; err != nil {
			log.Printf("[IAMCallback] Failed to update session: %v", err)
			c.Redirect(http.StatusFound, "/confirmation-result?status=error&message=save_failed")
			return
		}

		if err := h.executeAction(&session); err != nil {
			log.Printf("[IAMCallback] Failed to execute action: %v", err)
			// #2542: receipt must say failed, not error-with-message — the
			// /confirmation-result page renders the status=failed branch.
			failSession(db, &session, err)
			c.Redirect(http.StatusFound, "/confirmation-result?status=failed&action="+session.ActionType)
			return
		}

		log.Printf("[IAMCallback] Successfully confirmed session %s", sessionID)
		c.Redirect(http.StatusFound, "/confirmation-result?status=success&action="+session.ActionType)

	case "reject":
		session.Status = "rejected"
		session.Message = "Rejected by user via IAM callback"
		db.Save(&session)

		log.Printf("[IAMCallback] Session %s rejected", sessionID)
		c.Redirect(http.StatusFound, "/confirmation-result?status=rejected&action="+session.ActionType)

	case "failed":
		session.Status = "failed"
		session.Message = "IAM confirmation failed"
		db.Save(&session)

		log.Printf("[IAMCallback] Session %s failed", sessionID)
		c.Redirect(http.StatusFound, "/confirmation-result?status=failed&action="+session.ActionType)

	default:
		log.Printf("[IAMCallback] Unknown result: %s", result)
		c.Redirect(http.StatusFound, "/confirmation-result?status=error&message=unknown_result")
	}
}

// failSession persists the terminal "failed" state after executeAction fails.
//
// #2542: Confirm / SMSCallback persist "confirmed" *before* running the action,
// so a failed IAM write used to leave a success receipt behind. Every caller
// keeps its own surface shape (JSON 500, redirect, SMS text) and only delegates
// the persisted outcome here — one implementation, no per-caller copies.
func failSession(db *gorm.DB, session *models.ConfirmationSession, actionErr error) {
	session.Status = "failed"
	session.Message = fmt.Sprintf("Action execution failed: %v", actionErr)
	if err := db.Save(session).Error; err != nil {
		// Compensation must not be silently swallowed: keep the original action
		// error in the log so operators can reconcile the orphaned session.
		log.Printf("[Confirm] CRITICAL: failed to persist failure state for session %s: %v (action error: %v)",
			session.ID, err, actionErr)
	}
}

// executeAction executes the action after confirmation
func (h *ConfirmationSessionHandler) executeAction(session *models.ConfirmationSession) error {
	db := database.GetDB()

	var user models.User
	if err := db.Where("id = ?", session.UserID).First(&user).Error; err != nil {
		return fmt.Errorf("user not found")
	}

	if user.Status == "pending" {
		user.Status = "active"
		if err := db.Save(&user).Error; err != nil {
			return fmt.Errorf("failed to update user status")
		}
	}

	switch session.ActionType {
	case "merchant_admin":
		if session.MerchantID != "" {
			var merchant models.Merchant
			if err := db.Where("id = ?", session.MerchantID).First(&merchant).Error; err != nil {
				return fmt.Errorf("merchant not found: %s", session.MerchantID)
			}
			if merchant.OrgID != "" {
				iamClient := services.NewIAMClient()
				// #2540 D4（Owner 2026-11-26 裁定）：新建商户的管理员是商户的 OWNER。
				// 旧写法字面量 "admin" 把创建者降为 ADMIN → 无法再用 POST /organizations
				// 创建网点（beaconiam 该路由要求 PermissionOwner）。唯一权威映射 = toIAMRole。
				if err := iamClient.BindUserToOrganization(session.UserID, merchant.OrgID, toIAMRole("merchant_admin"), ""); err != nil {
					return fmt.Errorf("IAM bind failed: %w", err)
				}
				if t, ok := services.AllRoleTemplates["merchant_admin"]; ok && len(t.CusPermCodes) > 0 {
					cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(t.CusPermCodes, middleware.PermissionRegistry.GetCusPermBit)
					if err := iamClient.SetUserCustomerPermissions(merchant.OrgID, session.UserID, cusPerm, cusPermExt); err != nil {
						// #2542: failing to write the customer-permission bitmap means the
						// admin has no usable permissions, so treat it exactly like a bind
						// failure — roll the binding back (no purge: this flow did not create
						// the principal) and propagate the error. Logging and continuing
						// would report success for a half-provisioned admin (AGENTS.md red
						// line: no silent error swallowing in API call chains).
						if unbindErr := iamClient.UnbindUserFromOrganization(session.UserID, merchant.OrgID, ""); unbindErr != nil {
							log.Printf("[Confirm] CRITICAL: rollback unbind failed user=%s org=%s: %v (action error: %v)",
								session.UserID, merchant.OrgID, unbindErr, err)
							return fmt.Errorf("IAM set customer permissions failed: %w (rollback unbind also failed: %v)", err, unbindErr)
						}
						return fmt.Errorf("IAM set customer permissions failed: %w", err)
					}
				}
			}
		}
		return nil

	case "site_manager", "site_staff":
		role := "Manager"
		if session.ActionType == "site_staff" {
			role = "Staff"
		}

		siteMember := models.SiteMember{
			SiteID:    session.ActionTargetID,
			UserID:    session.UserID,
			Role:      role,
			Roles:     []string{role}, // #2034 多重角色（集）
			TenantID:  session.TenantID,
			CreatedAt: time.Now(),
		}

		result := db.Where("site_id = ? AND user_id = ?", session.ActionTargetID, session.UserID).FirstOrCreate(&siteMember)
		if result.Error != nil {
			return fmt.Errorf("failed to create site member: %w", result.Error)
		}
		return nil

	default:
		return fmt.Errorf("unsupported action type: %s", session.ActionType)
	}
}

// Callback for SMS confirmations
func (h *ConfirmationSessionHandler) SMSCallback(c *gin.Context) {
	token := c.Query("token")
	reply := c.Query("reply")

	if token == "" {
		c.String(http.StatusBadRequest, "token is required")
		return
	}

	// If reply is 'Y' or 'y', confirm the session
	if reply == "Y" || reply == "y" {
		db := database.GetDB()
		var session models.ConfirmationSession

		// Find session by token
		if err := db.Where("token = ? AND confirm_type = ? AND status = ?", token, "phone", "waiting").First(&session).Error; err != nil {
			c.String(http.StatusNotFound, "invalid token or session not found")
			return
		}

		// Mark as confirmed
		now := time.Now()
		session.Status = "confirmed"
		session.ConfirmedAt = &now

		if err := db.Save(&session).Error; err != nil {
			c.String(http.StatusInternalServerError, "failed to confirm session")
			return
		}

		// Execute action
		if err := h.executeAction(&session); err != nil {
			failSession(db, &session, err)
			c.String(http.StatusInternalServerError, "failed to execute action: "+err.Error())
			return
		}

		c.String(http.StatusOK, "Thank you! You have been successfully added to the organization.")
		return
	}

	c.String(http.StatusOK, "Invalid reply. Please reply with Y to confirm.")
}

func (h *ConfirmationSessionHandler) GetConfig() map[string]string {
	return map[string]string{
		"smtp_host":   services.GetSMTPHost(),
		"sms_gateway": services.GetSMSGateway(),
	}
}
