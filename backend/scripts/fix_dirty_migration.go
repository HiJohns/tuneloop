package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	// Connect to database
	db, err := sql.Open("postgres", "host=localhost port=5432 user=tuneloop_user password=tuneloop dbname=tuneloop_db sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check if migration table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'schema_migrations'
		)
	`).Scan(&tableExists)
	if err != nil {
		log.Fatal(err)
	}

	if !tableExists {
		fmt.Println("schema_migrations table does not exist. Creating...")
		_, err = db.Exec(`
			CREATE TABLE schema_migrations (
				version BIGINT PRIMARY KEY,
				dirty BOOLEAN NOT NULL
			)
		`)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Check current state
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 14").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}

	if count == 0 {
		// Insert version 14 as completed
		_, err = db.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (14, false)")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Inserted schema_migrations version 14 as not dirty")
	} else {
		// Update to not dirty
		_, err = db.Exec("UPDATE schema_migrations SET dirty = false WHERE version = 14")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Updated schema_migrations version 14 to not dirty")
	}

	fmt.Println("Migration state fixed successfully!")
}
