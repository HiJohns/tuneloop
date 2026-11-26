package service

import (
	"fmt"
	"regexp"
	"sort"
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

// #2579：headerMap 只保留**有数据源**的字段（D3）。brand / model 的模型字段已删除，
// monthly_rate 定价链不产出，stock 无库存概念 —— 导出它们只会得到空列/恒 0 列。
var headerMap = map[string]string{
	"sn":                "识别码",
	"category_name":     "分类名称",
	"level":             "级别",
	"daily_rate":        "日租金",
	"deposit":           "押金",
	"overdue_daily_fee": "逾期租金",
	"status":            "状态",
	"description":       "描述",
	"images":            "图片URL",
}

var requiredFields = []string{"category_name"}

// #2579 D10：导出字段白名单 = headerMap 的键（**有数据源**的字段）。handler 用它在导出前拒绝未知字段，
// 不得以 200 + 空列掩盖（如已移除的 brand/model/monthly_rate/stock）。
func ExportFieldNames() []string {
	names := make([]string, 0, len(headerMap))
	for name := range headerMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsExportField 判断字段是否在导出白名单内（订单无关，纯字段契约）。
func IsExportField(field string) bool {
	_, ok := headerMap[field]
	return ok
}

// priceExportFields 是需要 instrument:delete 才能导出的价格列（#2579 D9：导出字段 ⊆ 列表可见字段）。
var priceExportFields = []string{"daily_rate", "deposit", "overdue_daily_fee"}

// IsPriceExportField 判断字段是否为价格列。
func IsPriceExportField(field string) bool {
	for _, f := range priceExportFields {
		if f == field {
			return true
		}
	}
	return false
}

// TrimPriceExportFields 返回去掉价格列后的字段列表（保持原顺序）；调用方据权限决定是否裁剪。
func TrimPriceExportFields(fields []string) []string {
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		if IsPriceExportField(f) {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}


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
	Total   int           `json:"total"`
	Success int           `json:"success"`
	Failed  int           `json:"failed"`
	Errors  []ImportError `json:"errors"`
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

	// #2578：导出与列表同一套过滤语义（D1：可见性由 handler 注入闭包，
	// service 不复制权限逻辑）。
	SN            string                  `json:"sn"`             // 识别码模糊匹配（与 GET /api/instruments?sn= 同口径）
	CategoryIDs   []string                `json:"category_ids"`   // 已展开后代分类的 ID 集合（handler 用 getDescendantCategoryIDs）
	ExcludeStatus []string                `json:"exclude_status"` // stock_status NOT IN（csv）
	Scope         func(*gorm.DB) *gorm.DB `json:"-"`              // 可见性作用域闭包（applyInstrumentListScope）

	// #2579：金额字段的唯一数据源由 handler 注入（instrumentListPriceFields 同链，
	// D1：导出侧禁止自行解析 pricing / pricing_overrides JSONB）。
	// 返回 map 的键为 daily_rate / deposit / overdue_daily_fee（已格式化为「元，2 位小数」）；
	// 缺键表示无法解析 → 导出显示 `-`。nil 时所有金额列输出 `-`（不 panic）。
	MoneyFields func(*models.Instrument) map[string]string `json:"-"`
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

		// 乐器唯一标识为 SN（识别码），重复检查基于 SN
		var existing models.Instrument
		dupQuery := tx.Where("tenant_id = ? AND sn = ?", tenantID, instrument.SN)
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

		categoryID, err := s.resolveCategory(instrument, tenantID, tx)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:   rowNum,
				Error: fmt.Sprintf("分类解析失败: %v", err),
			})
			tx.Rollback()
			tx = s.db.Begin()
			continue
		}
		instrument.CategoryID = &categoryID

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

	// 乐器唯一标识为 SN（识别码），创建时必须提供
	instrument.SN = getCellValue("sn")
	if instrument.SN == "" {
		return nil, fmt.Errorf("sn (识别码) is required")
	}

	// Brand 字段已删除 - 遗留字段不再支持
	// instrument.Brand = getCellValue("brand")
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

	// #2578：顺序固定 —— 租户 → 可见性 → sn → 后代分类 → 状态 → 排除状态 → 既有参数。
	if opts.Scope != nil {
		query = opts.Scope(query)
	}
	if opts.SN != "" {
		query = query.Where("sn ILIKE ?", "%"+opts.SN+"%")
	}
	if len(opts.CategoryIDs) > 0 {
		query = query.Where("category_id IN ?", opts.CategoryIDs)
	}
	if opts.Status != "" {
		query = query.Where("stock_status = ?", opts.Status)
	}
	if len(opts.ExcludeStatus) > 0 {
		query = query.Where("stock_status NOT IN ?", opts.ExcludeStatus)
	}
	if opts.Category != "" {
		query = query.Where("category_name = ?", opts.Category)
	}
	if opts.SearchText != "" {
		query = query.Where("name ILIKE ? OR brand ILIKE ?",
			"%"+opts.SearchText+"%", "%"+opts.SearchText+"%")
	}

	var instruments []models.Instrument
	// #2579 D4：level 取 InstrumentLevel.Caption，需要预加载关联。
	if err := query.Preload("InstrumentLevel").Find(&instruments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch instruments: %w", err)
	}

	if len(instruments) == 0 {
		return nil, fmt.Errorf("no instruments found with given filters")
	}

	if len(opts.Fields) == 0 {
		// #2579 D6：默认字段集改为有数据源的识别码/分类/状态（原默认集全是死字段）。
		opts.Fields = []string{"sn", "category_name", "status"}
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
			value := s.getFieldValue(&instrument, field, opts)
			f.SetCellValue(sheetName, cell, value)
		}
	}

	for i := range opts.Fields {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheetName, col, col, 15)
	}

	return f, nil
}

