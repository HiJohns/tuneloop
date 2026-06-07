package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"time"
	"tuneloop-backend/models"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/joho/godotenv"
)

type contextKey string

const (
	TenantIDKey contextKey = "tenant_id"
	OrgIDKey    contextKey = "org_id"
	UserIDKey   contextKey = "user_id"
	RoleKey     contextKey = "role"
	IsOwnerKey  contextKey = "is_owner"
)

var dbInstance *gorm.DB

func init() {
	// 尝试多个可能的 .env 文件位置
	envPaths := []string{
		".env",       // 当前目录
		"../.env",    // 父目录（从 backend/ 运行时）
		"../../.env", // 更上级目录
	}

	for _, path := range envPaths {
		if err := godotenv.Load(path); err == nil {
			fmt.Printf("[DEBUG] Loaded .env from: %s\n", path)
			return
		}
	}

	// 如果都找不到，尝试默认位置（可能失败）
	godotenv.Load()
	fmt.Println("[WARNING] No .env file found in common paths")
}

type TenantScopedModel interface {
	SetTenantID(string)
	SetOrgID(string)
}

func registerTenantCallbacks(db *gorm.DB) {
	callback := db.Callback()

	callback.Create().Before("gorm:create").Register("tenant:before_create", setTenantIDFromContext)
	callback.Update().Before("gorm:update").Register("tenant:before_update", setTenantIDFromContext)
	callback.Query().Before("gorm:query").Register("tenant:before_query", addTenantScope)
	callback.Delete().Before("gorm:delete").Register("tenant:before_delete", addTenantScope)
}

func setTenantIDFromContext(db *gorm.DB) {
	if db.Statement.Error != nil || db.Statement.Context == nil {
		return
	}

	tenantID := GetTenantIDFromContext(db.Statement.Context)
	if tenantID == "" {
		return
	}

	if db.Statement.Schema != nil {
		if field := db.Statement.Schema.LookUpField("TenantID"); field != nil {
			if _, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); isZero {
				field.Set(db.Statement.Context, db.Statement.ReflectValue, tenantID)
			}
		}

		if field := db.Statement.Schema.LookUpField("OrgID"); field != nil {
			if orgID := GetOrgIDFromContext(db.Statement.Context); orgID != "" {
				if _, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); isZero {
					field.Set(db.Statement.Context, db.Statement.ReflectValue, orgID)
				}
			}
		}
	}
}

