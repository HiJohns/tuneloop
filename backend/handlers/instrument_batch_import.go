package handlers

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var importSessions = make(map[string]*ImportSession)

type ImportSession struct {
	ID          string
	TenantID    string
	Instruments []map[string]interface{}
	Images      map[string][]string
	CreatedAt   time.Time
}

type RowValidation struct {
	Row    int                    `json:"row"`
	SN     string                 `json:"sn"`
	Fields map[string]interface{} `json:"fields"`
	Errors []string               `json:"errors,omitempty"`
	Valid  bool                   `json:"valid"`
	Images []string               `json:"images,omitempty"`
}

// DownloadCSVTemplate generates a CSV template with dynamic property columns
// GET /api/instruments/import/template
func DownloadCSVTemplate(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	db := database.GetDB().WithContext(c.Request.Context())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	headers := []string{"识别码*", "分类名称*", "网点名称*", "级别名称*", "描述"}

	for _, prop := range properties {
		headers = append(headers, prop.Caption)
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"instrument_import_template.csv\"")

	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c.Writer)
	writer.Write(headers)

	sampleData := []string{"SN001", "钢琴", "北京旗舰店", "入门", "测试描述"}
	for range properties {
		sampleData = append(sampleData, "")
	}
	writer.Write(sampleData)

	notes := []string{
		"说明:",
		"- 识别码为必填项，且不可重复",
		"- 分类名称、网点名称、级别名称必须与系统中的名称完全一致",
		"- 带星号(*)的列为必填项",
		"- 动态属性列根据系统属性配置自动生成",
		"- 图片文件命名格式: 识别码_序号.jpg (如 SN001_1.jpg)",
	}
	for _, note := range notes {
		writer.Write([]string{note})
	}

	writer.Flush()
}

// PreviewBatchImport parses CSV file and validates rows
// POST /api/instruments/batch-import/preview
func PreviewBatchImport(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Tenant ID is required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "File upload failed: " + err.Error()})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Only CSV files (.csv) are supported"})
		return
	}

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "Failed to parse CSV: " + err.Error()})
		return
	}

	if len(records) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40005, "message": "CSV file is empty or has no data rows"})
		return
	}

	csvHeaders := records[0]
	if len(csvHeaders) > 0 {
		csvHeaders[0] = strings.TrimPrefix(csvHeaders[0], "\ufeff")
	}
	db := database.GetDB().WithContext(c.Request.Context())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)
	propNameMap := make(map[string]string)
	for _, prop := range properties {
		propNameMap[prop.Caption] = prop.Name
	}

	existingSNs := make(map[string]bool)
	var existingInstruments []models.Instrument
	db.Where("tenant_id = ?", tenantID).Select("sn").Find(&existingInstruments)
	for _, inst := range existingInstruments {
		if inst.SN != "" {
			existingSNs[inst.SN] = true
		}
	}

	seenSNs := make(map[string]int)
	var validations []RowValidation
	validCount := 0
	errorCount := 0

	for i, record := range records[1:] {
		rowNum := i + 2
		if isEmptyRow(record) {
			continue
		}

		fields := make(map[string]interface{})
		var errors []string
		sn := ""
		categoryName := ""
		siteName := ""
		levelName := ""

		for j, val := range record {
			if j >= len(csvHeaders) {
				break
			}
			val = strings.TrimSpace(val)
			headerLower := strings.TrimSpace(csvHeaders[j])

			switch headerLower {
			case "识别码*", "识别码", "sn", "serial_number":
				sn = val
				fields["sn"] = val
			case "分类名称*", "分类名称", "类别", "category_name":
				categoryName = val
				fields["category_name"] = val
			case "网点名称*", "网点名称", "存放网点", "site_name":
				siteName = val
				fields["site_name"] = val
			case "级别名称*", "级别名称", "等级", "level_name":
				levelName = val
				fields["level_name"] = val
			case "描述", "备注", "description":
				fields["description"] = val
			default:
				if val != "" {
					propName := propNameMap[headerLower]
					if propName != "" {
						fields["prop_"+propName] = val
					} else {
						fields[headerLower] = val
					}
				}
			}
		}

		if sn == "" {
			errors = append(errors, "识别码不能为空")
		} else if existingSNs[sn] {
			errors = append(errors, fmt.Sprintf("识别码 '%s' 已存在", sn))
		} else if prevRow, exists := seenSNs[sn]; exists {
			errors = append(errors, fmt.Sprintf("识别码 '%s' 与第 %d 行重复", sn, prevRow))
		} else {
			seenSNs[sn] = rowNum
		}

		if categoryName == "" {
			errors = append(errors, "分类名称不能为空")
		} else {
			catID, err := lookupCategoryID(categoryName, db, tenantID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("分类 '%s' 不存在", categoryName))
			} else {
				fields["category_id"] = catID
			}
		}

		if siteName == "" {
			errors = append(errors, "网点名称不能为空")
		} else {
			siteID, err := lookupSiteID(siteName, db, tenantID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("网点 '%s' 不存在", siteName))
			} else {
				fields["site_id"] = siteID
			}
		}

		if levelName != "" {
			levelID, err := lookupLevelID(levelName, db, tenantID)
			if err != nil {
				fields["_warning_level"] = fmt.Sprintf("级别 '%s' 不存在", levelName)
			} else {
				fields["level_id"] = levelID
			}
		}

		valid := len(errors) == 0
		if valid {
			validCount++
		} else {
			errorCount++
		}

		validations = append(validations, RowValidation{
			Row:    rowNum,
			SN:     sn,
			Fields: fields,
			Errors: errors,
			Valid:  valid,
		})
	}

	sessionID := uuid.New().String()
	importSessions[sessionID] = &ImportSession{
		ID:          sessionID,
		TenantID:    tenantID,
		Instruments: convertValidationsToMaps(validations),
		Images:      make(map[string][]string),
		CreatedAt:   time.Now(),
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"session_id":  sessionID,
			"total_count": len(validations),
			"valid_count": validCount,
			"error_count": errorCount,
			"rows":        validations,
			"can_import":  validCount > 0,
			"csv_headers": csvHeaders,
		},
	})
}

