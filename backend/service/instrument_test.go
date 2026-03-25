package service

import (
	"testing"
	"tuneloop-backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open GORM connection: %v", err)
	}

	return db, mock
}

func TestParseFloat(t *testing.T) {
	service := &InstrumentService{}

	tests := []struct {
		name    string
		value   string
		field   string
		rowNum  int
		want    float64
		wantErr bool
	}{
		{
			name:    "valid integer",
			value:   "100",
			field:   "daily_rate",
			rowNum:  1,
			want:    100.0,
			wantErr: false,
		},
		{
			name:    "valid decimal",
			value:   "99.99",
			field:   "monthly_rate",
			rowNum:  2,
			want:    99.99,
			wantErr: false,
		},
		{
			name:    "with currency symbol",
			value:   "¥199.50",
			field:   "deposit",
			rowNum:  3,
			want:    199.5,
			wantErr: false,
		},
		{
			name:    "with comma",
			value:   "1,234.56",
			field:   "daily_rate",
			rowNum:  4,
			want:    1234.56,
			wantErr: false,
		},
		{
			name:    "negative number",
			value:   "-50",
			field:   "daily_rate",
			rowNum:  5,
			wantErr: true,
		},
		{
			name:    "invalid format",
			value:   "abc123",
			field:   "deposit",
			rowNum:  6,
			wantErr: true,
		},
		{
			name:    "too many decimals",
			value:   "10.123",
			field:   "monthly_rate",
			rowNum:  7,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.parseFloat(tt.value, tt.field, tt.rowNum)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeValue(t *testing.T) {
	service := &InstrumentService{}

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "normal text",
			value: "Piano",
			want:  "Piano",
		},
		{
			name:  "Excel formula with equals",
			value: "=SUM(A1:A10)",
			want:  "'=SUM(A1:A10)",
		},
		{
			name:  "Excel formula with plus",
			value: "+CMD|'/c calc'!A0",
			want:  "'+CMD|'/c calc'!A0",
		},
		{
			name:  "Excel formula with minus",
			value: "-2+3",
			want:  "'-2+3",
		},
		{
			name:  "Excel formula with at",
			value: "@SUM(A1)",
			want:  "'@SUM(A1)",
		},
		{
			name:  "very long text",
			value: string(make([]byte, 1500)),
			want:  string(make([]byte, 1000)),
		},
		{
			name:  "whitespace trimming",
			value: "  Piano  ",
			want:  "Piano",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.sanitizeValue(tt.value)
			if got != tt.want {
				t.Errorf("sanitizeValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveCategory(t *testing.T) {
	tests := []struct {
		name         string
		setupMock    func(sqlmock.Sqlmock)
		instrument   *models.Instrument
		wantCategory string
		wantErr      bool
	}{
		{
			name: "exact match found",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "tenant_id", "name"}).
					AddRow("cat-123", "tenant-1", "Piano")
				mock.ExpectQuery(`SELECT \* FROM "categories"`).
					WillReturnRows(rows)
			},
			instrument:   &models.Instrument{Name: "Test"},
			wantCategory: "cat-123",
			wantErr:      false,
		},
		{
			name: "fuzzy match found",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "tenant_id", "name"}).
					AddRow("cat-456", "tenant-1", "Grand Piano")
				mock.ExpectQuery(`SELECT \* FROM "categories"`).
					WillReturnRows(rows)
			},
			instrument:   &models.Instrument{Name: "Test"},
			wantCategory: "cat-456",
			wantErr:      false,
		},
		{
			name: "no match - use default category",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT \* FROM "categories"`).
					WillReturnError(gorm.ErrRecordNotFound)

				mock.ExpectQuery(`SELECT \* FROM "categories"`).
					WillReturnError(gorm.ErrRecordNotFound)

				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "categories"`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("cat-new"))
				mock.ExpectCommit()
			},
			instrument:   &models.Instrument{Name: "Test"},
			wantCategory: "cat-new",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			service := NewInstrumentService(db)
			tx := db.Begin()

			tt.setupMock(mock)

			got, err := service.resolveCategory(tt.instrument, "tenant-1", tx)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveCategory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantCategory {
				t.Errorf("resolveCategory() = %v, want %v", got, tt.wantCategory)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestValidateRequiredColumns(t *testing.T) {
	service := &InstrumentService{}

	tests := []struct {
		name        string
		columnIndex map[string]int
		wantErr     bool
	}{
		{
			name: "all required columns present",
			columnIndex: map[string]int{
				"name":          0,
				"category_name": 1,
			},
			wantErr: false,
		},
		{
			name: "missing name column",
			columnIndex: map[string]int{
				"category_name": 1,
			},
			wantErr: true,
		},
		{
			name: "missing category_name column",
			columnIndex: map[string]int{
				"name": 0,
			},
			wantErr: true,
		},
		{
			name:        "both columns missing",
			columnIndex: map[string]int{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateRequiredColumns(tt.columnIndex)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequiredColumns() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseHeaderRow(t *testing.T) {
	service := &InstrumentService{}

	tests := []struct {
		name      string
		headerRow []string
		want      map[string]int
	}{
		{
			name:      "english headers",
			headerRow: []string{"name", "brand", "category_name"},
			want: map[string]int{
				"name":          0,
				"brand":         1,
				"category_name": 2,
			},
		},
		{
			name:      "chinese headers",
			headerRow: []string{"乐器名称", "品牌", "分类名称"},
			want: map[string]int{
				"name":          0,
				"brand":         1,
				"category_name": 2,
			},
		},
		{
			name:      "mixed headers",
			headerRow: []string{"name", "品牌", "category_name"},
			want: map[string]int{
				"name":          0,
				"brand":         1,
				"category_name": 2,
			},
		},
		{
			name:      "unknown headers",
			headerRow: []string{"unknown1", "unknown2"},
			want:      map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.parseHeaderRow(tt.headerRow)
			if len(got) != len(tt.want) {
				t.Errorf("parseHeaderRow() returned %d fields, want %d", len(got), len(tt.want))
			}
			for field, idx := range tt.want {
				if got[field] != idx {
					t.Errorf("parseHeaderRow() field %s = %d, want %d", field, got[field], idx)
				}
			}
		})
	}
}
