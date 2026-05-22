package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
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

// --- Policies ---

func (s *Store) UpsertPolicy(ctx context.Context, p intelligence.DriftPolicy) error {
	q := s.ph(`INSERT INTO drift_policies(id, tenant_id, model_id, method, warn_threshold, critical_threshold, window_seconds, min_samples)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET method = excluded.method,
          warn_threshold = excluded.warn_threshold, critical_threshold = excluded.critical_threshold,
          window_seconds = excluded.window_seconds, min_samples = excluded.min_samples`)
	if s.IsPostgres {
		q = strings.ReplaceAll(q, "excluded.", "EXCLUDED.")
	}
	_, err := s.DB.ExecContext(ctx, q,
		p.ID, p.TenantID, p.ModelID, string(p.Method),
		p.WarnThreshold, p.CriticalThreshold, p.WindowSeconds, p.MinSamples)
	return err
}

func (s *Store) ListPoliciesByModel(ctx context.Context, tenant, modelID string) ([]intelligence.DriftPolicy, error) {
	q := s.ph(`SELECT id, tenant_id, model_id, method, warn_threshold, critical_threshold, window_seconds, min_samples FROM drift_policies WHERE tenant_id = ? AND model_id = ?`)
	rows, err := s.DB.QueryContext(ctx, q, tenant, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.DriftPolicy
	for rows.Next() {
		var p intelligence.DriftPolicy
		var method string
		if err := rows.Scan(&p.ID, &p.TenantID, &p.ModelID, &method, &p.WarnThreshold, &p.CriticalThreshold, &p.WindowSeconds, &p.MinSamples); err != nil {
			return nil, err
		}
		p.Method = intelligence.DivergenceMethod(method)
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- References ---

func (s *Store) UpsertReference(ctx context.Context, r intelligence.DriftReference) error {
	classes, _ := json.Marshal(r.Distribution.Classes)
	if r.CapturedAt.IsZero() {
		r.CapturedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO drift_references(id, tenant_id, model_id, classes_json, samples, captured_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET classes_json = excluded.classes_json, samples = excluded.samples, captured_at = excluded.captured_at`)
	if s.IsPostgres {
		q = strings.ReplaceAll(q, "excluded.", "EXCLUDED.")
	}
	_, err := s.DB.ExecContext(ctx, q,
		r.ID, r.TenantID, r.ModelID, string(classes), r.Distribution.Samples, r.CapturedAt)
	return err
}

func (s *Store) GetReference(ctx context.Context, tenant, modelID string) (*intelligence.DriftReference, error) {
	q := s.ph(`SELECT id, tenant_id, model_id, classes_json, samples, captured_at FROM drift_references WHERE tenant_id = ? AND model_id = ? ORDER BY captured_at DESC LIMIT 1`)
	row := s.DB.QueryRowContext(ctx, q, tenant, modelID)
	var r intelligence.DriftReference
	var classes string
	if err := row.Scan(&r.ID, &r.TenantID, &r.ModelID, &classes, &r.Distribution.Samples, &r.CapturedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(classes), &r.Distribution.Classes)
	return &r, nil
}

// --- Signals ---

func (s *Store) InsertSignal(ctx context.Context, sig intelligence.DriftSignal) error {
	classes, _ := json.Marshal(sig.Observed.Classes)
	if sig.EmittedAt.IsZero() {
		sig.EmittedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO drift_signals(id, tenant_id, model_id, method, divergence, severity, classes_json, emitted_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		sig.ID, sig.TenantID, sig.ModelID, string(sig.Method), sig.Divergence, sig.Severity, string(classes), sig.EmittedAt)
	return err
}

func (s *Store) ListSignals(ctx context.Context, tenant string, since time.Time, limit int) ([]intelligence.DriftSignal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.ph(fmt.Sprintf(`SELECT id, tenant_id, model_id, method, divergence, severity, classes_json, emitted_at FROM drift_signals WHERE tenant_id = ? AND emitted_at >= ? ORDER BY emitted_at DESC LIMIT %d`, limit))
	rows, err := s.DB.QueryContext(ctx, q, tenant, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.DriftSignal
	for rows.Next() {
		var sig intelligence.DriftSignal
		var method, classes string
		if err := rows.Scan(&sig.ID, &sig.TenantID, &sig.ModelID, &method, &sig.Divergence, &sig.Severity, &classes, &sig.EmittedAt); err != nil {
			return nil, err
		}
		sig.Method = intelligence.DivergenceMethod(method)
		_ = json.Unmarshal([]byte(classes), &sig.Observed.Classes)
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