// UploadBatchMedia handles ZIP media upload and matches images to instruments
// POST /api/instruments/batch-import/media
func UploadBatchMedia(c *gin.Context) {
	sessionID := c.PostForm("session_id")
	session, exists := importSessions[sessionID]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Invalid or expired session"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "File upload failed: " + err.Error()})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Only ZIP files are supported"})
		return
	}

	tempDir := filepath.Join(os.TempDir(), "tuneloop-import", sessionID)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	zipReader, err := zip.NewReader(file.(io.ReaderAt), header.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "Failed to read ZIP: " + err.Error()})
		return
	}

	imageMap := make(map[string][]string)
	var unmatchedFiles []string
	var totalImages int

	snSet := make(map[string]bool)
	for _, inst := range session.Instruments {
		if sn, ok := inst["sn"].(string); ok && sn != "" {
			snSet[sn] = true
		}
	}

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() || !isImageFile(f.Name) {
			continue
		}

		totalImages++
		baseName := filepath.Base(f.Name)
		nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		parts := strings.SplitN(nameWithoutExt, "_", 2)
		sn := parts[0]

		if snSet[sn] {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			imgFileName := fmt.Sprintf("%s_%d%s", uuid.New().String()[:8], time.Now().UnixNano(), filepath.Ext(baseName))
			imgPath := filepath.Join("/uploads/batch", sessionID, imgFileName)

			destPath := filepath.Join(".", "uploads", "batch", sessionID)
			os.MkdirAll(destPath, 0755)

			destFile, err := os.Create(filepath.Join(destPath, imgFileName))
			if err != nil {
				rc.Close()
				continue
			}

			written, err := io.Copy(destFile, rc)
			rc.Close()
			destFile.Close()

			if err == nil && written > 0 {
				imageMap[sn] = append(imageMap[sn], imgPath)
			}
		} else {
			unmatchedFiles = append(unmatchedFiles, baseName)
		}
	}

	session.Images = imageMap

	matchedCount := 0
	for _, images := range imageMap {
		matchedCount += len(images)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"session_id":      sessionID,
			"total_images":    totalImages,
			"matched_count":   matchedCount,
			"unmatched_count": len(unmatchedFiles),
			"unmatched_files": unmatchedFiles,
			"matched_sn_list": getKeys(imageMap),
		},
	})
}