func addTenantScope(db *gorm.DB) {
	if db.Statement.Error != nil || db.Statement.Context == nil {
		return
	}

	tenantID := GetTenantIDFromContext(db.Statement.Context)
	if tenantID == "" {
		return
	}

	if db.Statement.Schema != nil {
		if field := db.Statement.Schema.LookUpField("TenantID"); field != nil {
			db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
				clause.Eq{Column: clause.Column{Table: db.Statement.Table, Name: "tenant_id"}, Value: tenantID},
			}})
		}
	}
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func LoadConfig() *Config {
	config := &Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     getEnv("POSTGRES_PORT", "5432"),
		User:     getEnv("POSTGRES_USER", "tuneloop"),
		Password: getEnv("POSTGRES_PASSWORD", ""),
		DBName:   getEnv("TUNELOOP_DB", "tuneloop"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	log.Printf("[DB] Configuration loaded: host=%s port=%s user=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.DBName, config.SSLMode)

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	log.Printf("[DB] Connecting: host=%s port=%s user=%s dbname=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.DBName)

	db, err := gorm.Open(gormPostgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Printf("[DB] Connected to database '%s'", cfg.DBName)

	registerTenantCallbacks(db)

	return db, nil
}

func SetTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

func SetOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, OrgIDKey, orgID)
}

func GetTenantIDFromContext(ctx context.Context) string {
	if tid, ok := ctx.Value(TenantIDKey).(string); ok {
		return tid
	}
	return ""
}

func GetOrgIDFromContext(ctx context.Context) string {
	if oid, ok := ctx.Value(OrgIDKey).(string); ok {
		return oid
	}
	return ""
}

func RunMigrations(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	migrationPath := os.Getenv("MIGRATION_PATH")
	if migrationPath == "" {
		migrationPath = "./database/migrations"
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationPath,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// RunMigrationsWithLogging runs database migrations with detailed logging
func RunMigrationsWithLogging(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	currentVersion, dirty, err := driver.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	fmt.Printf("Current database version: %d, Dirty: %v\n", currentVersion, dirty)

	m, err := migrate.NewWithDatabaseInstance(
		"file://database/migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if dirty {
		fmt.Printf("Warning: Database is in dirty state at version %d. Attempting to fix...\n", currentVersion)
		// Force the version to clear the dirty flag - this assumes the migration
		// partially succeeded and we want to retry from this version
		if err := m.Force(int(currentVersion)); err != nil {
			return fmt.Errorf("failed to force version %d: %w", currentVersion, err)
		}
		fmt.Printf("✓ Cleared dirty flag for version %d\n", currentVersion)
	}

	versionBefore, _, _ := driver.Version()

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			fmt.Println("✓ No new migrations to apply. Database is up to date.")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	versionAfter, _, _ := driver.Version()

	if versionAfter > versionBefore {
		fmt.Printf("✓ Successfully applied migrations: %d → %d\n", versionBefore, versionAfter)
	}

	return nil
}

// BootstrapDatabase ensures database is ready with all migrations applied
func BootstrapDatabase(db *gorm.DB) error {
	fmt.Println("Bootstrapping database...")

	if err := RunMigrationsWithLogging(db); err != nil {
		return fmt.Errorf("database bootstrap failed: %w", err)
	}

	// 验证关键表结构
	if err := validateDatabaseSchema(db); err != nil {
		return fmt.Errorf("database schema validation failed: %w", err)
	}

	fmt.Println("✓ Database bootstrap completed successfully")
	return nil
}

// modelTableName returns the table name for a given model struct.
// It uses GORM's naming strategy, except for models with a custom TableName() method.
func modelTableName(db *gorm.DB, instance interface{}) string {
	typ := reflect.TypeOf(instance)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	switch typ.Name() {
	case "OrderStatusHistory":
		return "order_status_history"
	case "AuditLog":
		return "audit_logs"
	default:
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(instance); err == nil && stmt.Schema != nil {
			return stmt.Schema.Table
		}
		return db.NamingStrategy.TableName(typ.Name())
	}
}

// validateModelColumns checks that all columns defined in the gorm tags
// of a model struct exist in the database table.
func validateModelColumns(db *gorm.DB, instance interface{}) error {
	typ := reflect.TypeOf(instance)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	tableName := modelTableName(db, instance)

	var tableExists int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?`, tableName).Scan(&tableExists).Error; err != nil {
		return fmt.Errorf("failed to check table %s: %w", tableName, err)
	}
	if tableExists == 0 {
		return nil
	}

	var expectedCols []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		gormTag := field.Tag.Get("gorm")
		if gormTag == "-" || strings.Contains(gormTag, "-:migration") {
			continue
		}

		// Skip GORM relationship fields (pointer to another model struct, e.g. *InstrumentLevel)
		if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct && field.Type.Elem() != reflect.TypeOf(time.Time{}) {
			continue
		}

		columnName := db.NamingStrategy.ColumnName("", field.Name)
		if gormTag != "" {
			for _, part := range strings.Split(gormTag, ";") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "column:") {
					columnName = strings.TrimPrefix(part, "column:")
					break
				}
			}
		}
		expectedCols = append(expectedCols, columnName)
	}

	if len(expectedCols) == 0 {
		return nil
	}

	var colCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name IN (?)`, tableName, expectedCols).Scan(&colCount).Error; err != nil {
		return fmt.Errorf("failed to check columns for table %s: %w", tableName, err)
	}

	if colCount != int64(len(expectedCols)) {
		for _, col := range expectedCols {
			var exists int64
			db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, tableName, col).Scan(&exists)
			if exists == 0 {
				return fmt.Errorf("schema validation failed: table %s missing column %s", tableName, col)
			}
		}
	}

	return nil
}

// validateDatabaseSchema checks that all model struct columns exist in the database.
func validateDatabaseSchema(db *gorm.DB) error {
	fmt.Println("Validating database schema...")

	modelsToValidate := []interface{}{
		&models.User{},
		&models.Category{},
		&models.Instrument{},
		&models.Order{},
		&models.Site{},
		&models.MaintenanceTicket{},
		&models.BrandConfig{},
		&models.OwnershipCertificate{},
		&models.Client{},
		&models.Tenant{},
		&models.Property{},
		&models.PropertyOption{},
		&models.InstrumentProperty{},
		&models.MaintenanceWorker{},
		&models.MaintenanceSession{},
		&models.MaintenanceSessionRecord{},
		&models.LeaseSession{},
		&models.ElectronicContract{},
		&models.DamageReport{},
		&models.DamageAssessment{},
		&models.Appeal{},
		&models.OrderStatusHistory{},
		&models.AuditLog{},
		&models.InstrumentPhotoBatch{},
		&models.InstrumentPhotoSpec{},
		&models.Merchant{},
		&models.SiteMember{},
		&models.Role{},
		&models.InstrumentMedia{},
		&models.SystemSetting{},
		&models.PricingTemplate{},
		&models.MerchantPricingConfig{},
	}

	for _, m := range modelsToValidate {
		if err := validateModelColumns(db, m); err != nil {
			return err
		}
	}

	fmt.Println("✓ Database schema validation passed")
	return nil
}

// CheckMigrationsStatus returns the current migration status without applying
func CheckMigrationsStatus(db *gorm.DB) (currentVersion uint, dirty bool, pendingCount int, err error) {
	sqlDB, err := db.DB()
	if err != nil {
		return 0, false, 0, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return 0, false, 0, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	versionInt, dirty, err := driver.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return 0, false, 0, fmt.Errorf("failed to get version: %w", err)
	}
	currentVersion = uint(versionInt)

	_, err = migrate.NewWithDatabaseInstance(
		"file://database/migrations",
		"postgres", driver)
	if err != nil {
		return currentVersion, dirty, 0, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return currentVersion, dirty, pendingCount, nil
}

func SetDB(db *gorm.DB) {
	dbInstance = db
}

func GetDB() *gorm.DB {
	if dbInstance == nil {
		panic("database not initialized, call InitDB first")
	}
	return dbInstance
}

func WithTenantScope(db *gorm.DB, tenantID string) *gorm.DB {
	if tenantID == "" {
		return db
	}
	return db.Where("tenant_id = ?", tenantID)
}

func WithTenantContext(db *gorm.DB, ctx context.Context) *gorm.DB {
	tenantID := GetTenantIDFromContext(ctx)
	if tenantID == "" {
		return db
	}
	return db.Where("tenant_id = ?", tenantID)
}
