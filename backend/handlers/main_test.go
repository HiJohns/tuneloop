package handlers

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	_ "github.com/lib/pq"
	"gorm.io/gorm"
)

// testDBName is the isolated database used by tests, separate from the dev database.
// Resolved in TestMain: TUNELOOP_TEST_DB > tuneloop_test_<node>_<hash> > tuneloop_test.
var testDBName = "tuneloop_test"

// 白名单：只允许测试库命名空间，禁止 TUNELOOP_TEST_DB 指向生产库。
var testDBNameRe = regexp.MustCompile(`^tuneloop_test(_[a-z0-9_]+)?$`)

// resolveTestDBName 返回按节点隔离的测试库名；非法名称必须显式报错（不得净化后使用）。
func resolveTestDBName() (string, error) {
	if v := strings.TrimSpace(os.Getenv("TUNELOOP_TEST_DB")); v != "" {
		name, err := validateTestDBName(v)
		if err != nil {
			return "", fmt.Errorf("TUNELOOP_TEST_DB=%q: %w", v, err)
		}
		return name, nil
	}
	node := strings.TrimSpace(os.Getenv("OPENCODE_NODE_ID"))
	if node == "" {
		return "tuneloop_test", nil
	}
	// 追加原始 ID 的短 hash：nodeA/nodea、node-2/node_2 不会映射到同一库。
	// #nosec G401 -- 非加密用途，仅用于名称去重。
	sum := sha1.Sum([]byte(node))
	name := fmt.Sprintf("tuneloop_test_%s_%s", sanitizeNodeSegment(node), hex.EncodeToString(sum[:])[:8])
	return validateTestDBName(name)
}

func validateTestDBName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("test DB name is empty")
	}
	if len(name) > 63 {
		return "", fmt.Errorf("test DB name too long (%d > 63)", len(name))
	}
	if !testDBNameRe.MatchString(name) {
		return "", fmt.Errorf("test DB name %q must match %s", name, testDBNameRe)
	}
	return name, nil
}

// sanitizeNodeSegment 仅用于节点名的可读片段；唯一性由 hash 保证。
func sanitizeNodeSegment(node string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(node) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "node"
	}
	return out
}

// allTestModels lists all GORM model types needed by handlers tests.
// AutoMigrate uses these to create the schema in the isolated test database.
var allTestModels = []interface{}{
	&models.Appeal{},
	&models.Banner{},
	&models.ConfirmationSession{},
	&models.Coupon{},
	&models.DamageReport{},
	&models.ElectronicContract{},
	&models.Guarantor{},
	&models.Instrument{},
	&models.InstrumentLevel{},
	&models.InstrumentMedia{},
	&models.InstrumentPhotoBatch{},
	&models.InstrumentPhotoSpec{},
	&models.InstrumentProperty{},
	&models.InventoryTransfer{},
	&models.Settlement{},
	&models.SettlementCalculation{},
	&models.InvoiceApplication{},
	&models.MaintenanceSession{},
	&models.MaintenanceSessionRecord{},
	&models.MaintenanceWorker{},
	&models.MaintenanceTicket{},
	&models.Merchant{},
	&models.Notification{},
	&models.Order{},
	&models.OrderTermVersion{},
	&models.ReferralCommission{},
	&models.OrderStatusHistory{},
	&models.PaymentSession{},
	&models.OrderPaymentRecord{},
	&models.OrderRefundRecord{},
	&models.FaceCaptureBatch{},
	&models.Property{},
	&models.PropertyOption{},
	&models.RepairRequest{},
	&models.RepairRequestRecord{},
	&models.RepairQuote{},
	&models.RepairTransitOrder{},
	&models.RegistrationSession{},
	&models.Role{},
	&models.Site{},
	&models.SiteImage{},
	&models.SiteMember{},
	&models.Tenant{},
	&models.TransitRoute{},
	&models.User{},
	&models.UserInstrument{},
	&models.Warning{},
	&models.PointBatch{},            // #1947 Sub-D
	&models.PointBatchConsumption{}, // #1947 Sub-D
}

