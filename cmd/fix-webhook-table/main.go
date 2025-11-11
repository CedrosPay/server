package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	// Connection string from local.yaml
	connStr := "postgresql://app_user:app_password@10.0.0.1:5420/development?sslmode=disable"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("✓ Connected to database successfully")

	// Drop the old table
	fmt.Println("Dropping cedros_pay_test_webhook_queue...")
	_, err = db.Exec("DROP TABLE IF EXISTS cedros_pay_test_webhook_queue CASCADE")
	if err != nil {
		log.Fatal("Failed to drop table:", err)
	}

	fmt.Println("✓ Table dropped successfully!")
	fmt.Println("\nNext step: Restart your server to recreate the table with correct schema.")
	fmt.Println("The server will automatically create the table with all required columns.")
}
