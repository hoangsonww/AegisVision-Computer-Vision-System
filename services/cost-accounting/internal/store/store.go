// Per-tenant cost store. Production cost ingestion runs as a scheduled job
// that reads OpenCost (k8s resource cost) and DCGM (GPU-second) and
// projects them onto our tenants via standard labels.
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

type Line struct {
	TenantID, PipelineID, Day, LineItem string
	Quantity, UnitCostUSD, TotalUSD     float64
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

// Upsert merges a cost line; quantity + total_usd are added; unit_cost_usd
// is replaced (cost rates change daily, not by event).
func (s *Store) Upsert(ctx context.Context, l Line) error {
	now := time.Now().UTC()
	if s.IsPostgres {
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO cost_lines(tenant_id, pipeline_id, day, line_item, quantity, unit_cost_usd, total_usd, updated_at)
             VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
             ON CONFLICT (tenant_id, day, line_item, pipeline_id) DO UPDATE SET
               quantity = cost_lines.quantity + EXCLUDED.quantity,
               unit_cost_usd = EXCLUDED.unit_cost_usd,
               total_usd = cost_lines.total_usd + EXCLUDED.total_usd,
               updated_at = EXCLUDED.updated_at`,
			l.TenantID, l.PipelineID, l.Day, l.LineItem, l.Quantity, l.UnitCostUSD, l.TotalUSD, now)
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO cost_lines(tenant_id, pipeline_id, day, line_item, quantity, unit_cost_usd, total_usd, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(tenant_id, day, line_item, pipeline_id) DO UPDATE SET
           quantity = quantity + excluded.quantity,
           unit_cost_usd = excluded.unit_cost_usd,
           total_usd = total_usd + excluded.total_usd,
           updated_at = excluded.updated_at`,
		l.TenantID, l.PipelineID, l.Day, l.LineItem, l.Quantity, l.UnitCostUSD, l.TotalUSD, now)
	return err
}

// TenantTotal returns total USD over a date range.
func (s *Store) TenantTotal(ctx context.Context, tenant, from, to string) (float64, error) {
	q := s.ph(`SELECT COALESCE(SUM(total_usd),0) FROM cost_lines WHERE tenant_id = ? AND day BETWEEN ? AND ?`)
	var t float64
	err := s.DB.QueryRowContext(ctx, q, tenant, from, to).Scan(&t)
	return t, err
}

// Breakdown returns cost broken down by line_item.
func (s *Store) Breakdown(ctx context.Context, tenant, from, to string) (map[string]float64, error) {
	q := s.ph(`SELECT line_item, SUM(total_usd) FROM cost_lines WHERE tenant_id = ? AND day BETWEEN ? AND ?
        GROUP BY line_item`)
	rows, err := s.DB.QueryContext(ctx, q, tenant, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var li string
		var t float64
		if err := rows.Scan(&li, &t); err != nil {
			return nil, err
		}
		out[li] = t
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