func TestMain(m *testing.M) {
	// #2288: 按节点隔离测试库；名称不在白名单内则拒绝运行（防误连生产库）
	resolved, err := resolveTestDBName()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	testDBName = resolved
	// Redirect all tests to the isolated test database
	os.Setenv("TUNELOOP_DB", testDBName)
	// #2288: 共享连接池 + 测试间 DDL → 禁用 pgx prepared statement 缓存
	os.Setenv("TUNELOOP_DB_SIMPLE_PROTOCOL", "1")

	// Safety guard: verify the env var actually took effect
	gotDB := os.Getenv("TUNELOOP_DB")
	if gotDB != testDBName {
		fmt.Fprintf(os.Stderr, "FATAL: failed to set TUNELOOP_DB to '%s', got '%s'\n", testDBName, gotDB)
		os.Exit(1)
	}

	// Ensure the test database exists
	ensureTestDB()

	// Build schema via AutoMigrate (works on fresh DBs, unlike incremental migrations)
	cfg := database.LoadConfig()
	testDB, err := database.InitDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to connect to test DB '%s': %v\n", testDBName, err)
		os.Exit(1)
	}

	// Create tables for all models via AutoMigrate (works on fresh DBs)
	if err := testDB.AutoMigrate(allTestModels...); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: AutoMigrate failed on test DB '%s': %v\n", testDBName, err)
		os.Exit(1)
	}

	// iam_sub has -:migration tag and is excluded from AutoMigrate; add it manually
	addIAMSubColumn(testDB)

	// invoice_application_orders is a plain join table not modeled by GORM; create manually
	testDB.Exec(`CREATE TABLE IF NOT EXISTS invoice_application_orders (
		application_id UUID NOT NULL,
		order_id UUID NOT NULL,
		PRIMARY KEY (application_id, order_id)
	)`)

	// Truncate tables that tests seed directly (without setupE2ETestEnv) to
	// avoid duplicate-key errors from rows left by previous test runs.
	for _, tbl := range []string{"damage_reports", "orders", "instruments", "users", "settlements", "invoice_applications", "invoice_application_orders", "notifications"} {
		testDB.Exec("TRUNCATE TABLE " + tbl + " CASCADE")
	}

	database.SetDB(testDB)

	code := m.Run()
	os.Exit(code)
}

// addIAMSubColumn adds the iam_sub column to users if it does not exist.
// The User model has `-:migration` on IAMSub, so AutoMigrate skips it.
func addIAMSubColumn(db *gorm.DB) {
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
}

// ensureTestDB creates the test database if it does not exist.
// Connects to the maintenance database ('postgres') to issue the CREATE DATABASE command.
func ensureTestDB() {
	cfg := database.LoadConfig()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: cannot connect to postgres maintenance DB: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please create the '%s' database manually:\n", testDBName)
		fmt.Fprintf(os.Stderr, "  CREATE DATABASE %s;\n", testDBName)
		os.Exit(1)
	}
	defer db.Close()

	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", testDBName).Scan(&exists)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: cannot check if test DB exists: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure '%s' exists and has proper permissions.\n", testDBName)
		os.Exit(1)
	}

	if !exists {
		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: cannot create test DB '%s': %v\n", testDBName, err)
			fmt.Fprintf(os.Stderr, "Please create it manually:\n")
			fmt.Fprintf(os.Stderr, "  CREATE DATABASE %s;\n", testDBName)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Created test database '%s'\n", testDBName)
	}
}

func TestDatabaseIsolation(t *testing.T) {
	cfg := database.LoadConfig()
	if cfg.DBName != testDBName {
		t.Fatalf("tests should run on '%s' but got '%s'", testDBName, cfg.DBName)
	}
}
