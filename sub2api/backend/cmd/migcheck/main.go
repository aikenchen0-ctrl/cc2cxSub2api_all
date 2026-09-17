package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL required")
		os.Exit(1)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := repository.ApplyMigrations(ctx, db); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ok schema_migrations=%d\n", n)
}
