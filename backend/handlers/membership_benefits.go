package handlers

import (
	"net/http"
	"strconv"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// benefitItem is a single admin-editable benefit row (title + description).
type benefitItem struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

// ListLevelBenefits returns benefit rows of one membership level (admin).
func ListLevelBenefits(c *gin.Context) {
	levelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid level id"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	items, err := queryLevelBenefits(db, levelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"level_id": levelID, "list": items}})
}

// UpdateLevelBenefits bulk-replaces benefit rows of one membership level (admin).
func UpdateLevelBenefits(c *gin.Context) {
	levelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid level id"})
		return
	}
	var req struct {
		Items []benefitItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var level models.MembershipLevel
	if err := db.Where("id = ?", levelID).First(&level).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "membership level not found"})
		return
	}

	var rows []models.MembershipLevelBenefit
	for i, item := range req.Items {
		rows = append(rows, models.MembershipLevelBenefit{
			LevelID:     levelID,
			SortOrder:   i + 1,
			Title:       item.Title,
			Description: item.Description,
		})
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("level_id = ?", levelID).Delete(&models.MembershipLevelBenefit{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			return tx.Create(&rows).Error
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"level_id": levelID, "list": rows}})
}

// GetMembershipBenefits returns benefit rows for the given membership level
// (customer-facing, e.g. Membership Center page).
func GetMembershipBenefits(c *gin.Context) {
	levelIDStr := c.Query("level_id")
	if levelIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "level_id is required"})
		return
	}
	levelID, err := strconv.Atoi(levelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid level_id"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	items, err := queryLevelBenefits(db, levelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"level_id": levelID, "list": items}})
}

func queryLevelBenefits(db *gorm.DB, levelID int) ([]models.MembershipLevelBenefit, error) {
	var items []models.MembershipLevelBenefit
	err := db.Where("level_id = ?", levelID).Order("sort_order ASC").Find(&items).Error
	return items, err
}