// ExecuteBatchImport creates instruments from validated preview data
// POST /api/instruments/batch-import
func ExecuteBatchImport(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "session_id is required"})
		return
	}

	session, exists := importSessions[req.SessionID]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "Invalid or expired session"})
		return
	}

	if time.Since(session.CreatedAt) > 30*time.Minute {
		delete(importSessions, req.SessionID)
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Session expired, please re-upload"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	type ImportResult struct {
		SN     string `json:"sn"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	var results []ImportResult
	successCount := 0
	failCount := 0

	for _, instData := range session.Instruments {
		sn, _ := instData["sn"].(string)
		result := ImportResult{SN: sn}

		err := db.Transaction(func(tx *gorm.DB) error {
			instrument := models.Instrument{
				TenantID:       session.TenantID,
				SN:             sn,
				StockStatus:    "available",
				Images:         "[]",
				Specifications: "{}",
				Pricing:        "{}",
			}

			if catID, ok := instData["category_id"].(string); ok && catID != "" {
				instrument.CategoryID = &catID
				var cat struct{ Name string }
				if err := tx.Table("categories").Where("id = ?", catID).Select("name").First(&cat).Error; err == nil {
					instrument.CategoryName = cat.Name
				}
			}

			if siteID, ok := instData["site_id"].(string); ok && siteID != "" {
				if siteUUID, err := uuid.Parse(siteID); err == nil {
					instrument.SiteID = &siteUUID
				}
			}

			if levelID, ok := instData["level_id"].(string); ok && levelID != "" {
				if parsedID, err := uuid.Parse(levelID); err == nil {
					instrument.LevelID = &parsedID
					var level models.InstrumentLevel
					if err := tx.Where("id = ?", levelID).First(&level).Error; err == nil {
						instrument.LevelName = level.Caption
					}
				}
			}

			if desc, ok := instData["description"].(string); ok {
				instrument.Description = desc
			}

			if images, exists := session.Images[sn]; exists && len(images) > 0 {
				imagesJSON, _ := json.Marshal(images)
				instrument.Images = string(imagesJSON)
			}

			props := make(map[string]interface{})
			for k, v := range instData {
				if strings.HasPrefix(k, "prop_") {
					propName := strings.TrimPrefix(k, "prop_")
					if val, ok := v.(string); ok && val != "" {
						props[propName] = []string{val}
					}
				}
			}

			if len(props) > 0 {
				propsJSON, _ := json.Marshal(props)
				instrument.Specifications = string(propsJSON)
			}

			if err := tx.Create(&instrument).Error; err != nil {
				return fmt.Errorf("failed to create instrument: %w", err)
			}

			if len(props) > 0 {
				if err := processProperties(tx, instrument.ID, session.TenantID, props); err != nil {
					log.Printf("[WARN] Properties processing failed for %s: %v", sn, err)
				}
			}

			return nil
		})

		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			failCount++
		} else {
			result.Status = "success"
			successCount++
		}

		results = append(results, result)
	}

	delete(importSessions, req.SessionID)

	cleanupDir := filepath.Join(".", "uploads", "batch", req.SessionID)
	os.RemoveAll(cleanupDir)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"total":         len(results),
			"success_count": successCount,
			"fail_count":    failCount,
			"results":       results,
		},
	})
}

func isEmptyRow(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func convertValidationsToMaps(validations []RowValidation) []map[string]interface{} {
	var result []map[string]interface{}
	for _, v := range validations {
		m := make(map[string]interface{})
		for k, val := range v.Fields {
			m[k] = val
		}
		m["sn"] = v.SN
		m["_row"] = v.Row
		m["_valid"] = v.Valid
		if len(v.Errors) > 0 {
			m["_errors"] = v.Errors
		}
		result = append(result, m)
	}
	return result
}

func getKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Cleanup expired import sessions periodically
func init() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			now := time.Now()
			for id, session := range importSessions {
				if now.Sub(session.CreatedAt) > 30*time.Minute {
					delete(importSessions, id)
					cleanupDir := filepath.Join(".", "uploads", "batch", id)
					os.RemoveAll(cleanupDir)
				}
			}
		}
	}()
}

// Legacy compatibility: BatchImportInstruments redirects to ExecuteBatchImport
func BatchImportInstruments(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Tenant ID is required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "File upload failed: " + err.Error()})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Only ZIP files (.zip) are supported"})
		return
	}

	zipReader, err := zip.NewReader(file.(io.ReaderAt), header.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "Failed to read ZIP: " + err.Error()})
		return
	}

	csvData, _, errors := parseZIPArchive(zipReader)
	if len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40005, "message": "ZIP parsing errors", "errors": errors})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	mappedInstruments := mapNamesToIDs(csvData, nil, db, tenantID)

	type ImportResult struct {
		SN     string `json:"sn"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	var results []ImportResult
	successCount := 0
	failCount := 0

	for _, instData := range mappedInstruments {
		sn, _ := instData["sn"].(string)
		result := ImportResult{SN: sn}

		if instData["_error_category"] != nil || instData["_error_site"] != nil {
			result.Status = "failed"
			errMsgs := []string{}
			if e, ok := instData["_error_category"].(string); ok {
				errMsgs = append(errMsgs, e)
			}
			if e, ok := instData["_error_site"].(string); ok {
				errMsgs = append(errMsgs, e)
			}
			result.Error = strings.Join(errMsgs, "; ")
			failCount++
			results = append(results, result)
			continue
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			instrument := models.Instrument{
				TenantID:       tenantID,
				SN:             sn,
				StockStatus:    "available",
				Images:         "[]",
				Specifications: "{}",
				Pricing:        "{}",
			}

			if catID, ok := instData["category_id"].(string); ok {
				instrument.CategoryID = &catID
				if catName, ok := instData["category_name"].(string); ok {
					instrument.CategoryName = catName
				}
			}
			if siteID, ok := instData["site_id"].(string); ok {
				if siteUUID, err := uuid.Parse(siteID); err == nil {
					instrument.SiteID = &siteUUID
				}
			}
			if levelID, ok := instData["level_id"].(string); ok {
				if parsedID, err := uuid.Parse(levelID); err == nil {
					instrument.LevelID = &parsedID
				}
			}
			if desc, ok := instData["description"].(string); ok {
				instrument.Description = desc
			}

			if err := tx.Create(&instrument).Error; err != nil {
				return err
			}

			if metadata, ok := instData["metadata"].(map[string]interface{}); ok && len(metadata) > 0 {
				propsJSON, _ := json.Marshal(metadata)
				instrument.Specifications = string(propsJSON)
				tx.Model(&instrument).Update("specifications", instrument.Specifications)
			}

			return nil
		})

		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			failCount++
		} else {
			result.Status = "success"
			successCount++
		}
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"total":         len(results),
			"success_count": successCount,
			"fail_count":    failCount,
			"results":       results,
		},
	})
}

