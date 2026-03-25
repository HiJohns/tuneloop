package database

import (
	"context"
	"fmt"
	"os"
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
	godotenv.Load()
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
	return &Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     getEnv("POSTGRES_PORT", "5432"),
		User:     getEnv("POSTGRES_USER", "tuneloop"),
		Password: getEnv("POSTGRES_PASSWORD", ""),
		DBName:   getEnv("TUNELOOP_DB", "tuneloop"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
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

	db, err := gorm.Open(gormPostgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

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

	m, err := migrate.NewWithDatabaseInstance(
		"file://database/migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Instrument{},
		&models.Order{},
		&models.Site{},
		&models.MaintenanceTicket{},
		&models.BrandConfig{},
		&models.OwnershipCertificate{},
		&models.Client{},
	)
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
