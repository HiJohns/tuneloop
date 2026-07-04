package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	_ "github.com/lib/pq"
	"gorm.io/gorm"
)

// testDBName is the isolated database used by tests, separate from the dev database.
const testDBName = "tuneloop_test"

// allTestModels lists all GORM model types needed by handlers tests.
// AutoMigrate uses these to create the schema in the isolated test database.
var allTestModels = []interface{}{
	&models.Appeal{},
	&models.Banner{},
	&models.ConfirmationSession{},
	&models.DamageAssessment{},
	&models.DamageReport{},
	&models.ElectronicContract{},
	&models.Instrument{},
	&models.InstrumentLevel{},
	&models.InstrumentMedia{},
	&models.InstrumentPhotoBatch{},
	&models.InstrumentPhotoSpec{},
	&models.InstrumentProperty{},
	&models.InventoryTransfer{},
	&models.LeaseSession{},
	&models.MaintenanceSession{},
	&models.MaintenanceSessionRecord{},
	&models.MaintenanceTicket{},
	&models.MaintenanceWorker{},
	&models.Merchant{},
	&models.Notification{},
	&models.Order{},
	&models.OrderStatusHistory{},
	&models.Property{},
	&models.PropertyOption{},
	&models.RepairRequest{},
	&models.RepairRequestRecord{},
	&models.RepairQuote{},
	&models.RepairTransitOrder{},
	&models.Role{},
	&models.Site{},
	&models.SiteImage{},
	&models.SiteMember{},
	&models.Tenant{},
	&models.TransitRoute{},
	&models.User{},
	&models.UserInstrument{},
	&models.Warning{},
}

func TestMain(m *testing.M) {
	// Redirect all tests to the isolated test database
	os.Setenv("TUNELOOP_DB", testDBName)

	// Safety guard: verify the env var actually took effect
	resolved := os.Getenv("TUNELOOP_DB")
	if resolved != testDBName {
		fmt.Fprintf(os.Stderr, "FATAL: failed to set TUNELOOP_DB to '%s', got '%s'\n", testDBName, resolved)
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
