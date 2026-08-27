package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/tencentcloud"

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
// #1782: accepts id_card_no, id_photo_front, id_photo_back, other_cert_photo.
func (h *GuarantorHandler) CreateGuarantor(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var req struct {
		Name           string `json:"name" binding:"required"`
		Phone          string `json:"phone" binding:"required"`
		Company        string `json:"company"`
		Title          string `json:"title"`
		Address        string `json:"address"`
		IDCardNo       string `json:"id_card_no" binding:"required"`
		IDPhotoFront   string `json:"id_photo_front" binding:"required"`
		IDPhotoBack    string `json:"id_photo_back" binding:"required"`
		OtherCertPhoto string `json:"other_cert_photo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	if len(req.IDCardNo) != 18 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "身份证号必须为18位"})
		return
	}

	g := models.Guarantor{
		ID:             uuid.New().String(),
		UserID:         userID,
		Name:           req.Name,
		Phone:          req.Phone,
		Company:        req.Company,
		Title:          req.Title,
		Address:        req.Address,
		IDCardNo:       req.IDCardNo,
		IDPhotoFront:   req.IDPhotoFront,
		IDPhotoBack:    req.IDPhotoBack,
		OtherCertPhoto: req.OtherCertPhoto,
		CreatedAt:      time.Now(),
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

// OCRIDCard performs ID card OCR on a previously uploaded image (#1782).
// POST /user/idcard-ocr { storage_key, side }
func (h *GuarantorHandler) OCRIDCard(c *gin.Context) {
	var req struct {
		StorageKey string `json:"storage_key" binding:"required"`
		Side       string `json:"side" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	if req.Side != "FRONT" && req.Side != "BACK" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "side must be FRONT or BACK"})
		return
	}

	// Resolve local file path from storage_key
	imagePath := filepath.Join("uploads", "media", req.StorageKey)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "image not found"})
		return
	}

	cfg := tencentcloud.LoadConfig()
	if !cfg.Configured() {
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"data":    gin.H{"available": false},
			"message": "OCR service not configured",
		})
		return
	}

	provider := tencentcloud.NewRealOCRProvider(cfg)
	info, err := provider.RecognizeIDCard(imagePath, req.Side)
	if err != nil {
		// Return available:false on failure so frontend can degrade to manual
		c.JSON(http.StatusOK, gin.H{
			"code":    20000,
			"data":    gin.H{"available": false},
			"message": "OCR recognition failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"available": true,
			"name":      info.Name,
			"sex":       info.Sex,
			"nation":    info.Nation,
			"birth":     info.Birth,
			"address":   info.Address,
			"id_num":    info.IdNum,
			"authority": info.Authority,
			"valid_date": info.ValidDate,
		},
	})
}
