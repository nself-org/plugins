package main

// db.go — database connection helper

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

// mustOpenDB opens a Postgres connection from DATABASE_URL or panics.
func mustOpenDB() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("rotation: DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("rotation: open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("rotation: ping db: %v", err)
	}
	fmt.Println("rotation: database connected")
	return db
}
