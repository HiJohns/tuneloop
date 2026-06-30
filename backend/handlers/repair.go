package handlers

import (
	"log"
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

type RepairHandler struct{}

func NewRepairHandler() *RepairHandler {
	return &RepairHandler{}
}

// StartRepair transitions an instrument from repair_pending to repair_in_progress,
// setting the current user as the repair worker.
func (h *RepairHandler) StartRepair(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var inst models.Instrument
	if err := db.Where("id = ?", instrumentID).First(&inst).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if inst.RepairStatus != "repair_pending" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument is not pending repair"})
		return
	}

	// If another worker is assigned, allow takeover
	if inst.RepairWorkerID != nil && *inst.RepairWorkerID != userID {
		// Allow takeover — caller has confirmed
	}

	if err := db.Model(&inst).Updates(map[string]interface{}{
		"repair_status":    "repair_in_progress",
		"repair_worker_id": userID,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to start repair"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair started"})
}

// CompleteRepair transitions an instrument from repair_in_progress to repair_completed.
// Requires at least one repair record with photos (validated in #1104).
func (h *RepairHandler) CompleteRepair(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var inst models.Instrument
	if err := db.Where("id = ?", instrumentID).First(&inst).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if inst.RepairStatus != "repair_in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument is not being repaired"})
		return
	}

	if inst.RepairWorkerID == nil || *inst.RepairWorkerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "only the assigned repair worker can complete"})
		return
	}

	if err := db.Model(&inst).Update("repair_status", "repair_completed").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to complete repair"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair completed"})
}

// AcceptRepair transitions an instrument from repair_completed to available.
// Only staff belonging to the instrument's site can accept.
func (h *RepairHandler) AcceptRepair(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var inst models.Instrument
	if err := db.Where("id = ?", instrumentID).First(&inst).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if inst.RepairStatus != "repair_completed" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument is not repair_completed"})
		return
	}

	// Verify the current user belongs to this instrument's site
	userID := middleware.GetUserID(ctx)
	siteID := inst.CurrentSiteID
	if siteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument has no site"})
		return
	}

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	var count int64
	db.Table("site_members").Where("user_id = ? AND site_id = ?", localUser.ID, siteID.String()).Count(&count)
	if count == 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "you are not a member of this instrument's site"})
		return
	}

	if err := db.Model(&inst).Updates(map[string]interface{}{
		"stock_status":    "available",
		"repair_status":   nil,
		"repair_worker_id": nil,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to accept repair"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair accepted, instrument is now available"})
}

// RejectRepair returns an instrument from repair_completed back to repair_in_progress
// with a required comment (stored via #1104 records API).
func (h *RepairHandler) RejectRepair(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Comment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "comment is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var inst models.Instrument
	if err := db.Where("id = ?", instrumentID).First(&inst).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if inst.RepairStatus != "repair_completed" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument is not repair_completed"})
		return
	}

	if err := db.Model(&inst).Update("repair_status", "repair_in_progress").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reject repair"})
		return
	}

	log.Printf("[Repair] Rejected: instrument=%s comment=%q", instrumentID, req.Comment)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair rejected, returned to in_progress"})
}
