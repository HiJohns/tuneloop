package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
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
// migrationsDir is the on-disk migration source directory (relative to the
// service working directory).
const migrationsDir = "database/migrations"

// maxLocalMigrationVersion scans the migration directory and returns the
// highest numeric filename prefix. Supports legacy 3-digit names
// (031_add_photo_tables.up.sql) and timestamp names (20260914001_...).
// Empty directory or no match -> (0, nil).
func maxLocalMigrationVersion(dir string) (uint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}
	var max uint
	re := regexp.MustCompile(`^([0-9]+)_`)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		if uint(n) > max {
			max = uint(n)
		}
	}
	return max, nil
}

// checkPackageFreshness aborts when the database schema version is ahead of
// the newest migration shipped in this package (#1913 停机根因 / #1929 防御)。
func checkPackageFreshness(dbVersion, localMax uint) error {
	if dbVersion > localMax {
		return fmt.Errorf("database schema is AHEAD of this build (db=%d, package max=%d) — this package is outdated; deploy a package containing migration %d or later", dbVersion, localMax, dbVersion)
	}
	return nil
}

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

	// #1929: 包过期防御 —— DB schema 超前于本包迁移目录时，在 m.Up() 前显式报错
	// （避免 golang-migrate 抛出的晦涩错误，并明确告知"包过期"）。
	localMax, err := maxLocalMigrationVersion(migrationsDir)
	if err != nil {
		return fmt.Errorf("package freshness check failed: %w", err)
	}
	if err := checkPackageFreshness(uint(currentVersion), localMax); err != nil {
		return err
	}

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

	// #1986：迁移记录 ↔ 实表工件交叉校验（记录已应用但工件缺失 → fail-fast）。
	if err := verifyMigrationArtifacts(db); err != nil {
		return fmt.Errorf("migration artifact verification failed: %w", err)
	}
	fmt.Println("✓ Migration artifact verification passed")

	fmt.Println("✓ Database bootstrap completed successfully")
	return nil
}

// migrationArtifact 断言：当 schema_migrations 版本 ≥ Version 时，Table 必须存在
// （Column 非空时还要求该列存在）。用于捕获「迁移记录已应用但实表工件缺失」的快照
// 库不一致（#1986/#1913；如预生产 orders.order_no 曾缺失）。
type migrationArtifact struct {
	Version uint
	Table   string
	Column  string
	Desc    string
}

// defaultMigrationArtifacts 关键工件清单；新增迁移时必须同步登记（AGENTS.md 迁移纪律）。
var defaultMigrationArtifacts = []migrationArtifact{
	{20260917007, "orders", "order_no", "业务订单号列"},
	{20260917008, "technician_profiles", "", "维修师傅档案表"},
	{20260917009, "invoice_applications", "invoice_type", "发票类型列"},
	{20260917009, "invoice_applications", "title", "发票抬头列"},
	{20260917009, "invoice_applications", "tax_number", "税号列"},
	{20260917010, "point_batches", "", "乐币批次表"},
	{20260917010, "point_batch_consumptions", "", "批次扣减留痕表"},
	{20260917011, "gift_policies", "referral_ratio", "裂变比例列"},
	{20260917011, "gift_policies", "referral_reg_points", "邀请奖乐币列"},
	{20260921001, "orders", "delivery_address", "收货地址列（并入 orders，#2010 S1）"},
	{20260922001, "staff_invites", "", "员工邀请码表（#2031 邀请制自助加入）"},
	{20260922002, "site_members", "roles", "网点成员多重角色列（#2034）"},
	{20260923001, "pending_orders", "", "待提交订单缓存表（#2041）"},
}

