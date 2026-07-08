package handlers

import (
	"log"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SubmitQuote submits a repair quote (v3). Reads repair-request ID from path param :id.
func SubmitQuote(c *gin.Context) {
	repairRequestID := c.Param("id")
	if repairRequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "repair_request_id required"})
		return
	}

	var req struct {
		MaterialFee  float64 `json:"material_fee"`
		ServiceFee   float64 `json:"service_fee"`
		LogisticsFee float64 `json:"logistics_fee"`
		Duration     string  `json:"duration"`
		Comment      string  `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request"})
		return
	}

	// Scan comment for sensitive content
	if req.Comment != "" {
		if services.HandleSensitiveQuote(repairRequestID, "", req.Comment) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "quote comment contains sensitive information"})
			return
		}
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	// Determine the technician's site
	var siteID string
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		var members []models.SiteMember
		db.Where("user_id = ? AND role = ?", localUser.ID, "repair_technician").Limit(1).Find(&members)
		if len(members) > 0 {
			siteID = members[0].SiteID
		}
	}

	// Generate a unique quote number
	quoteNo := "Q" + uuid.New().String()[:8]

	quote := models.RepairQuote{
		ID:              uuid.New().String(),
		RepairRequestID: repairRequestID,
		SiteID:          siteID,
		WorkerID:        userID,
		QuoteNo:         quoteNo,
		MaterialFee:     req.MaterialFee,
		ServiceFee:      req.ServiceFee,
		LogisticsFee:    req.LogisticsFee,
		Duration:        req.Duration,
		Comment:         req.Comment,
		Status:          models.RepairQuotePending,
		CreatedAt:       time.Now(),
	}
	if err := db.Create(&quote).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create quote"})
		return
	}

	// Notify customer of new quote
	var reqModel models.RepairRequest
	if err := db.Where("id = ?", repairRequestID).First(&reqModel).Error; err == nil {
		var customerUser models.User
		if err := database.GetDB().Where("iam_sub = ?", reqModel.UserID).First(&customerUser).Error; err == nil {
			title := "收到新报价"
			content := "您的报修单收到一份新报价，请查看并确认。"
			services.Notify(db, reqModel.TenantID, customerUser.ID, "quote", title, content, repairRequestID, "repair_request")
		}
	}

	createRepairRecord(db, repairRequestID, userID, "quote_submitted", "师傅提交报价", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": quote})
}

// ListQuotes lists quotes for a repair request with visibility filtering (v3).
func ListQuotes(c *gin.Context) {
	repairRequestID := c.Param("id")
	if repairRequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "repair_request_id required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	role := middleware.GetRole(ctx)

	var quotes []models.RepairQuote
	query := db.Where("repair_request_id = ?", repairRequestID)

	// Visibility filtering (v3):
	// - The repair request owner sees all quotes (with desensitization for controlled)
	// - Technicians see only quotes from their own site
	// - Cross-site quotes are invisible

	// Check if this is a controlled repair request (for desensitization)
	var repairReq models.RepairRequest
	isControlled := false
	if err := db.Where("id = ?", repairRequestID).First(&repairReq).Error; err == nil {
		isControlled = repairReq.MerchantType == models.MerchantTypeControlled
	}

	if role == "USER" {
		query.Find(&quotes)
	} else {
		// Staff/tech: find the user's site, then show only that site's quotes
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			var members []models.SiteMember
			db.Where("user_id = ?", localUser.ID).Limit(1).Find(&members)
			if len(members) > 0 {
				query = query.Where("site_id = ?", members[0].SiteID)
			}
		}
		query.Find(&quotes)
	}

	// Desensitize for controlled: strip worker/site identity for USER
	if isControlled && role == "USER" {
		desensitized := make([]gin.H, len(quotes))
		for i, q := range quotes {
			desensitized[i] = gin.H{
				"id":            q.ID,
				"quote_no":      q.QuoteNo,
				"material_fee":  q.MaterialFee,
				"service_fee":   q.ServiceFee,
				"logistics_fee": q.LogisticsFee,
				"duration":      q.Duration,
				"comment":       q.Comment,
				"status":        q.Status,
				"created_at":    q.CreatedAt,
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": desensitized}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": quotes}})
}

// AcceptQuote marks a quote as accepted and transitions the repair request (v3).
// Path params: :id = repair_request_id, :qid = quote_id
func AcceptQuote(c *gin.Context) {
	repairRequestID := c.Param("id")
	quoteID := c.Param("qid")
	if repairRequestID == "" || quoteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "repair_request_id and quote_id required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var quote models.RepairQuote
	if err := db.Where("id = ? AND repair_request_id = ?", quoteID, repairRequestID).First(&quote).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "quote not found"})
		return
	}
	if quote.Status != models.RepairQuotePending {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "quote is not pending"})
		return
	}

	// Transaction: mark accepted, supersede others, set accepted_quote_id, transition status
	tx := db.Begin()

	// Mark this quote as accepted
	if err := tx.Model(&quote).Update("status", models.RepairQuoteAccepted).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to accept quote"})
		return
	}

	// Supersede all other pending quotes for this repair request
	tx.Model(&models.RepairQuote{}).
		Where("repair_request_id = ? AND id != ? AND status = ?", repairRequestID, quoteID, models.RepairQuotePending).
		Update("status", models.RepairQuoteSuperseded)

	updates := map[string]interface{}{
		"accepted_quote_id": quoteID,
		"status":            models.RepairReqStatusPendingPay,
	}

	// If controlled, record the winning quote's site as controlled_site_id
	if quote.SiteID != "" {
		updates["controlled_site_id"] = quote.SiteID
	}

	if err := tx.Model(&models.RepairRequest{}).Where("id = ?", repairRequestID).Updates(updates).Error; err != nil {
		tx.Rollback()
		log.Printf("[AcceptQuote] Failed to update repair request %s: %v", repairRequestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update repair request"})
		return
	}

	tx.Commit()

	// Notify winning technician's site
	var reqModel models.RepairRequest
	if err := db.Where("id = ?", repairRequestID).First(&reqModel).Error; err == nil {
		if quote.SiteID != "" {
			title := "报价已被接受"
			content := "您的报价已被报修人接受，报修单进入待付款状态。"
			services.NotifyTechniciansOfSite(db, reqModel.TenantID, quote.SiteID, "quote_accepted", title, content, repairRequestID, "repair_request")
		}
	}

	createRepairRecord(db, repairRequestID, middleware.GetUserID(ctx), "quote_accepted", "接受报价", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "quote accepted"})
}