func parseZIPArchive(zipReader *zip.Reader) ([]map[string]interface{}, []string, []string) {
	var csvData []map[string]interface{}
	var imageDirs []string
	var errors []string
	var csvFound bool

	imageDirMap := make(map[string]bool)

	for _, file := range zipReader.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			csvFound = true
			data, err := parseCSVFromZIP(file)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to parse CSV %s: %v", file.Name, err))
				continue
			}
			csvData = append(csvData, data...)
		}

		if isImageFile(file.Name) {
			dir := filepath.Dir(file.Name)
			if dir != "." {
				imageDirMap[dir] = true
			}
		}
	}

	for dir := range imageDirMap {
		imageDirs = append(imageDirs, dir)
	}

	if !csvFound {
		errors = append(errors, "No CSV file found in ZIP archive")
	}

	return csvData, imageDirs, errors
}

func parseCSVFromZIP(zipFile *zip.File) ([]map[string]interface{}, error) {
	csvReader, err := zipFile.Open()
	if err != nil {
		return nil, err
	}
	defer csvReader.Close()

	reader := csv.NewReader(csvReader)
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	var data []map[string]interface{}
	lineNum := 1

	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading line %d: %w", lineNum, err)
		}

		isEmpty := true
		for _, field := range record {
			if strings.TrimSpace(field) != "" {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			continue
		}

		row := make(map[string]interface{})
		metadata := make(map[string]interface{})

		for i, header := range headers {
			if i < len(record) {
				value := strings.TrimSpace(record[i])
				switch strings.ToLower(header) {
				case "sn", "识别码", "serial_number":
					row["sn"] = value
				case "category_name", "分类名称", "类别":
					row["category_name"] = value
				case "site_name", "网点名称", "存放网点":
					row["site_name"] = value
				case "level_name", "级别名称", "等级":
					row["level_name"] = value
				case "description", "描述", "备注":
					row["description"] = value
				default:
					if value != "" {
						metadata[header] = value
					}
				}
			}
		}

		if len(metadata) > 0 {
			row["metadata"] = metadata
		}

		data = append(data, row)
	}

	return data, nil
}

