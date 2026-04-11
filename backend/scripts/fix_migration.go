package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	// Get database URL from environment or construct it
	dbHost := os.Getenv("POSTGRES_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}

	dbUser := os.Getenv("POSTGRES_USER")
	if dbUser == "" {
		dbUser = "tuneloop"
	}

	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	if dbPassword == "" {
		fmt.Println("❌ POSTGRES_PASSWORD environment variable not set")
		os.Exit(1)
	}

	dbName := os.Getenv("TUNELOOP_DB")
	if dbName == "" {
		dbName = "tuneloop"
	}

	dbURL := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbName)

	// Create migrate instance
	m, err := migrate.New(
		"file://database/migrations",
		dbURL)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	// Force version 14 to fix dirty state
	fmt.Println("🔧 Forcing migration version 14 as completed...")
	err = m.Force(14)
	if err != nil {
		log.Fatalf("Failed to force version: %v", err)
	}

	fmt.Println("✅ Migration version 14 forced successfully")
	fmt.Println("📊 Current version:")
	version, dirty, err := m.Version()
	if err != nil {
		log.Printf("Warning: Could not get version: %v", err)
	} else {
		fmt.Printf("   Version: %d, Dirty: %t\n", version, dirty)
	}
}
