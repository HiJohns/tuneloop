package handlers

import (
	"bytes"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
	"gorm.io/gorm"
	"tuneloop-backend/models"
)

// GenerateAssessmentReport generates a PDF assessment report for a completed lease
func GenerateAssessmentReport(db *gorm.DB, orderID string, c *gin.Context) ([]byte, error) {
	// Fetch order details
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	// Fetch instrument details
	var instrument models.Instrument
	if err := db.Where("id = ?", order.InstrumentID).First(&instrument).Error; err != nil {
		return nil, fmt.Errorf("instrument not found: %w", err)
	}

	// Fetch user details
	var user models.User
	if err := db.Where("id = ?", order.UserID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Create PDF
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Title
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "归还鉴定报告")
	pdf.Ln(12)

	// Report info
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 8, fmt.Sprintf("报告编号: %s", orderID))
	pdf.Ln(8)
	pdf.Cell(40, 8, fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	pdf.Ln(12)

	// Asset information
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "资产信息")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 8, fmt.Sprintf("乐器名称: %s", instrument.Name))
	pdf.Ln(8)
	pdf.Cell(40, 8, fmt.Sprintf("品牌: %s", instrument.Brand))
	pdf.Ln(8)
	pdf.Cell(40, 8, fmt.Sprintf("型号: %s", instrument.Model))
	pdf.Ln(8)
	pdf.Cell(40, 8, fmt.Sprintf("序列号: %s", instrument.SN))
	pdf.Ln(12)

	// Lease information
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "租赁信息")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 8, fmt.Sprintf("用户: %s", user.Name))
	pdf.Ln(8)
	startDate := ""
	if order.StartDate != nil {
		startDate = *order.StartDate
	}
	pdf.Cell(40, 8, fmt.Sprintf("开始日期: %s", startDate))
	pdf.Ln(8)
	endDate := ""
	if order.EndDate != nil {
		endDate = *order.EndDate
	}
	pdf.Cell(40, 8, fmt.Sprintf("结束日期: %s", endDate))
	pdf.Ln(12)

	// Assessment details
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "定损结果")
	pdf.Ln(10)

	// TODO: Add actual damage assessment details
	// For now, add placeholder
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 8, "定损描述: 待补充")
	pdf.Ln(8)
	pdf.Cell(40, 8, "损伤程度: 待评估")
	pdf.Ln(8)

	// Signature (if available)
	pdf.Ln(20)
	pdf.Cell(40, 8, "鉴定人签字: ___________")
	pdf.Ln(8)
	pdf.Cell(40, 8, "鉴定日期: ___________")

	// Save PDF to temporary buffer
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}

// HandleAssessmentReport handles the API endpoint for generating assessment reports
func HandleAssessmentReport(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID := c.Param("order_id")

		// Generate PDF
		pdfBytes, err := GenerateAssessmentReport(db, orderID, c)
		if err != nil {
			c.JSON(500, gin.H{
				"code":    50000,
				"message": fmt.Sprintf("Failed to generate report: %v", err),
			})
			return
		}

		// Set headers for PDF download
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=assessment_report_%s.pdf", orderID))
		c.Header("Content-Type", "application/pdf")
		c.Data(200, "application/pdf", pdfBytes)
	}
}
