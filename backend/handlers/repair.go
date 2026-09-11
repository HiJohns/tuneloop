package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

	// Verify at least one repair record with photos exists (#1875: photos is
	// JSONB — comparing it to '' raises a Postgres error, so use jsonb_typeof/
	// jsonb_array_length which are safe for any valid JSON value).
	var recordCount int64
	if err := db.Model(&models.RepairRecord{}).
		Where("instrument_id = ? AND worker_id = ?", instrumentID, userID).
		Where("photos IS NOT NULL AND jsonb_typeof(photos) = 'array' AND jsonb_array_length(photos) > 0").
		Count(&recordCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to verify repair records"})
		return
	}
	if recordCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "at least one photo record is required before completing"})
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

	// #1882 R1: only staff of this instrument's site may accept, and the
	// assigned repair worker can never accept their own repair.
	userID := middleware.GetUserID(ctx)
	siteID := inst.CurrentSiteID
	if siteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument has no site"})
		return
	}
	memberships := resolveOperatorSiteMemberships(db, userID)
	if !hasSiteRole(memberships, siteID.String(), "site_admin", "site_member") {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "only staff of this instrument's site can accept"})
		return
	}
	if inst.RepairWorkerID != nil && *inst.RepairWorkerID == userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "the repair worker cannot accept their own repair"})
		return
	}

	if err := db.Model(&inst).Updates(map[string]interface{}{
		"stock_status":     "available",
		"repair_status":    nil,
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

	// #1882 R1: same site/role gate as accept; the repair worker cannot reject
	// their own repair either.
	userID := middleware.GetUserID(ctx)
	siteID := inst.CurrentSiteID
	if siteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument has no site"})
		return
	}
	memberships := resolveOperatorSiteMemberships(db, userID)
	if !hasSiteRole(memberships, siteID.String(), "site_admin", "site_member") {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "only staff of this instrument's site can reject"})
		return
	}
	if inst.RepairWorkerID != nil && *inst.RepairWorkerID == userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "the repair worker cannot reject their own repair"})
		return
	}

	// #1882 R3: persist the rejection reason as a repair record
	record := models.RepairRecord{
		ID:           uuid.New().String(),
		InstrumentID: instrumentID,
		WorkerID:     userID,
		Comment:      "验收驳回：" + req.Comment,
		Photos:       "[]",
		CreatedAt:    time.Now(),
	}
	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record rejection reason"})
		return
	}

	if err := db.Model(&inst).Update("repair_status", "repair_in_progress").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reject repair"})
		return
	}

	log.Printf("[Repair] Rejected: instrument=%s comment=%q", instrumentID, req.Comment)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair rejected, returned to in_progress"})
}

// TakeoverRepair sets the current user as the repair worker for an instrument
// that is repair_pending or repair_in_progress. Used when a worker takes over
// an existing repair from another worker.
func (h *RepairHandler) TakeoverRepair(c *gin.Context) {
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

	if inst.RepairStatus != "repair_pending" && inst.RepairStatus != "repair_in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument must be pending or in progress"})
		return
	}

	// #1882 R4: takeover is restricted to the instrument's site (no cross-site)
	siteID := inst.CurrentSiteID
	if siteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument has no site"})
		return
	}
	memberships := resolveOperatorSiteMemberships(db, userID)
	if !hasSiteRole(memberships, siteID.String(), "site_admin", "site_member", "repair_technician") {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "you do not belong to this instrument's site"})
		return
	}

	if err := db.Model(&inst).Updates(map[string]interface{}{
		"repair_status":    "repair_in_progress",
		"repair_worker_id": userID,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to takeover repair"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair taken over"})
}

