// Daily counter store for the metering-service. Per the architecture doc
// the production billing pipeline runs ClickHouse with a SummingMergeTree
// engine over (tenant, metric, day); this implementation is the canonical
// reference + the SQLite path that powers unit tests.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
)

//go:embed *.sql
var migrationsFS embed.FS

func MigrateUp(ctx context.Context, db *sql.DB) error {
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

// Increment bumps the (tenant, metric, today) counter by n. Today is
// computed UTC so all replicas agree.
func (s *Store) Increment(ctx context.Context, tenant, metric string, n int64) error {
	day := time.Now().UTC().Format("2006-01-02")
	if s.IsPostgres {
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO meters(tenant_id, metric, day, count) VALUES ($1,$2,$3,$4)
             ON CONFLICT (tenant_id, metric, day) DO UPDATE SET count = meters.count + EXCLUDED.count`,
			tenant, metric, day, n)
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO meters(tenant_id, metric, day, count) VALUES (?, ?, ?, ?)
         ON CONFLICT(tenant_id, metric, day) DO UPDATE SET count = count + excluded.count`,
		tenant, metric, day, n)
	return err
}

// Day returns the count for one (tenant, metric, day).
func (s *Store) Day(ctx context.Context, tenant, metric, day string) (int64, error) {
	q := s.ph(`SELECT count FROM meters WHERE tenant_id = ? AND metric = ? AND day = ?`)
	var n int64
	err := s.DB.QueryRowContext(ctx, q, tenant, metric, day).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// Range returns daily counts for a metric over [from, to] inclusive.
func (s *Store) Range(ctx context.Context, tenant, metric, from, to string) (map[string]int64, error) {
	q := s.ph(`SELECT day, count FROM meters WHERE tenant_id = ? AND metric = ? AND day BETWEEN ? AND ? ORDER BY day`)
	rows, err := s.DB.QueryContext(ctx, q, tenant, metric, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var day string
		var n int64
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		out[day] = n
	}
	return out, rows.Err()
}

func (s *Store) ph(q string) string {
	if !s.IsPostgres {
		return q
	}
	var out strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			fmt.Fprintf(&out, "$%d", n)
		} else {
			out.WriteByte(q[i])
		}
	}
	return out.String()
}
