package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/intelligence"
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

func New(db *sql.DB, isPg bool) *Store { return &Store{DB: db, IsPostgres: isPg} }

func (s *Store) UpsertTarget(ctx context.Context, t intelligence.SLOTarget) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	paused := 0
	if t.Paused {
		paused = 1
	}
	q := s.ph(`INSERT INTO slo_targets(id, tenant_id, name, promql, target, rolling_window_seconds, paused, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET name = excluded.name, promql = excluded.promql,
          target = excluded.target, rolling_window_seconds = excluded.rolling_window_seconds,
          paused = excluded.paused, updated_at = excluded.updated_at`)
	if s.IsPostgres {
		q = strings.ReplaceAll(q, "excluded.", "EXCLUDED.")
	}
	_, err := s.DB.ExecContext(ctx, q, t.ID, t.TenantID, t.Name, t.PromQL, t.Target,
		t.RollingWindowSeconds, paused, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *Store) ListActiveTargets(ctx context.Context) ([]intelligence.SLOTarget, error) {
	q := s.ph(`SELECT id, tenant_id, name, promql, target, rolling_window_seconds, paused, created_at, updated_at FROM slo_targets WHERE paused = 0 ORDER BY tenant_id, name LIMIT 1000`)
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.SLOTarget
	for rows.Next() {
		var t intelligence.SLOTarget
		var paused int
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.PromQL, &t.Target, &t.RollingWindowSeconds, &paused, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Paused = paused == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListByTenant(ctx context.Context, tenant string) ([]intelligence.SLOTarget, error) {
	q := s.ph(`SELECT id, tenant_id, name, promql, target, rolling_window_seconds, paused, created_at, updated_at FROM slo_targets WHERE tenant_id = ? ORDER BY name`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.SLOTarget
	for rows.Next() {
		var t intelligence.SLOTarget
		var paused int
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.PromQL, &t.Target, &t.RollingWindowSeconds, &paused, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Paused = paused == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) InsertSignal(ctx context.Context, sig intelligence.SLOSignal) error {
	if sig.EmittedAt.IsZero() {
		sig.EmittedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO slo_signals(id, tenant_id, target_id, burn_rate_1h, burn_rate_6h, burn_rate_24h, severity, emitted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, sig.ID, sig.TenantID, sig.TargetID, sig.BurnRate1h, sig.BurnRate6h, sig.BurnRate24h, sig.Severity, sig.EmittedAt)
	return err
}

func (s *Store) ListSignals(ctx context.Context, tenant string, since time.Time, limit int) ([]intelligence.SLOSignal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.ph(fmt.Sprintf(`SELECT id, tenant_id, target_id, burn_rate_1h, burn_rate_6h, burn_rate_24h, severity, emitted_at FROM slo_signals WHERE tenant_id = ? AND emitted_at >= ? ORDER BY emitted_at DESC LIMIT %d`, limit))
	rows, err := s.DB.QueryContext(ctx, q, tenant, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.SLOSignal
	for rows.Next() {
		var sig intelligence.SLOSignal
		if err := rows.Scan(&sig.ID, &sig.TenantID, &sig.TargetID, &sig.BurnRate1h, &sig.BurnRate6h, &sig.BurnRate24h, &sig.Severity, &sig.EmittedAt); err != nil {
			return nil, err
		}
		out = append(out, sig)
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
