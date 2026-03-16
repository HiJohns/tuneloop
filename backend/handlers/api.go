package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func GetInstruments(c *gin.Context) {
	c.File("data/instruments.json")
}

func GetSites(c *gin.Context) {
	c.File("data/sites.json")
}

func HandleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No file uploaded",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"fileName": file.Filename,
		"url":      "https://dummy.tuneloop.com/uploads/mock-image.jpg",
		"size":     file.Size,
	})
}

// GetOverdueLeases returns overdue lease data (replaces the old abnormal work orders API)
func GetOverdueLeases(c *gin.Context) {
	overdueLeases := []gin.H{
		{
			"id":              "LEASE-001",
			"instrument_name": "雅马哈 U1 立式钢琴",
			"renter_name":     "张三",
			"lease_end_date":  "2026-03-15",
			"overdue_days":    3,
			"contact":         "138****1234",
			"status":          "逾期",
		},
		{
			"id":              "LEASE-002",
			"instrument_name": "卡马 F1 民谣吉他",
			"renter_name":     "李四",
			"lease_end_date":  "2026-03-10",
			"overdue_days":    8,
			"contact":         "139****5678",
			"status":          "逾期",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  overdueLeases,
		"total": len(overdueLeases),
	})
}
