// Package migrate runs ordered SQL migrations against a sql.DB.
//
// Migrations are embedded into each service (typically via go:embed) and
// applied at startup. Each migration is one file; the filename starts with
// a zero-padded numeric prefix (e.g. 0001_create_pipelines.sql). A single
// `schema_migrations` table tracks applied versions.
//
// Migrations are applied within a single transaction per file — partial
// success isn't a thing. Failures abort startup, which is the correct
// posture for a control-plane service.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration is one ordered step.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// LoadFS loads every *.sql file from fsys, sorted by leading numeric prefix.
func LoadFS(fsys fs.FS) ([]Migration, error) {
	var ms []Migration
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if !strings.HasSuffix(path, ".sql") {
			return nil
		}
		base := filepath.Base(path)
		// Expect "NNNN_name.sql".
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			return fmt.Errorf("migrate: bad filename %q (want NNNN_name.sql)", base)
		}
		ver, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("migrate: bad version in %q: %w", base, err)
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		ms = append(ms, Migration{
			Version: ver,
			Name:    strings.TrimSuffix(parts[1], ".sql"),
			SQL:     string(body),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Version < ms[j].Version })
	// Sanity: no duplicate version numbers.
	for i := 1; i < len(ms); i++ {
		if ms[i].Version == ms[i-1].Version {
			return nil, fmt.Errorf("migrate: duplicate version %d", ms[i].Version)
		}
	}
	return ms, nil
}

// Apply runs every migration whose version > the highest applied. Idempotent;
// safe to call on every process startup.
func Apply(ctx context.Context, db *sql.DB, ms []Migration) error {
	if err := ensureTable(ctx, db); err != nil {
		return err
	}
	applied, err := loadApplied(ctx, db)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("migrate: %d_%s: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func ensureTable(ctx context.Context, db *sql.DB) error {
	const q = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMP NOT NULL
	)`
	_, err := db.ExecContext(ctx, q)
	return err
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	out := map[int]bool{}
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func applyOne(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if errors.Is(tx.Rollback(), sql.ErrTxDone) {
			// already committed
		}
	}()
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().UTC(),
	); err != nil {
		// Postgres rejects ? placeholders; retry with $1/$2/$3.
		if _, err2 := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, name, applied_at) VALUES ($1, $2, $3)`,
			m.Version, m.Name, time.Now().UTC(),
		); err2 != nil {
			return err
		}
	}
	return tx.Commit()
}
