package handlers

import (
	"crypto/rand"
	"encoding/json"
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

// #1934: 中转物流费分段编号（docs/cases/transit.md 承担矩阵）
const (
	TransitSegmentOutboundControlledToTransit = 1 // ①受控→中转（顾客承担）
	TransitSegmentOutboundTransitToCustomer   = 2 // ②中转→顾客（顾客承担）
	TransitSegmentReturnCustomerToTransit     = 3 // ③顾客→中转（顾客自付）
	TransitSegmentReturnTransitToControlled   = 4 // ④中转→受控（商户承担）
)

// recordTransitFee 幂等写入分段费（同 order 同 segment 只记一行，重复调用更新金额）
func recordTransitFee(db *gorm.DB, session models.ForwardingSession, segment int, yuan float64, recordedBy string) {
	if yuan <= 0 && segment != 0 {
		// 运费可缺省（面单随单不收费），仍记录占位行以保承载矩阵完整性
		yuan = 0
	}
	dir := session.Direction
	paidBy := "customer"
	if segment == TransitSegmentReturnTransitToControlled {
		paidBy = "merchant"
	}
	var fee models.TransitShippingFee
	err := db.Where("order_id = ? AND direction = ? AND segment = ?", session.OrderID, dir, segment).First(&fee).Error
	if err == nil {
		if err := db.Model(&models.TransitShippingFee{}).Where("id = ?", fee.ID).
			Updates(map[string]interface{}{
				"amount":      models.FromYuan(yuan),
				"recorded_by": recordedBy,
			}).Error; err != nil {
			log.Printf("[recordTransitFee] update existing segment %d failed: %v", segment, err)
		}
		return
	}
	db.Create(&models.TransitShippingFee{
		OrderID:    session.OrderID,
		Direction:  dir,
		Segment:    segment,
		Amount:     models.FromYuan(yuan),
		PaidBy:     paidBy,
		RecordedBy: recordedBy,
	})
}

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
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		OrgID:           orgID,
		LeaseSessionID:  leaseSessionID,
		OrderID:         orderID,
		MerchantID:      merchantID,
		Direction:       direction,
		Status:          models.ForwardingStatusPending,
		SessionCode:     code,
		TrackingNumbers: "[]", // #1934: jsonb 列禁止空串（22P02）
		InstrumentID:    instrumentID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
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
	// #1934: 中转工作台会话查找（按订单）
	if orderID := c.Query("order_id"); orderID != "" {
		query = query.Where("order_id = ?", orderID)
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

	// #1934: 受控发货回填段级物流（物流公司/单号/运费元）
	var req struct {
		TrackingCompany string  `json:"tracking_company"`
		TrackingNumber  string  `json:"tracking_number"`
		ShippingFee     float64 `json:"shipping_fee"` // 元
	}
	_ = c.ShouldBindJSON(&req)

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
	// #1934: outbound 受控→中转段发运即记录分段①（顾客承担）——金额用员工填写的运费
	if req.ShippingFee > 0 || req.TrackingNumber != "" {
		recordTransitFee(db, session, TransitSegmentOutboundControlledToTransit, req.ShippingFee, middleware.GetUserID(ctx))
		if req.TrackingCompany != "" {
			db.Model(&session).Updates(map[string]interface{}{
				"tracking_company": req.TrackingCompany,
				"tracking_number":  req.TrackingNumber,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "shipped"})
}

// PUT /api/forwarding/sessions/:id/receive - Receive goods at forwarding site
func ReceiveForwardingSession(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	// #1934: 中转收货拍照留痕（可选）
	var req struct {
		PhotoKeys []string `json:"photo_keys"`
	}
	_ = c.ShouldBindJSON(&req)

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusInTransit && session.Status != models.ForwardingStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be pending or in_transit to receive"})
		return
	}
	if len(req.PhotoKeys) > 0 {
		photoJSON, _ := json.Marshal(req.PhotoKeys)
		if err := db.Model(&session).Updates(map[string]interface{}{
			"status": models.ForwardingStatusReceived,
			"photos": string(photoJSON),
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
			return
		}
	} else if err := db.Model(&session).Update("status", models.ForwardingStatusReceived).Error; err != nil {
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

	// #1934: 中转发货三件套（物流公司/单号/物流费）
	var req struct {
		TrackingCompany string  `json:"tracking_company" binding:"required"`
		TrackingNumber  string  `json:"tracking_number" binding:"required"`
		LogisticsFee    float64 `json:"logistics_fee"` // 元；outbound=分段②（顾客承担）return=分段④（商户承担）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "missing tracking_company, tracking_number or logistics_fee"})
		return
	}

	var session models.ForwardingSession
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "forwarding session not found"})
		return
	}
	if session.Status != models.ForwardingStatusReady {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session must be ready for last mile"})
		return
	}
	feeYuan := req.LogisticsFee
	updates := map[string]interface{}{
		"status":           models.ForwardingStatusLastMile,
		"tracking_company": req.TrackingCompany,
		"tracking_number":  req.TrackingNumber,
	}
	if err := db.Model(&session).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	// #1934: outbound ②（顾客承担）；return ④（商户承担）
	segment := TransitSegmentOutboundTransitToCustomer
	if session.Direction == models.ForwardingDirectionReturn {
		segment = TransitSegmentReturnTransitToControlled
	}
	recordTransitFee(db, session, segment, feeYuan, middleware.GetUserID(ctx))
	_ = feeYuan
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
			"status": models.OrderStatusCancelled,
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
