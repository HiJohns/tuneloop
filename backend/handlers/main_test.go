package handlers

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

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
	&models.PricingTemplate{},
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
	// #2336: NewIAMService log.Fatal()s when credentials are absent, which kills
	// the whole test process without a test failure. Provide package-wide
	// defaults; individual tests may still override via t.Setenv.
	os.Setenv("IAM_NAMESPACE", "test-ns")
	os.Setenv("IAM_SECRET", "test-secret-1489")
	// #2288: 共享连接池 + 测试间 DDL → 禁用 pgx prepared statement 缓存
	os.Setenv("TUNELOOP_DB_SIMPLE_PROTOCOL", "1")

	// Safety guard: verify the env var actually took effect
	gotDB := os.Getenv("TUNELOOP_DB")
	if gotDB != testDBName {
		fmt.Fprintf(os.Stderr, "FATAL: failed to set TUNELOOP_DB to '%s', got '%s'\n", testDBName, gotDB)
		os.Exit(1)
	}

	// #2341: 模板库（AutoMigrate + seed）→ 克隆 run 库。
	cfg := database.LoadConfig()
	opts := testfixtures.TemplateOptions{
		Host: cfg.Host, Port: cfg.Port, User: cfg.User,
		Password: cfg.Password, SSLMode: cfg.SSLMode,
		TemplateName: testfixtures.TemplateDBName,
		Models:       allTestModels,
		PostMigrate: func(db *gorm.DB) error {
			// iam_sub 有 -:migration 标签，AutoMigrate 不建列，需手工补。
			addIAMSubColumn(db)
			// 纯 SQL join 表（无 GORM 模型）。
			return db.Exec(`CREATE TABLE IF NOT EXISTS invoice_application_orders (
				application_id UUID NOT NULL,
				order_id UUID NOT NULL,
				PRIMARY KEY (application_id, order_id)
			)`).Error
		},
	}
	if os.Getenv("FORCE_TEMPLATE_REBUILD") == "1" {
		if err := testfixtures.DropDatabase(opts, opts.TemplateName); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: drop template: %v\n", err)
			os.Exit(1)
		}
	}
	if rebuilt, err := testfixtures.EnsureTemplate(opts); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: ensure template: %v\n", err)
		os.Exit(1)
	} else if rebuilt {
		fmt.Fprintf(os.Stderr, "Built test template %s\n", opts.TemplateName)
	}
	if err := testfixtures.CloneTemplate(opts, testDBName); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: clone test db: %v\n", err)
		os.Exit(1)
	}

	testDB, err := database.InitDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to connect to test DB '%s': %v\n", testDBName, err)
		os.Exit(1)
	}
	// D4: 金额列必须为 bigint；numeric 会被 Cents.Scan 当作元 ×100。
	if err := testfixtures.AssertCentsSchema(testDB, allTestModels); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	database.SetDB(testDB)

	// #2336: shipping retries must never sleep minutes inside the test process.
	// Tests that assert the retry path override this locally; this default keeps
	// any incidental reportWechatShipping goroutine fast and network-free.
	restoreShippingBackoffs := services.SetShippingBackoffsForTesting([]time.Duration{0, 10 * time.Millisecond})
	defer restoreShippingBackoffs()

	code := m.Run()

	// #2341: 用后清理 run 库（模板保留为共享 fixture）。
	restoreShippingBackoffs()
	if sqlDB, err := testDB.DB(); err == nil {
		sqlDB.Close()
	}
	if os.Getenv("KEEP_TEST_DB") != "1" {
		if err := testfixtures.DropDatabase(opts, testDBName); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: drop test db %s: %v\n", testDBName, err)
		}
	}
	os.Exit(code)
}

// addIAMSubColumn adds the iam_sub column to users if it does not exist.
// The User model has `-:migration` on IAMSub, so AutoMigrate skips it.
func addIAMSubColumn(db *gorm.DB) {
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
}

func TestDatabaseIsolation(t *testing.T) {
	cfg := database.LoadConfig()
	if cfg.DBName != testDBName {
		t.Fatalf("tests should run on '%s' but got '%s'", testDBName, cfg.DBName)
	}
}
