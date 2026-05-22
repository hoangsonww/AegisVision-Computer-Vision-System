// Package sqldb is the platform's database access layer.
//
// We use database/sql with pgx's stdlib driver as the canonical production
// path; SQLite (modernc.org/sqlite, pure Go) is the test fallback so unit
// tests run without a Docker dep. Services consume only the *sql.DB and the
// helpers here; they do not import pgx directly.
package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // clickhouse driver
	_ "github.com/jackc/pgx/v5/stdlib"         // postgres driver
	_ "modernc.org/sqlite"                     // sqlite driver (pure Go)
)

// Driver names accepted by Open. "postgres" routes to pgx; "sqlite" routes
// to modernc; "clickhouse" routes to clickhouse-go/v2.
const (
	DriverPostgres   = "postgres"
	DriverSQLite     = "sqlite"
	DriverClickHouse = "clickhouse"
)

// Config tunes the pool. Defaults are set in Open if not provided.
type Config struct {
	Driver          string        // postgres | sqlite
	DSN             string        // pgx DSN or sqlite filename / ":memory:"
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	PingTimeout     time.Duration
}

// Open returns a connection pool configured per Config. It pings the DB once
// to fail fast on misconfiguration.
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.Driver == "" {
		return nil, errors.New("sqldb: Driver is required")
	}
	if cfg.DSN == "" {
		return nil, errors.New("sqldb: DSN is required")
	}
	driverName := cfg.Driver
	if driverName == DriverPostgres {
		driverName = "pgx"
	}
	db, err := sql.Open(driverName, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sqldb: open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(25)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(5)
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = 30 * time.Minute
	}
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = 5 * time.Second
	}
	pctx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqldb: ping: %w", err)
	}
	return db, nil
}

// InTx runs fn within a single transaction; commits on nil return, rolls back
// on error or panic. Use for cross-row invariants — single-statement
// operations don't need this wrapper.
func InTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqldb: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("sqldb: commit: %w", err)
	}
	return nil
}

// IsPostgres reports whether the DSN points at Postgres.
func IsPostgres(cfg Config) bool { return cfg.Driver == DriverPostgres }

// IsClickHouse reports whether the DSN points at ClickHouse. Used by stores
// that ship ClickHouse-specific DDL (MergeTree, TTL clauses, etc.).
func IsClickHouse(cfg Config) bool { return cfg.Driver == DriverClickHouse }