func (s *InstrumentService) resolveCategory(instrument *models.Instrument, tenantID string, tx *gorm.DB) (string, error) {
	categoryName := ""
	for field, header := range headerMap {
		if field == "category_name" {
			categoryName = header
			break
		}
	}

	if categoryName == "" {
		categoryName = "未分类"
	} else {
		var category models.Category
		if err := tx.Where("tenant_id = ? AND name ILIKE ?", tenantID, categoryName).First(&category).Error; err != nil {
			categoryName = "未分类"
		} else {
			return category.ID, nil
		}
	}

	var defaultCategory models.Category
	if err := tx.Where("tenant_id = ? AND name = ?", tenantID, categoryName).First(&defaultCategory).Error; err != nil {
		defaultCategory = models.Category{
			TenantID: tenantID,
			Name:     categoryName,
		}
		if err := tx.Create(&defaultCategory).Error; err != nil {
			return "", fmt.Errorf("failed to create default category: %w", err)
		}
	}

	return defaultCategory.ID, nil
}

// FormatCentsYuan 把「分」格式化为「元，2 位小数」（#2579 D2）。
// 导出与测试共用同一格式化口径，避免导出侧各自实现。
func FormatCentsYuan(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func (s *InstrumentService) getFieldValue(instrument *models.Instrument, field string, opts ExportOptions) string {
	switch field {
	case "sn":
		return s.sanitizeValue(instrument.SN)
	case "level":
		// D4：优先 InstrumentLevel.Caption（LevelID 关联），否则回落受弃用字符串，都没有则 `-`。
		if instrument.InstrumentLevel != nil && strings.TrimSpace(instrument.InstrumentLevel.Caption) != "" {
			return s.sanitizeValue(instrument.InstrumentLevel.Caption)
		}
		if v := strings.TrimSpace(instrument.Level); v != "" {
			return s.sanitizeValue(v)
		}
		return "-"
	case "status":
		return s.sanitizeValue(instrument.StockStatus)
	case "category_name":
		return s.sanitizeValue(instrument.CategoryName)
	case "description":
		return s.sanitizeValue(instrument.Description)
	case "images":
		return s.sanitizeValue(instrument.Images)
	case "daily_rate", "deposit", "overdue_daily_fee":
		// D1/D2：金额只经注入的单一链取值；缺键/不可解析 → `-`（不得用 0 冒充缺失）。
		if opts.MoneyFields == nil {
			return "-"
		}
		values := opts.MoneyFields(instrument)
		if v, ok := values[field]; ok && strings.TrimSpace(v) != "" {
			return v
		}
		return "-"
	default:
		return ""
	}
}
