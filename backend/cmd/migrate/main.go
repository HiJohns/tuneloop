package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers"
)

func main() {
	dbName := os.Getenv("TUNELOOP_DB")
	if dbName == "" {
		dbName = os.Getenv("POSTGRES_DB")
	}
	if dbName == "" {
		dbName = "tuneloop_debug"
	}

	db, err := database.InitDB(database.LoadConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	database.SetDB(db)

	if err := database.BootstrapDatabase(db); err != nil {
		log.Printf("Bootstrap warning: %v", err)
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run cmd/migrate/main.go [flags]\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nAvailable commands (pass as arg):\n")
		fmt.Fprintf(os.Stderr, "  migrate-display-webp    Convert all display images to WebP\n")
		fmt.Fprintf(os.Stderr, "  preview-webp             Preview how many images would be converted\n")
		fmt.Fprintf(os.Stderr, "  migrate-cover-images     Generate cover images for instruments without one\n")
	}

	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	switch args[0] {
	case "migrate-display-webp":
		dryRun := len(args) > 1 && args[1] == "--dry-run"
		count, err := handlers.MigrateDisplayImagesToWebP(dryRun)
		if err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		if dryRun {
			log.Printf("DRY RUN: %d images would be converted", count)
		} else {
			log.Printf("Successfully converted %d images to WebP", count)
		}

	case "preview-webp":
		total, already, needs, err := handlers.PreviewMigrateDisplayImages()
		if err != nil {
			log.Fatalf("Preview failed: %v", err)
		}
		fmt.Printf("Display images: %d total, %d already WebP, %d need conversion\n", total, already, needs)

	case "migrate-cover-images":
		dryRun := len(args) > 1 && args[1] == "--dry-run"
		count, err := handlers.MigrateInstrumentCoverImages(dryRun)
		if err != nil {
			log.Fatalf("Cover migration failed: %v", err)
		}
		if dryRun {
			log.Printf("DRY RUN: would generate %d cover images", count)
		} else {
			log.Printf("Generated %d cover images", count)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}
