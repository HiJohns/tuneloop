package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"tuneloop-backend/models"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type InstrumentService struct {
	db *gorm.DB
}

func NewInstrumentService(db *gorm.DB) *InstrumentService {
	return &InstrumentService{db: db}
}

var headerMap = map[string]string{
	"name":           "乐器名称",
	"brand":          "品牌",
	"model":          "型号",
	"category_name":  "分类名称",
	"level":          "级别",
	"daily_rate":     "日租金",
	"monthly_rate":   "月租金",
	"deposit":        "押金",
	"stock":          "库存数量",
	"status":         "状态",
	"description":    "描述",
	"images":         "图片URL",
}

var requiredFields = []string{"name", "category_name"}

var validLevels = map[string]bool{
	"entry":  true,
	"pro":    true,
	"master": true,
}

var validStatuses = map[string]bool{
	"available":   true,
	"rented":      true,
	"maintenance": true,
}

type ImportResult struct {
	Total   int            `json:"total"`
	Success int            `json:"success"`
	Failed  int            `json:"failed"`
	Errors  []ImportError  `json:"errors"`
}

type ImportError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

type ExportOptions struct {
	Fields     []string `json:"fields"`
	Category   string   `json:"category"`
	Status     string   `json:"status"`
	SearchText string   `json:"search_text"`
}

func (s *InstrumentService) ImportInstruments(file *excelize.File, tenantID string) (*ImportResult, error) {
	result := &ImportResult{
		Errors: []ImportError{},
	}

	sheetName := file.GetSheetName(0)
	if sheetName == "" {
		return nil, fmt.Errorf("no sheet found in Excel file")
	}

	rows, err := file.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read Excel rows: %w", err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("Excel file must contain header row and at least one data row")
	}

	headerRow := rows[0]
	columnIndex := s.parseHeaderRow(headerRow)

	if err := s.validateRequiredColumns(columnIndex); err != nil {
		return nil, err
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for i, row := range rows[1:] {
		rowNum := i + 2
		result.Total++

		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:   rowNum,
				Error: "Empty row",
			})
			continue
		}

		instrument, err := s.parseRow(row, columnIndex, rowNum)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:   rowNum,
				Error: err.Error(),
			})
			tx.Rollback()
			tx = s.db.Begin()
			continue
		}

		instrument.TenantID = tenantID

		var existing models.Instrument
		dupQuery := tx.Where("tenant_id = ? AND name = ?", tenantID, instrument.Name)
		if instrument.Brand != "" {
			dupQuery = dupQuery.Where("brand = ?", instrument.Brand)
		}
		if err := dupQuery.First(&existing).Error; err == nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:   rowNum,
				Error: "Instrument already exists",
			})
			tx.Rollback()
			tx = s.db.Begin()
			continue
		}

		if err := tx.Create(&instrument).Error; err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:   rowNum,
				Error: fmt.Sprintf("Database error: %v", err),
			})
			tx.Rollback()
			tx = s.db.Begin()
			continue
		}

		result.Success++

		if result.Success%100 == 0 {
			if err := tx.Commit().Error; err != nil {
				return nil, fmt.Errorf("failed to commit batch: %w", err)
			}
			tx = s.db.Begin()
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit final batch: %w", err)
	}

	return result, nil
}

func (s *InstrumentService) parseHeaderRow(headerRow []string) map[string]int {
	columnIndex := make(map[string]int)
	
	for i, header := range headerRow {
		header = strings.TrimSpace(header)
		for field, chineseHeader := range headerMap {
			if header == field || header == chineseHeader {
				columnIndex[field] = i
				break
			}
		}
	}
	
	return columnIndex
}

func (s *InstrumentService) validateRequiredColumns(columnIndex map[string]int) error {
	missing := []string{}
	for _, field := range requiredFields {
		if _, exists := columnIndex[field]; !exists {
			missing = append(missing, headerMap[field])
		}
	}
	
	if len(missing) > 0 {
		return fmt.Errorf("missing required columns: %s", strings.Join(missing, ", "))
	}
	
	return nil
}

