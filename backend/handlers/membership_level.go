package handlers

import (
	"net/http"
	"strconv"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func ListMembershipLevels(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var levels []models.MembershipLevel
	if err := db.Order("id ASC").Find(&levels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": levels})
}

func CreateMembershipLevel(c *gin.Context) {
	var req struct {
		ID        int     `json:"id" binding:"required"`
		Name      string  `json:"name" binding:"required"`
		MinAmount float64 `json:"min_amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	level := models.MembershipLevel{ID: req.ID, Name: req.Name, MinAmount: req.MinAmount}
	if err := db.Create(&level).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": level})
}

func UpdateMembershipLevel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	var req struct {
		Name      *string  `json:"name"`
		MinAmount *float64 `json:"min_amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.MinAmount != nil {
		updates["min_amount"] = *req.MinAmount
	}
	if err := db.Model(&models.MembershipLevel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

func DeleteMembershipLevel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if err := db.Delete(&models.MembershipLevel{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "deleted"})
}
