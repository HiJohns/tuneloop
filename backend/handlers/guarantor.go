package handlers

import (
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GuarantorHandler manages the current user's deposit guarantors (#1557).
// Registered in userOptionalAuth — customers have no tenant/org context.
type GuarantorHandler struct{}

func NewGuarantorHandler() *GuarantorHandler {
	return &GuarantorHandler{}
}

// ListGuarantors returns the current user's saved guarantors.
func (h *GuarantorHandler) ListGuarantors(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var guarantors []models.Guarantor
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&guarantors).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query guarantors"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": guarantors,
		},
	})
}

// CreateGuarantor saves a new guarantor for the current user.
func (h *GuarantorHandler) CreateGuarantor(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var req struct {
		Name    string `json:"name" binding:"required"`
		Phone   string `json:"phone" binding:"required"`
		Company string `json:"company"`
		Title   string `json:"title"`
		Address string `json:"address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	g := models.Guarantor{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		Phone:     req.Phone,
		Company:   req.Company,
		Title:     req.Title,
		Address:   req.Address,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&g).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create guarantor"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    g,
	})
}

// DeleteGuarantor removes a guarantor belonging to the current user.
func (h *GuarantorHandler) DeleteGuarantor(c *gin.Context) {
	ctx := c.Request.Context()
	guarantorID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	result := db.Where("id = ? AND user_id = ?", guarantorID, userID).Delete(&models.Guarantor{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "guarantor not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}
