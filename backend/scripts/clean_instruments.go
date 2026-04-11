package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func main() {
	// Initialize database connection (will load .env automatically)
	db := database.GetDB()
	if db == nil {
		log.Fatal("Failed to connect to database")
	}

	// Count current records
	var count int64
	err := db.Model(&models.Instrument{}).Count(&count).Error
	if err != nil {
		log.Fatalf("Error counting instruments: %v", err)
	}

	if count == 0 {
		fmt.Println("ℹ Instrument table is already empty")
		os.Exit(0)
	}

	fmt.Printf("⚠ Found %d instruments in the table\n", count)
	fmt.Print("Are you sure you want to delete ALL instruments? (type 'YES' to confirm): ")

	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(strings.ToUpper(response))

	if response != "YES" {
		fmt.Println("✓ Operation cancelled")
		os.Exit(0)
	}

	// Delete all records (using Unscoped to bypass soft delete if it exists)
	result := db.Unscoped().Exec("TRUNCATE TABLE instruments RESTART IDENTITY CASCADE")
	if result.Error != nil {
		log.Fatalf("Error truncating instruments table: %v", result.Error)
	}

	fmt.Printf("✓ Successfully cleaned instrument table (%d rows affected)\n", result.RowsAffected)

	// Verify it's empty
	err = db.Model(&models.Instrument{}).Count(&count).Error
	if err != nil {
		log.Fatalf("Error verifying count: %v", err)
	}
	fmt.Printf("ℹ Instrument table now contains %d records\n", count)
}