func lookupCategoryID(name string, db *gorm.DB, tenantID string) (string, error) {
	var category struct{ ID string }
	err := db.Table("categories").Where("name = ? AND tenant_id = ?", name, tenantID).Select("id").First(&category).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("category not found: %s", name)
		}
		return "", err
	}
	return category.ID, nil
}

func lookupSiteID(name string, db *gorm.DB, tenantID string) (string, error) {
	var site struct{ ID string }
	err := db.Table("sites").Where("name = ? AND tenant_id = ?", name, tenantID).Select("id").First(&site).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("site not found: %s", name)
		}
		return "", err
	}
	return site.ID, nil
}

func lookupLevelID(name string, db *gorm.DB, tenantID string) (string, error) {
	var level struct{ ID string }
	err := db.Table("instrument_levels").Where("caption = ? OR code = ?", name, name).Select("id").First(&level).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("level not found: %s", name)
		}
		return "", err
	}
	return level.ID, nil
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, imgExt := range []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"} {
		if ext == imgExt {
			return true
		}
	}
	return false
}

func mapNamesToIDs(instruments []map[string]interface{}, imageDirs []string, db *gorm.DB, tenantID string) []map[string]interface{} {
	var mapped []map[string]interface{}
	for _, instrument := range instruments {
		mappedInst := make(map[string]interface{})
		for k, v := range instrument {
			mappedInst[k] = v
		}

		if categoryName, ok := instrument["category_name"].(string); ok && categoryName != "" {
			categoryID, err := lookupCategoryID(categoryName, db, tenantID)
			if err != nil {
				mappedInst["_error_category"] = fmt.Sprintf("分类不存在: %s", categoryName)
			} else {
				mappedInst["category_id"] = categoryID
			}
		}

		if siteName, ok := instrument["site_name"].(string); ok && siteName != "" {
			siteID, err := lookupSiteID(siteName, db, tenantID)
			if err != nil {
				mappedInst["_error_site"] = fmt.Sprintf("网点不存在: %s", siteName)
			} else {
				mappedInst["site_id"] = siteID
			}
		}

		if levelName, ok := instrument["level_name"].(string); ok && levelName != "" {
			levelID, err := lookupLevelID(levelName, db, tenantID)
			if err != nil {
				mappedInst["_warning_level"] = fmt.Sprintf("级别不存在: %s", levelName)
			} else {
				mappedInst["level_id"] = levelID
			}
		}

		mapped = append(mapped, mappedInst)
	}
	return mapped
}
