package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
)

func TestUpdateInstrument(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		instrumentID  string
		setupDBMock   func(sqlmock.Sqlmock)
		requestBody   map[string]interface{}
		expectedCode  int
		expectedError string
	}{
		{
			name:         "instrument not found",
			instrumentID: "inst-999",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
					"model", "description", "images", "video", "specifications", "pricing",
					"stock_status", "created_at", "updated_at",
				})
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WithArgs("inst-999", "tenant-001", 1).
					WillReturnRows(rows)
			},
			requestBody: map[string]interface{}{
				"name": "Test",
			},
			expectedCode:  404,
			expectedError: "instrument not found",
		},
		{
			name:         "missing required field name returns validation error",
			instrumentID: "inst-004",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
					"model", "description", "images", "video", "specifications", "pricing",
					"stock_status", "created_at", "updated_at",
				}).AddRow(
					"inst-004", "tenant-001", "org-001", "cat-001", "Piano", "Yamaha", "beginner",
					"U1", "Description", "[]", "", `[]`, "{}",
					"available", time.Now(), time.Now(),
				)
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WithArgs("inst-004", "tenant-001", 1).
					WillReturnRows(rows)
			},
			requestBody: map[string]interface{}{
				"level":       "beginner",
				"category_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedCode:  400,
			expectedError: "Name",
		},
		{
			name:         "missing required field level returns validation error",
			instrumentID: "inst-005",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
					"model", "description", "images", "video", "specifications", "pricing",
					"stock_status", "created_at", "updated_at",
				}).AddRow(
					"inst-005", "tenant-001", "org-001", "cat-001", "Guitar", "Taylor", "intermediate",
					"A2", "Description", "[]", "", `[]`, "{}",
					"available", time.Now(), time.Now(),
				)
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WithArgs("inst-005", "tenant-001", 1).
					WillReturnRows(rows)
			},
			requestBody: map[string]interface{}{
				"name":        "Test Instrument",
				"category_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedCode:  400,
			expectedError: "Level",
		},
		{
			name:         "missing required field category_id returns validation error",
			instrumentID: "inst-006",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
					"model", "description", "images", "video", "specifications", "pricing",
					"stock_status", "created_at", "updated_at",
				}).AddRow(
					"inst-006", "tenant-001", "org-001", "cat-001", "Violin", "Suzuki", "professional",
					"V1", "Description", "[]", "", `[]`, "{}",
					"available", time.Now(), time.Now(),
				)
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WithArgs("inst-006", "tenant-001", 1).
					WillReturnRows(rows)
			},
			requestBody: map[string]interface{}{
				"name":  "Test Instrument",
				"level": "beginner",
			},
			expectedCode:  400,
			expectedError: "CategoryID",
		},
		{
			name:         "successful update with specifications",
			instrumentID: "inst-001",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
					"model", "description", "images", "video", "specifications", "pricing",
					"stock_status", "created_at", "updated_at",
				}).AddRow(
					"inst-001", "tenant-001", "org-001", "550e8400-e29b-41d4-a716-446655440000", "Piano", "Yamaha", "beginner",
					"U1", "Test description", "[]", "", `[{"name":"standard","daily_rent":100,"monthly_rent":2500,"deposit":5000,"stock":5}]`, "{}",
					"available", time.Now(), time.Now(),
				)
				mock.ExpectQuery(`SELECT \* FROM "instruments"`).
					WithArgs("inst-001", "tenant-001", 1).
					WillReturnRows(rows)

				catRows := sqlmock.NewRows([]string{"name"}).AddRow("Piano")
				mock.ExpectQuery(`SELECT "name" FROM "categories"`).
					WithArgs("550e8400-e29b-41d4-a716-446655440000", 1).
					WillReturnRows(catRows)

				mock.ExpectBegin()
				mock.ExpectExec(`UPDATE "instruments"`).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
			requestBody: map[string]interface{}{
				"name":        "Piano Updated",
				"brand":       "Yamaha",
				"level":       "beginner",
				"model":       "U1",
				"category_id": "550e8400-e29b-41d4-a716-446655440000",
				"description": "Test description",
				"images":      []string{},
				"video":       "",
				"specifications": []map[string]interface{}{
					{
						"name":         "standard",
						"daily_rent":   100.0,
						"weekly_rent":  600.0,
						"monthly_rent": 2500.0,
						"deposit":      5000.0,
						"stock":        5,
					},
				},
			},
			expectedCode:  200,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			assert.NoError(t, err)
			defer sqlDB.Close()

			dialector := postgres.New(postgres.Config{
				Conn:       sqlDB,
				DriverName: "postgres",
			})

			db, err := gorm.Open(dialector, &gorm.Config{})
			assert.NoError(t, err)

			database.SetDB(db)

			if tt.setupDBMock != nil {
				tt.setupDBMock(mock)
			}

			bodyBytes, err := json.Marshal(tt.requestBody)
			assert.NoError(t, err)

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			ctx.Params = gin.Params{{Key: "id", Value: tt.instrumentID}}
			ctx.Request = httptest.NewRequest("PUT", "/api/instruments/"+tt.instrumentID, bytes.NewReader(bodyBytes))
			ctx.Request.Header.Set("Content-Type", "application/json")

			tenantCtx := context.WithValue(context.Background(), middleware.ContextKeyTenantID, "tenant-001")
			ctx.Request = ctx.Request.WithContext(tenantCtx)

			UpdateInstrument(ctx)

			assert.Equal(t, tt.expectedCode, w.Code, "Expected status code %d, got %d. Body: %s", tt.expectedCode, w.Code, w.Body.String())

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				message, ok := response["message"].(string)
				assert.True(t, ok, "Expected response to have message field")
				assert.Contains(t, message, tt.expectedError, "Expected error message to contain '%s', got '%s'", tt.expectedError, message)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpdateInstrumentPricingField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer sqlDB.Close()

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	assert.NoError(t, err)

	database.SetDB(db)

	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "org_id", "category_id", "name", "brand", "level",
		"model", "description", "images", "video", "specifications", "pricing",
		"stock_status", "created_at", "updated_at",
	}).AddRow(
		"inst-100", "tenant-001", "org-001", "550e8400-e29b-41d4-a716-446655440000", "Flute", "Yamaha", "advanced",
		"YFL-221", "Professional flute", "[]", "", `[]`, "{}",
		"available", time.Now(), time.Now(),
	)
	mock.ExpectQuery(`SELECT \* FROM "instruments"`).
		WithArgs("inst-100", "tenant-001", 1).
		WillReturnRows(rows)

	catRows := sqlmock.NewRows([]string{"name"}).AddRow("Flute")
	mock.ExpectQuery(`SELECT "name" FROM "categories"`).
		WithArgs("550e8400-e29b-41d4-a716-446655440000", 1).
		WillReturnRows(catRows)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "instruments"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	requestBody := map[string]interface{}{
		"name":        "Flute Updated",
		"brand":       "Yamaha",
		"level":       "advanced",
		"model":       "YFL-221",
		"category_id": "550e8400-e29b-41d4-a716-446655440000",
		"description": "Professional flute",
		"specifications": []map[string]interface{}{
			{
				"name":         "silver-plated",
				"daily_rent":   150.0,
				"weekly_rent":  800.0,
				"monthly_rent": 3500.0,
				"deposit":      10000.0,
				"stock":        2,
			},
		},
		"pricing": map[string]interface{}{
			"discount_days": 7,
			"discount_rate": 0.9,
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Params = gin.Params{{Key: "id", Value: "inst-100"}}
	ctx.Request = httptest.NewRequest("PUT", "/api/instruments/inst-100", bytes.NewReader(bodyBytes))
	ctx.Request.Header.Set("Content-Type", "application/json")

	tenantCtx := context.WithValue(context.Background(), middleware.ContextKeyTenantID, "tenant-001")
	ctx.Request = ctx.Request.WithContext(tenantCtx)

	UpdateInstrument(ctx)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200, got %d. Body: %s", w.Code, w.Body.String())

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(20000), response["code"])

	assert.NoError(t, mock.ExpectationsWereMet())
}
