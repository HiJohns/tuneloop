package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LabelHandler handles label management operations
type LabelHandler struct {
	db *gorm.DB
}

// NewLabelHandler creates a new label handler
func NewLabelHandler(db *gorm.DB) *LabelHandler {
	return &LabelHandler{db: db}
}

// GetLabels returns all labels for a tenant with optional filtering
func (h *LabelHandler) GetLabels(c *gin.Context) {
	status := c.DefaultQuery("status", "")

	var labels []struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Alias       string    `json:"alias"`
		AuditStatus string    `json:"audit_status"`
		CreatedAt   time.Time `json:"created_at"`
	}

	query := h.db.Table("labels").Where("tenant_id = ?", c.GetString("tenant_id"))
	if status != "" {
		query = query.Where("audit_status = ?", status)
	}

	if err := query.Find(&labels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch labels",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": labels,
	})
}

// CreateLabel creates a new label
func (h *LabelHandler) CreateLabel(c *gin.Context) {
	var req struct {
		Name  string   `json:"name" binding:"required"`
		Alias []string `json:"alias"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data",
		})
		return
	}

	label := map[string]interface{}{
		"tenant_id":    c.GetString("tenant_id"),
		"name":         req.Name,
		"alias":        toJSONString(req.Alias),
		"audit_status": "pending",
		"created_at":   time.Now(),
	}

	if err := h.db.Table("labels").Create(&label).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create label",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Label created successfully",
		"data":    label,
	})
}

// ApproveLabel approves a pending label
func (h *LabelHandler) ApproveLabel(c *gin.Context) {
	labelID := c.Param("id")

	if err := h.db.Table("labels").Where("id = ? AND tenant_id = ?", labelID, c.GetString("tenant_id")).Update("audit_status", "approved").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to approve label",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Label approved successfully",
	})
}

// RejectLabel rejects a pending label
func (h *LabelHandler) RejectLabel(c *gin.Context) {
	labelID := c.Param("id")

	if err := h.db.Table("labels").Where("id = ? AND tenant_id = ?", labelID, c.GetString("tenant_id")).Update("audit_status", "rejected").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to reject label",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Label rejected successfully",
	})
}

// MergeLabels merges multiple labels into a target label
func (h *LabelHandler) MergeLabels(c *gin.Context) {
	var req struct {
		SourceLabelIDs []string `json:"source_label_ids" binding:"required"`
		TargetLabelID  string   `json:"target_label_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data",
		})
		return
	}

	tenantID := c.GetString("tenant_id")

	// 1. Update source labels to point to target
	if err := h.db.Table("labels").Where("id IN ? AND tenant_id = ?", req.SourceLabelIDs, tenantID).Updates(map[string]interface{}{
		"normalized_to_id": req.TargetLabelID,
		"audit_status":     "merged",
		"updated_at":       time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update source labels",
		})
		return
	}

	// 2. Get source label names for replacement
	var sourceLabels []struct {
		Name string `json:"name"`
	}

	if err := h.db.Table("labels").Where("id IN ? AND tenant_id = ?", req.SourceLabelIDs, tenantID).Select("name").Find(&sourceLabels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch source label names",
		})
		return
	}

	// 3. Get target label name
	var targetLabel struct {
		Name string `json:"name"`
	}

	if err := h.db.Table("labels").Where("id = ? AND tenant_id = ?", req.TargetLabelID, tenantID).Select("name").First(&targetLabel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch target label name",
		})
		return
	}

	// 4. Update instrument metadata - this is a simplified version
	// In production, you would need to iterate through all instruments and update their metadata JSONB
	sourceNames := make([]string, len(sourceLabels))
	for i, label := range sourceLabels {
		sourceNames[i] = label.Name
	}

	// For now, return the mapping info
	// Actual update would be: UPDATE instruments SET metadata = metadata || jsonb_set(metadata, '{key}', '"new_value"')

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Labels merged successfully",
		"data": gin.H{
			"source_names":   sourceNames,
			"target_name":    targetLabel.Name,
			"affected_count": len(sourceLabels),
		},
	})
}

// Helper function to convert string slice to JSON string
func toJSONString(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	// Simple implementation - in production use proper JSON marshaling
	result := "["
	for i, s := range arr {
		if i > 0 {
			result += ","
		}
		result += "\"" + s + "\""
	}
	result += "]"
	return result
}
