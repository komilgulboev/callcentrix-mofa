package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net/url"

	_ "github.com/lib/pq"
)

//go:embed migration.sql
var migrationSQL string

func Connect(dsn string) (*sql.DB, error) {
	log.Printf("DB connecting → %s", safeDSN(dsn))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		log.Printf("DB connect FAILED: %v", err)
		return nil, fmt.Errorf("ping db: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	log.Printf("DB connected  ✓ %s", safeDSN(dsn))
	return db, nil
}

// safeDSN masks the password in a DSN before logging.
func safeDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	if _, hasPass := u.User.Password(); hasPass {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

func Migrate(db *sql.DB) error {
	log.Println("running migrations…")
	_, err := db.Exec(migrationSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Println("migrations done")
	return nil
}
