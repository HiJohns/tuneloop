package handlers

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
	"tuneloop-backend/middleware"
	"tuneloop-backend/service"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// getMemoryUsage returns current memory usage in MB
func getMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}

// ImportInstruments handles Excel file upload and import with performance monitoring
func ImportInstruments(c *gin.Context) {
	// Performance monitoring start
	startTime := time.Now()
	startMemory := getMemoryUsage()
	fmt.Printf("[PERF] Import started: file=%s, time=%s, memory=%dMB\n",
		c.Request.FormValue("file"), startTime.Format(time.RFC3339), startMemory)

	// Get tenant ID from context
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Tenant ID is required",
		})
		return
	}

	// Parse multipart form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "File upload failed: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// Validate file type
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") &&
		!strings.HasSuffix(strings.ToLower(header.Filename), ".xls") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "Only Excel files (.xlsx, .xls) are supported",
		})
		return
	}

	// Read Excel file with performance tracking
	parseStart := time.Now()
	excelFile, err := excelize.OpenReader(file)
	parseDuration := time.Since(parseStart)
	parseMemory := getMemoryUsage()

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "Failed to read Excel file: " + err.Error(),
		})
		return
	}

	// Get total rows for reporting
	sheetName := excelFile.GetSheetName(0)
	rows, _ := excelFile.GetRows(sheetName)
	totalRows := len(rows) - 1 // Subtract header row

	fmt.Printf("[PERF] Parse completed: file=%s, duration=%.3fs, memory=%dMB, records=%d\n",
		header.Filename, parseDuration.Seconds(), parseMemory, totalRows)

	// Get database connection
	db, exists := c.Get("db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "Database connection not available",
		})
		return
	}

	// Create service and import
	instrumentService := service.NewInstrumentService(db.(*gorm.DB))

	// Note: Service needs to be modified to accept batch callbacks for monitoring
	// For now, we measure the total import duration
	importStart := time.Now()
	result, err := instrumentService.ImportInstruments(excelFile, tenantID)
	importDuration := time.Since(importStart)
	endMemory := getMemoryUsage()

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40005,
			"message": "Import failed: " + err.Error(),
		})
		return
	}

	// Calculate throughput
	throughput := 0.0
	if importDuration.Seconds() > 0 {
		throughput = float64(result.Total) / importDuration.Seconds()
	}

	// Log final performance metrics
	totalDuration := time.Since(startTime)
	fmt.Printf("[PERF] Import completed: file=%s, total_duration=%.3fs, "+
		"import_duration=%.3fs, memory_start=%dMB, memory_end=%dMB, "+
		"success=%d, failed=%d, throughput=%.1f records/s\n",
		header.Filename, totalDuration.Seconds(), importDuration.Seconds(),
		startMemory, endMemory, result.Success, result.Failed, throughput)

	// Return result (supports partial success)
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
		"message": fmt.Sprintf("Import completed: %d success, %d failed (%.1f records/s)",
			result.Success, result.Failed, throughput),
	})
}

// ExportInstruments handles instrument export to Excel
func ExportInstruments(c *gin.Context) {
	// Get tenant ID from context
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Tenant ID is required",
		})
		return
	}

	// Parse query parameters
	opts := service.ExportOptions{
		Category:   c.Query("category"),
		Status:     c.Query("status"),
		SearchText: c.Query("search_text"),
	}

	// Parse fields parameter (comma-separated)
	fieldsParam := c.Query("fields")
	if fieldsParam != "" {
		opts.Fields = strings.Split(fieldsParam, ",")
		// Trim spaces
		for i := range opts.Fields {
			opts.Fields[i] = strings.TrimSpace(opts.Fields[i])
		}
	}

	// Get database connection
	db, exists := c.Get("db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "Database connection not available",
		})
		return
	}

	// Create service and export
	instrumentService := service.NewInstrumentService(db.(*gorm.DB))
	excelFile, err := instrumentService.ExportInstruments(opts, tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40006,
			"message": "Export failed: " + err.Error(),
		})
		return
	}

	// Set response headers for file download
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=\"instruments_"+
		fmt.Sprintf("%d", c.GetInt64("request_time"))+".xlsx\"")

	// Write Excel file to response
	if err := excelFile.Write(c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "Failed to generate Excel file: " + err.Error(),
		})
		return
	}
}

// DownloadImportTemplate provides a template Excel file for import
func DownloadImportTemplate(c *gin.Context) {
	f := excelize.NewFile()
	sheetName := "Template"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)

	// Set sample data headers
	headers := []string{
		"乐器名称", "品牌", "型号", "分类名称", "级别",
		"日租金", "月租金", "押金", "库存数量", "状态", "描述", "图片URL",
	}

	// Write header row
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// Write sample data row
	sampleData := []interface{}{
		"雅马哈立式钢琴 U1", "Yamaha", "U1", "钢琴", "entry",
		50, 1200, 5000, 5, "available", "经典立式钢琴", "",
	}
	for i, value := range sampleData {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheetName, cell, value)
	}

	// Add notes/comments
	f.SetCellValue(sheetName, "A4", "说明:")
	f.SetCellValue(sheetName, "A5", "- 级别可选: entry/pro/master")
	f.SetCellValue(sheetName, "A6", "- 状态可选: available/rented/maintenance")
	f.SetCellValue(sheetName, "A7", "- 红色标题为必填项")
	f.SetCellValue(sheetName, "A8", "- 价格和押金请输入数字，不要包含货币符号")

	// Style header row (make required fields red)
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "FF0000", Bold: true},
	})
	requiredIndices := []int{0, 3} // 乐器名称 and 分类名称
	for _, idx := range requiredIndices {
		cell, _ := excelize.CoordinatesToCellName(idx+1, 1)
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	// Auto-size columns
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheetName, col, col, 15)
	}

	// Set response headers
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=\"instrument_import_template.xlsx\"")

	// Write to response
	if err := f.Write(c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "Failed to generate template: " + err.Error(),
		})
		return
	}
}
