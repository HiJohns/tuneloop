package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
)

func TestCreateCategory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		requestBody   map[string]interface{}
		setupDBMock   func(sqlmock.Sqlmock)
		expectedCode  float64 // JSON unmarshal converts numbers to float64
		expectedError string
		checkData     bool
		expectedSort  int
	}{
		{
			name: "successfully create category with numeric sort",
			requestBody: map[string]interface{}{
				"name":    "钢琴",
				"icon":    "🎹",
				"sort":    10,
				"visible": true,
			},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				// Use flexible argument matching
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "categories"`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("cat-123"))
				mock.ExpectCommit()
			},
			expectedCode: 20100.0,
			checkData:    true,
			expectedSort: 10,
		},
		{
			name: "sort as string should fail - simulates the bug",
			requestBody: map[string]interface{}{
				"name":    "吉他",
				"sort":    "20", // String instead of int
				"visible": true,
			},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				// Should not reach DB
			},
			expectedCode:  40001.0,
			expectedError: "json: cannot unmarshal string",
			checkData:     false,
		},
		{
			name: "missing required name field",
			requestBody: map[string]interface{}{
				"sort":    5,
				"visible": true,
			},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				// Should not reach DB
			},
			expectedCode:  40001.0,
			expectedError: "required",
			checkData:     false,
		},
		{
			name: "default values - sort and visible",
			requestBody: map[string]interface{}{
				"name": "小提琴",
			},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "categories"`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("cat-456"))
				mock.ExpectCommit()
			},
			expectedCode: 20100.0,
			checkData:    true,
			expectedSort: 0, // Default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock database
			db, mock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			gormDB, err := gorm.Open(postgres.New(postgres.Config{
				Conn: db,
			}), &gorm.Config{})
			assert.NoError(t, err)

			// Set the global DB
			database.SetDB(gormDB)

			// Setup mock
			tt.setupDBMock(mock)

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/categories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// Create context with tenant ID
			ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-001")
			req = req.WithContext(ctx)

			// Create response recorder
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Call handler
			CreateCategory(c)

			// Check response
			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedCode, response["code"])

			if tt.expectedError != "" {
				assert.Contains(t, response["message"], tt.expectedError)
			}

			if tt.checkData {
				data, ok := response["data"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, tt.requestBody["name"], data["name"])
				if sort, exists := tt.requestBody["sort"]; exists && sort != nil {
					// JSON unmarshal converts numbers to float64
					expectedFloat := float64(tt.expectedSort)
					assert.Equal(t, expectedFloat, data["sort"])
				}
			}

			// Verify all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("there were unfulfilled expectations: %s", err)
			}
		})
	}
}
