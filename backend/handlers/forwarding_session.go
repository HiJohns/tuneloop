package handlers

import (
	"crypto/rand"
	"log"
	"math/big"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func generateSessionCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	charlen := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charlen)
		if err != nil {
			log.Printf("[generateSessionCode] rand failed: %v", err)
			b[i] = '0'
			continue
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func createForwardingSession(ctx *gin.Context, tx *gorm.DB, tenantID, orgID, leaseSessionID, orderID, instrumentID string, direction string) {
	merchantID := middleware.GetOrgID(ctx.Request.Context())
	// Retry up to 3 times on session_code collision
	var code string
	for attempt := 0; attempt < 3; attempt++ {
		code = generateSessionCode()
		var count int64
		tx.Model(&models.ForwardingSession{}).Where("session_code = ?", code).Count(&count)
		if count == 0 {
			break
		}
	}
	session := models.ForwardingSession{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          orgID,
		LeaseSessionID: leaseSessionID,
		OrderID:        orderID,
		MerchantID:     merchantID,
		Direction:      direction,
		Status:         models.ForwardingStatusPending,
		SessionCode:    code,
		InstrumentID:   instrumentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := tx.Create(&session).Error; err != nil {
		log.Printf("[createForwardingSession] Failed to create: %v", err)
	}
}

// GET /api/forwarding/sessions - List forwarding sessions
func ListForwardingSessions(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var sessions []models.ForwardingSession
	query := db.Where("tenant_id = ?", tenantID)
	if code := c.Query("session_code"); code != "" {
		query = query.Where("session_code = ?", code)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("created_at DESC").Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query forwarding sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    gin.H{"list": sessions},
	})
}

// PUT /api/forwarding/sessions/:id/ship - Ship from controlled merchant to forwarding site
func ShipForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be pending to ship"})
		return
	}
	if err := db.Model(&session).Update("status", models.ForwardingStatusInTransit).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "shipped"})
}

// PUT /api/forwarding/sessions/:id/receive - Receive goods at forwarding site
func ReceiveForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusInTransit && session.Status != models.ForwardingStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be pending or in_transit to receive"})
		return
	}
	if err := db.Model(&session).Update("status", models.ForwardingStatusReceived).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "received"})
}

// PUT /api/forwarding/sessions/:id/ready - Repack complete, ready for last mile
func ReadyForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusReceived {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be received to mark ready"})
		return
	}
	if err := db.Model(&session).Update("status", models.ForwardingStatusReady).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "ready"})
}

// PUT /api/forwarding/sessions/:id/last-mile - Forward last mile
func LastMileForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusReady {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be ready for last mile"})
		return
	}
	if err := db.Model(&session).Update("status", models.ForwardingStatusLastMile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "last mile dispatched"})
}

// PUT /api/forwarding/sessions/:id/complete - Complete forwarding
func CompleteForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusLastMile && session.Status != models.ForwardingStatusDelivered {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be in last_mile or delivered to complete"})
		return
	}
	if err := db.Model(&session).Updates(map[string]interface{}{
		"status":     models.ForwardingStatusCompleted,
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to complete session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "completed"})
}

// PUT /api/forwarding/sessions/:id/lost - Mark as lost
func LostForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if err := db.Model(&session).Update("status", models.ForwardingStatusLost).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to mark as lost"})
		return
	}
	// Update instrument stock_status to lost
	if session.InstrumentID != "" {
		db.Table("instruments").Where("id = ?", session.InstrumentID).Update("stock_status", models.StockStatusLost)
	}
	// For outbound direction, cancel the order and refund
	if session.Direction == models.ForwardingDirectionOutbound && session.OrderID != "" {
		db.Model(&models.Order{}).Where("id = ?", session.OrderID).Updates(map[string]interface{}{
			"status": models.OrderStatusInStore,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "marked as lost"})
}

// PUT /api/forwarding/sessions/:id/recover - Recover from lost to previous status
func RecoverForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusLost {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be lost to recover"})
		return
	}
	// Recover to in_transit (forwarding site will re-process)
	if err := db.Model(&session).Update("status", models.ForwardingStatusInTransit).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to recover session"})
		return
	}
	// Restore instrument stock status
	if session.InstrumentID != "" {
		db.Table("instruments").Where("id = ?", session.InstrumentID).Update("stock_status", models.StockStatusAvailable)
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "recovered"})
}