// ReassignRepair changes the repair worker for an active repair.
// Intended for admin/manager use.
func (h *RepairHandler) ReassignRepair(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var req struct {
		WorkerID string `json:"worker_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.WorkerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "worker_id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var inst models.Instrument
	if err := db.Where("id = ?", instrumentID).First(&inst).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if inst.RepairStatus != "repair_in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument must be in progress"})
		return
	}

	// #1882 R5: only the instrument's site_admin may reassign; the target must
	// be a member of the same site.
	userID := middleware.GetUserID(ctx)
	siteID := inst.CurrentSiteID
	if siteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "instrument has no site"})
		return
	}
	if !hasSiteRole(resolveOperatorSiteMemberships(db, userID), siteID.String(), "site_admin") {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "only site_admin can reassign"})
		return
	}
	if !hasSiteRole(resolveOperatorSiteMemberships(db, req.WorkerID), siteID.String(), "site_admin", "site_member", "repair_technician") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "worker is not a member of this instrument's site"})
		return
	}

	if err := db.Model(&inst).Update("repair_worker_id", req.WorkerID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reassign repair"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair reassigned"})
}

// AddRecord adds a repair record (comment + photos) to an instrument repair session.
func (h *RepairHandler) AddRecord(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var req struct {
		Comment string   `json:"comment"`
		Photos  []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	photosJSON, _ := json.Marshal(req.Photos)

	record := models.RepairRecord{
		ID:           uuid.New().String(),
		InstrumentID: instrumentID,
		WorkerID:     userID,
		Comment:      req.Comment,
		Photos:       string(photosJSON),
		CreatedAt:    time.Now(),
	}
	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create record"})
		return
	}

	if len(req.Photos) > 0 {
		tenantID := middleware.GetTenantID(ctx)
		orgID := middleware.GetOrgID(ctx)
		batchID := uuid.New().String()
		for i, url := range req.Photos {
			media := models.InstrumentMedia{
				TenantID:     tenantID,
				OrgID:        orgID,
				InstrumentID: &instrumentID,
				BatchID:      batchID,
				BatchType:    "repair",
				FileName:     filepath.Base(url),
				FileType:     "image",
				StorageKey:   url,
				IsDisplay:    false,
				SortOrder:    i,
			}
			db.Create(&media)
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": record.ID}})
}

// ListRecords returns all repair records for an instrument, ordered by creation time.
func (h *RepairHandler) ListRecords(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var records []models.RepairRecord
	if err := db.Where("instrument_id = ?", instrumentID).Order("created_at ASC").Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query records"})
		return
	}

	// (#1873) resolve worker display names (worker_id stores the IAM sub)
	workerIDs := make([]string, 0, len(records))
	for _, r := range records {
		if r.WorkerID != "" {
			workerIDs = append(workerIDs, r.WorkerID)
		}
	}
	workerNames := resolveUserNamesByIAMSub(db, workerIDs)

	recordList := make([]gin.H, len(records))
	for i, r := range records {
		recordList[i] = gin.H{
			"id":            r.ID,
			"instrument_id": r.InstrumentID,
			"worker_id":     r.WorkerID,
			"worker_name":   workerNames[r.WorkerID],
			"comment":       r.Comment,
			"photos":        r.Photos,
			"created_at":    r.CreatedAt,
		}
	}

	// (#1866): include latest damage report for context display
	var latestDamage models.DamageReport
	damageObj := interface{}(nil)
	if err := db.Where("instrument_id = ?", instrumentID).Order("created_at DESC").First(&latestDamage).Error; err == nil {
		damageObj = latestDamage
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"records": recordList, "damage": damageObj}})
}

// ListMyRepairs returns all repairs assigned to the current user.
func (h *RepairHandler) ListMyRepairs(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var instruments []models.Instrument
	if err := db.Where("repair_worker_id = ? AND repair_status IS NOT NULL", userID).
		Order("updated_at DESC").Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query repairs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": instruments}})
}

// ListPendingRepairs returns repair_pending instruments at the operator's
// sites (#1882: site-scoped, no tenant-wide leak).
func (h *RepairHandler) ListPendingRepairs(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	memberships := resolveOperatorSiteMemberships(db, userID)
	siteIDs := make([]string, 0, len(memberships))
	for _, m := range memberships {
		siteIDs = append(siteIDs, m.SiteID)
	}
	if len(siteIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": []interface{}{}}})
		return
	}

	var instruments []models.Instrument
	if err := db.Where("current_site_id IN ? AND repair_status = ?", siteIDs, "repair_pending").
		Order("updated_at DESC").Find(&instruments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query repairs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": instruments}})
}

// siteMembership is one site_members row for an operator (#1882).
type siteMembership struct {
	SiteID string
	Role   string
}

// resolveOperatorSiteMemberships returns all site memberships for the operator
// (#1882): used for site- and role-scoped gating across the repair domain.
// Returns nil for unknown users or lookup failures.
func resolveOperatorSiteMemberships(db *gorm.DB, userID string) []siteMembership {
	if userID == "" {
		return nil
	}
	var localUser models.User
	if err := db.Select("id").Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		return nil
	}
	var members []models.SiteMember
	if err := db.Where("user_id = ?", localUser.ID).Order("created_at ASC").Find(&members).Error; err != nil {
		return nil
	}
	out := make([]siteMembership, 0, len(members))
	for _, m := range members {
		out = append(out, siteMembership{SiteID: m.SiteID, Role: m.Role})
	}
	return out
}

// hasSiteRole reports whether the memberships grant one of roles at siteID.
func hasSiteRole(memberships []siteMembership, siteID string, roles ...string) bool {
	for _, m := range memberships {
		if m.SiteID != siteID {
			continue
		}
		for _, r := range roles {
			if m.Role == r {
				return true
			}
		}
	}
	return false
}

// resolveOperatorSiteID returns the site the operator belongs to (first
// site_members row, oldest first). Used when an instrument enters the repair
// flow so the acceptance gate has a site to check (#1889). Returns nil when
// the operator has no site membership — callers fall back to the order's site.
func resolveOperatorSiteID(db *gorm.DB, userID string) *string {
	memberships := resolveOperatorSiteMemberships(db, userID)
	if len(memberships) == 0 {
		return nil
	}
	siteID := memberships[0].SiteID
	if siteID == "" {
		return nil
	}
	return &siteID
}