// verifyArtifacts 纯逻辑（便于单测）：对 version ≥ a.Version 的工件断言存在。
func verifyArtifacts(db *gorm.DB, version uint, arts []migrationArtifact) error {
	for _, a := range arts {
		if version < a.Version {
			continue
		}
		if a.Column != "" {
			var n int64
			if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, a.Table, a.Column).Scan(&n).Error; err != nil {
				return fmt.Errorf("check column %s.%s: %w", a.Table, a.Column, err)
			}
			if n == 0 {
				return fmt.Errorf("migration %d recorded applied but column %s.%s is missing (%s) — snapshot/schema drift, replay the migration artifacts", a.Version, a.Table, a.Column, a.Desc)
			}
			continue
		}
		var n int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?`, a.Table).Scan(&n).Error; err != nil {
			return fmt.Errorf("check table %s: %w", a.Table, err)
		}
		if n == 0 {
			return fmt.Errorf("migration %d recorded applied but table %s is missing (%s) — snapshot/schema drift, replay the migration artifacts", a.Version, a.Table, a.Desc)
		}
	}
	return nil
}

// verifyMigrationArtifacts 读取当前迁移版本并校验关键工件（版本不可读时不阻塞启动）。
func verifyMigrationArtifacts(db *gorm.DB) error {
	version, _, _, err := CheckMigrationsStatus(db)
	if err != nil {
		fmt.Printf("Warning: cannot read migration status for artifact check: %v\n", err)
		return nil
	}
	return verifyArtifacts(db, version, defaultMigrationArtifacts)
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
		// #1716: a missing table for a persistence model is a hard failure —
		// the migration is missing (models without a table previously slipped
		// through silently; see merchant_members/damage_reports incidents).
		// Exempt known non-persisted/deprecated models so startup still works.
		switch tableName {
		case "confirmation_sessions", "labels":
			return nil // intentionally non-persisted
		}
		return fmt.Errorf("table %q for model %s is missing — a migration is required (models must have matching up/down SQL)", tableName, typ.Name())
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

	// Full persistence-model list (#1716): every model with a table must be
	// validated. Table-missing models below are NOT failures — they are either
	// intentionally exempt (non-persisted / composite) or deprecated:
	//   ConfirmationSession, Label — no table ever created (non-persisted)
	modelsToValidate := []interface{}{
		&models.User{},
		&models.FaceCaptureBatch{}, // #1789 T1: 核身批次表（20260829003 migration）
		&models.Category{},
		&models.Instrument{},
		&models.Referral{},
		&models.Notification{},
		&models.InstrumentPhotoSpec{},
		&models.Order{},
		&models.MerchantSettlementConfig{},
		&models.Settlement{},
		&models.SettlementCalculation{},
		&models.OverdueCharge{},
		&models.OrderLog{},
		&models.PaymentSession{},
		&models.SessionOrderLink{},
		&models.OrderPaymentRecord{},
		&models.OrderRefundRecord{},
		&models.Site{},
		&models.MaintenanceTicket{},
		&models.BrandConfig{},
		&models.OwnershipCertificate{},
		&models.Technician{},
		&models.SiteImage{},
		&models.InventoryTransfer{},
		&models.Client{},
		&models.Lease{},
		&models.Deposit{},
		&models.UserInstrument{},
		&models.RepairRequest{},
		&models.RepairLogisticsFee{},   // #1942: 维修服务分段物流费
		&models.RepairReview{},         // #1942: 维修服务评价
		&models.InstrumentLossRecord{}, // #1948: 乐器丢失台账
		&models.TechnicianProfile{},    // #1974 T1: 师傅档案（直属商户）
		&models.RepairRequestRecord{},
		&models.PointsTransaction{},
		&models.Tenant{},
		&models.InstrumentLevel{},
		&models.Property{},
		&models.PropertyOption{},
		&models.InstrumentProperty{},
		&models.MaintenanceWorker{},
		&models.MaintenanceSession{},
		&models.MaintenanceSessionRecord{},
		&models.RepairRecord{},
		&models.ForwardingSession{},
		&models.TransitShippingFee{}, // #1934: 受控中转物流费分段
		&models.ElectronicContract{},
		&models.DamageReport{},
		&models.Appeal{},
		&models.OrderStatusHistory{},
		&models.AuditLog{},
		&models.InstrumentPhotoBatch{},
		&models.Merchant{},
		&models.SiteMember{},
		&models.StaffInvite{},
		&models.PendingOrder{},
		&models.MerchantMember{},
		&models.Role{},
		&models.InstrumentMedia{},
		&models.SystemSetting{},
		&models.MediaAsset{},
		&models.PricingTemplate{},
		&models.MerchantPricingConfig{},
		&models.UserAddress{},
		&models.TransitRoute{},
		&models.TransitOrder{},
		&models.RepairTransitOrder{},
		&models.Warning{},
		&models.Banner{},
		&models.InvoiceApplication{},
		&models.MembershipLevelBenefit{},  // #1830: 会员权益行（20260907001 migration）
		&models.InstrumentPromoOverride{}, // #1863: 乐器促销覆盖（20260910001 migration）
		&models.ConfirmationSession{},
		&models.Label{},
		&models.PointBatch{},            // #1947 Sub-D: 乐币批次（20260917010 migration）
		&models.PointBatchConsumption{}, // #1947 Sub-D: 批次扣减留痕
		&models.GiftPolicy{},            // #1945 Sub-B: 乐币规则（20260917011 变更列）
	}

	for _, m := range modelsToValidate {
		if err := validateModelColumns(db, m); err != nil {
			return err
		}
	}

	// #1727: 金额列必须是 BIGINT（分）。DECIMAL/numeric 残留 = 迁移未执行 → FATAL。
	if err := validateMoneyColumnsBigInt(db); err != nil {
		return err
	}

	fmt.Println("✓ Database schema validation passed")
	return nil
}

// ValidateMoneyColumnsForTest 导出金额列 bigint 校验（测试用）。
func ValidateMoneyColumnsForTest(db *gorm.DB) error {
	return validateMoneyColumnsBigInt(db)
}

// moneyColumns 是 #1727 迁移的金额列清单（表名, 列名）。
// 校验通过 = 全部为 bigint；存在 numeric/decimal 类型 = 迁移缺失 → 拒绝启动。
var moneyColumns = [][2]string{
	{"appeals", "final_amount"},
	{"damage_reports", "damage_amount"},
	{"damage_reports", "deposit_deducted"},
	{"damage_reports", "overdue_fee"},
	{"deposits", "amount"},
	{"discount_policies", "max_amount"},
	{"instruments", "base_daily_rate"},
	{"instruments", "deposit"},
	{"instruments", "total_price"},
	{"leases", "deposit_amount"},
	{"leases", "monthly_rent"},
	{"maintenance_tickets", "estimated_cost"},
	{"membership_levels", "min_amount"},
	{"order_payment_records", "amount"},
	{"order_refund_records", "amount"},
	{"orders", "cash_paid"},
	{"orders", "deposit"},
	{"orders", "gift_points_used"},
	{"orders", "monthly_rent"},
	{"orders", "prepaid_points_used"},
	{"orders", "shipping_fee"},
	{"overdue_charges", "amount"},
	{"payment_sessions", "amount"},
	{"points_transactions", "amount"},
	{"points_transactions", "balance_after_prepaid"},
	{"registration_sessions", "amount"},
	{"repair_quotes", "logistics_fee"},
	{"repair_quotes", "material_fee"},
	{"repair_quotes", "service_fee"},
	{"repair_requests", "check_fee_snapshot"},
	{"repair_requests", "inspection_fee"},
	{"repair_requests", "paid_amount"},
	{"repair_requests", "quote_amount"},
	{"repair_requests", "shipping_fee"},
	{"repair_transit_orders", "transit_logistics_fee"},
	{"repair_transit_orders", "transit_service_fee"},
	{"settlements", "actual_rent_amount"},
	{"settlements", "cash_refundable"},
	{"settlements", "gift_points_refunded"},
	{"settlements", "original_rent_amount"},
	{"settlements", "overdue_charges_total"},
	{"settlements", "prepaid_refunded"},
	{"users", "prepaid_points"},
	{"users", "total_spending"},
}

// validateMoneyColumnsBigInt 校验金额列类型为 bigint（#1727 fail-fast）。
// 列缺失（表未建/迁移未跑）与列类型非 bigint 都视为失败。
func validateMoneyColumnsBigInt(db *gorm.DB) error {
	for _, mc := range moneyColumns {
		table, column := mc[0], mc[1]
		var cnt int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_name = ? AND column_name = ?`, table, column).Scan(&cnt).Error; err != nil {
			return fmt.Errorf("money column check %s.%s: %w", table, column, err)
		}
		if cnt == 0 {
			// 表不存在（旧库无此表）也属异常——迁移应已建表
			var tableCnt int64
			if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?`, table).Scan(&tableCnt).Error; err != nil {
				return fmt.Errorf("money column check table %s: %w", table, err)
			}
			if tableCnt > 0 {
				return fmt.Errorf("money column %s.%s missing — cents migration (20260820001) not applied", table, column)
			}
			continue // 表不存在则跳过（豁免废弃表）
		}
		var dataType string
		if err := db.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = ? AND column_name = ?`, table, column).Scan(&dataType).Error; err != nil {
			return fmt.Errorf("money column type check %s.%s: %w", table, column, err)
		}
		if dataType != "bigint" {
			return fmt.Errorf("money column %s.%s is %s, expected bigint (cents) — run migration 20260820001_cents_money_columns", table, column, dataType)
		}
	}
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
