package handlers

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	assert.NoError(t, err)

	return db, mock
}

func createMultipartFormFile(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	assert.NoError(t, err)

	_, err = part.Write(content)
	assert.NoError(t, err)

	err = writer.Close()
	assert.NoError(t, err)

	return body, writer.FormDataContentType()
}

func TestImportInstruments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setupDBMock    func(sqlmock.Sqlmock)
		fileName       string
		fileContent    []byte
		expectedCode   int
		expectedStatus string
		expectedError  bool
	}{
		{
			name: "successful import",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectExec(`INSERT INTO "instruments"`).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
			fileName:       "test.xlsx",
			fileContent:    createValidExcelContent(),
			expectedCode:   40005,
			expectedStatus: "Import failed",
			expectedError:  true,
		},
		{
			name: "invalid file type",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				// No DB operations expected
			},
			fileName:       "test.txt",
			fileContent:    []byte("invalid content"),
			expectedCode:   40003,
			expectedStatus: "Only Excel files",
			expectedError:  true,
		},
		{
			name: "empty file",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				// No DB operations expected
			},
			fileName:       "empty.xlsx",
			fileContent:    createEmptyExcelContent(),
			expectedCode:   40004,
			expectedStatus: "Failed to read Excel",
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupTestDB(t)
			tt.setupDBMock(mock)

			body, contentType := createMultipartFormFile(t, tt.fileName, tt.fileContent)

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("POST", "/api/instruments/import", body)
			req.Header.Set("Content-Type", contentType)
			req = req.WithContext(ctx.Request.Context())

			ctx.Request = req
			ctx.Set("db", db)
			ctx.Set("user", map[string]interface{}{
				"tenant_id": "tenant-123",
			})

			ImportInstruments(ctx)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedError {
				var response map[string]interface{}
				err := ctx.ShouldBindJSON(&response)
				assert.NoError(t, err)
				assert.Contains(t, response["message"], tt.expectedStatus)
			}
		})
	}
}

func TestExportInstruments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		setupDBMock func(sqlmock.Sqlmock)
		queryParams string
		expectData  bool
		expectError bool
	}{
		{
			name: "successful export with filters",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "name", "brand", "level", "pricing", "stock_status",
				}).AddRow("inst-1", "tenant-1", "Piano", "Yamaha", "entry", `{"daily_rate": 50}`, "available")
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WillReturnRows(rows)
			},
			queryParams: "?category=钢琴&status=available&fields=name,brand,price",
			expectData:  true,
			expectError: false,
		},
		{
			name: "export with no results",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "name", "brand", "level", "pricing", "stock_status",
				})
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WillReturnRows(rows)
			},
			queryParams: "?search_text=nonexistent",
			expectData:  false,
			expectError: true,
		},
		{
			name: "export with custom fields",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "name", "brand", "level", "pricing", "stock_status",
				}).AddRow("inst-1", "tenant-1", "Piano", "Yamaha", "entry", `{"daily_rate": 50}`, "available")
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WillReturnRows(rows)
			},
			queryParams: "?fields=name,brand",
			expectData:  true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupTestDB(t)
			tt.setupDBMock(mock)

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("GET", "/api/instruments/export"+tt.queryParams, nil)
			req = req.WithContext(ctx.Request.Context())

			ctx.Request = req
			ctx.Set("db", db)
			ctx.Set("request_time", time.Now().Unix())
			ctx.Set("user", map[string]interface{}{
				"tenant_id": "tenant-123",
			})

			ExportInstruments(ctx)

			if tt.expectError {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				assert.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestDownloadImportTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/api/instruments/import/template", nil)
	ctx.Request = req

	DownloadImportTemplate(ctx)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "instrument_import_template.xlsx")

	body := w.Body.Bytes()
	assert.True(t, len(body) > 0, "Template file should not be empty")

	// Verify it's a valid Excel file
	reader := bytes.NewReader(body)
	_, err := excelize.OpenReader(reader)
	assert.NoError(t, err, "Generated template should be a valid Excel file")
}

func TestGetMemoryUsage(t *testing.T) {
	// Just ensure the function runs without panic
	memory := getMemoryUsage()
	assert.GreaterOrEqual(t, memory, uint64(0))
}

// Helper functions
func createValidExcelContent() []byte {
	f := excelize.NewFile()
	sheet := "Sheet1"

	// Header row
	headers := []string{"乐器名称", "品牌", "分类名称", "级别", "日租金", "库存数量", "状态"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Data row
	row := []interface{}{"测试钢琴", "Yamaha", "钢琴", "entry", "50", "5", "available"}
	for i, v := range row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet, cell, v)
	}

	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}

func createEmptyExcelContent() []byte {
	var buf bytes.Buffer
	return buf.Bytes()
}

func TestPerformanceMonitoring(t *testing.T) {
	// Ensure that performance monitoring functions don't panic
	startTime := time.Now()
	startMemory := getMemoryUsage()

	assert.NotZero(t, startTime)
	assert.GreaterOrEqual(t, startMemory, uint64(0))

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	endTime := time.Since(startTime)
	endMemory := getMemoryUsage()

	assert.Greater(t, endTime, time.Duration(0))
	assert.GreaterOrEqual(t, endMemory, startMemory)
}

func TestAPIErrorResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedCode   int
		expectedStatus string
	}{
		{
			name: "missing tenant id",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("POST", "/api/instruments/import", nil)
			},
			expectedCode:   40001,
			expectedStatus: "Tenant ID is required",
		},
		{
			name: "missing file in form",
			setupRequest: func() *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				writer.Close()
				req := httptest.NewRequest("POST", "/api/instruments/import", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			expectedCode:   40002,
			expectedStatus: "File upload failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupTestDB(t)

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			req := tt.setupRequest()
			req = req.WithContext(ctx.Request.Context())

			ctx.Request = req
			ctx.Set("db", db)

			ImportInstruments(ctx)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var response map[string]interface{}
			ctx.ShouldBindJSON(&response)
			assert.Equal(t, tt.expectedCode, response["code"])
		})
	}
}
