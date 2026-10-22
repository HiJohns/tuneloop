package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// resolveOverrideAnchor（#2260）：促销覆盖的归属 = **乐器商户 org**（数据归属）。
// 可见范围按乐器 tenant（= 商户 org）过滤；操作者 tid 为空（平台管理员）时不加范围，
// 避免历史 instrumentation 行（org_id 为 NULL）被 org-scope 误排除。
// 返回 ("", false) → 404（跨商户不可见）。
func resolveOverrideAnchor(c *gin.Context, db *gorm.DB, instrumentID string) (string, bool) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	q := db.Model(&models.Instrument{}).Where("id = ?", instrumentID)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var inst models.Instrument
	if err := q.Select("tenant_id").First(&inst).Error; err != nil || inst.TenantID == "" {
		return "", false
	}
	return inst.TenantID, true
}

func GetInstrumentPromoOverrides(c *gin.Context) {
	instrumentID := c.Param("id")
	if _, err := uuid.Parse(instrumentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid instrument id"})
		return
	}
	db := database.GetDB().WithContext(c.Request.Context())
	anchor, ok := resolveOverrideAnchor(c, db, instrumentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}
	var overrides []models.InstrumentPromoOverride
	if err := db.Where("tenant_id = ? AND instrument_id = ?", anchor, instrumentID).Find(&overrides).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": overrides})
}

func UpdateInstrumentPromoOverride(c *gin.Context) {
	instrumentID := c.Param("id")
	if _, err := uuid.Parse(instrumentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid instrument id"})
		return
	}
	var req struct {
		OverrideType string  `json:"override_type" binding:"required"`
		Enabled      *bool   `json:"enabled"`
		Content      *string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if req.OverrideType != "discount" && req.OverrideType != "rebate" && req.OverrideType != "rent_to_own" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "override_type must be discount, rebate, or rent_to_own"})
		return
	}
	db := database.GetDB().WithContext(c.Request.Context())
	// #2260：归属 = 乐器商户 org（写入数据归属，不用操作者 tid）
	anchor, ok := resolveOverrideAnchor(c, db, instrumentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	var existing models.InstrumentPromoOverride
	result := db.Where("tenant_id = ? AND instrument_id = ? AND override_type = ?", anchor, instrumentID, req.OverrideType).First(&existing)
	if result.Error != nil {
		override := models.InstrumentPromoOverride{
			TenantID:     anchor,
			InstrumentID: instrumentID,
			OverrideType: req.OverrideType,
		}
		if req.Enabled != nil {
			v := *req.Enabled
			override.Enabled = &v
		}
		if req.Content != nil {
			override.Content = *req.Content
		}
		if err := db.Create(&override).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	} else {
		// #1863 audit fix: only update fields explicitly provided — an omitted
		// content must NOT wipe the stored custom copy (PC switch sends
		// enabled-only). Empty string still resets to the default copy.
		updates := map[string]interface{}{}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
		if req.Content != nil {
			updates["content"] = *req.Content
		}
		if len(updates) > 0 {
			if err := db.Model(&existing).Updates(updates).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
				return
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