func (s *InstrumentService) parseRow(row []string, columnIndex map[string]int, rowNum int) (*models.Instrument, error) {
	instrument := &models.Instrument{}

	getCellValue := func(field string) string {
		if idx, exists := columnIndex[field]; exists && idx < len(row) {
			value := strings.TrimSpace(row[idx])
			return s.sanitizeValue(value)
		}
		return ""
	}

	instrument.Name = getCellValue("name")
	if instrument.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	instrument.Brand = getCellValue("brand")
	instrument.Description = getCellValue("description")

	level := strings.ToLower(getCellValue("level"))
	if level != "" {
		if !validLevels[level] {
			return nil, fmt.Errorf("invalid level: %s (must be entry/pro/master)", level)
		}
		instrument.Level = level
	} else {
		instrument.Level = "entry"
	}

	status := strings.ToLower(getCellValue("status"))
	if status == "" {
		status = "available"
	} else if !validStatuses[status] {
		return nil, fmt.Errorf("invalid status: %s (must be available/rented/maintenance)", status)
	}
	instrument.StockStatus = status

	if dailyRateStr := getCellValue("daily_rate"); dailyRateStr != "" {
		rate, err := s.parseFloat(dailyRateStr, "daily_rate", rowNum)
		if err != nil {
			return nil, err
		}
		instrument.Pricing = fmt.Sprintf(`{"daily_rate": %.2f}`, rate)
	}

	if monthlyRateStr := getCellValue("monthly_rate"); monthlyRateStr != "" {
		rate, err := s.parseFloat(monthlyRateStr, "monthly_rate", rowNum)
		if err != nil {
			return nil, err
		}
		if instrument.Pricing == "" {
			instrument.Pricing = fmt.Sprintf(`{"monthly_rate": %.2f}`, rate)
		} else {
			instrument.Pricing = fmt.Sprintf(`%s, "monthly_rate": %.2f`, 
				strings.TrimSuffix(instrument.Pricing, "}"), rate) + "}"
		}
	}

	if depositStr := getCellValue("deposit"); depositStr != "" {
		deposit, err := s.parseFloat(depositStr, "deposit", rowNum)
		if err != nil {
			return nil, err
		}
		if instrument.Pricing == "" {
			instrument.Pricing = fmt.Sprintf(`{"deposit": %.2f}`, deposit)
		} else {
			instrument.Pricing = fmt.Sprintf(`%s, "deposit": %.2f`, 
				strings.TrimSuffix(instrument.Pricing, "}"), deposit) + "}"
		}
	}

	if imagesStr := getCellValue("images"); imagesStr != "" {
		images := strings.Split(imagesStr, ",")
		imageList := "["
		for i, img := range images {
			img = strings.TrimSpace(img)
			if img != "" {
				if i > 0 {
					imageList += ", "
				}
				imageList += fmt.Sprintf(`"%s"`, img)
			}
		}
		imageList += "]"
		instrument.Images = imageList
	} else {
		instrument.Images = "[]"
	}

	return instrument, nil
}

func (s *InstrumentService) parseFloat(value, field string, rowNum int) (float64, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "¥", "")
	value = strings.ReplaceAll(value, "$", "")
	value = strings.ReplaceAll(value, ",", "")

	re := regexp.MustCompile(`^\d+(\.\d{1,2})?$`)
	if !re.MatchString(value) {
		return 0, fmt.Errorf("invalid %s format: %s (must be numeric)", field, value)
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %s", field, value)
	}

	if f < 0 {
		return 0, fmt.Errorf("%s cannot be negative: %f", field, f)
	}

	return f, nil
}

func (s *InstrumentService) sanitizeValue(value string) string {
	value = strings.TrimSpace(value)
	
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || 
	   strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") {
		return "'" + value
	}
	
	if len(value) > 1000 {
		return value[:1000]
	}
	
	return value
}

func (s *InstrumentService) ExportInstruments(opts ExportOptions, tenantID string) (*excelize.File, error) {
	f := excelize.NewFile()
	sheetName := "Instruments"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)

	query := s.db.Where("tenant_id = ?", tenantID)
	
	if opts.Category != "" {
		query = query.Where("category_name = ?", opts.Category)
	}
	if opts.Status != "" {
		query = query.Where("stock_status = ?", opts.Status)
	}
	if opts.SearchText != "" {
		query = query.Where("name ILIKE ? OR brand ILIKE ?", 
			"%"+opts.SearchText+"%", "%"+opts.SearchText+"%")
	}

	var instruments []models.Instrument
	if err := query.Find(&instruments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch instruments: %w", err)
	}

	if len(instruments) == 0 {
		return nil, fmt.Errorf("no instruments found with given filters")
	}

	if len(opts.Fields) == 0 {
		opts.Fields = []string{"name", "brand", "level", "daily_rate", "monthly_rate", "deposit", "stock_status"}
	}

	headers := make([]string, len(opts.Fields))
	for i, field := range opts.Fields {
		headers[i] = headerMap[field]
	}
	
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	for rowIdx, instrument := range instruments {
		rowNum := rowIdx + 2
		for colIdx, field := range opts.Fields {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowNum)
			value := s.getFieldValue(&instrument, field)
			f.SetCellValue(sheetName, cell, value)
		}
	}

	for i := range opts.Fields {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheetName, col, col, 15)
	}

	return f, nil
}

func (s *InstrumentService) getFieldValue(instrument *models.Instrument, field string) string {
	switch field {
	case "name":
		return s.sanitizeValue(instrument.Name)
	case "brand":
		return s.sanitizeValue(instrument.Brand)
	case "level":
		return s.sanitizeValue(instrument.Level)
	case "stock":
		return "0"
	case "status":
		return s.sanitizeValue(instrument.StockStatus)
	case "description":
		return s.sanitizeValue(instrument.Description)
	default:
		return ""
	}
}
