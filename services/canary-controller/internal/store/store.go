package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/canary"
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

func (s *Store) CreatePlan(ctx context.Context, p intelligence.CanaryPlan) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	ladder, _ := json.Marshal(p.Ladder)
	ap := 0
	if p.AutoPromote {
		ap = 1
	}
	q := s.ph(`INSERT INTO canary_plans(id, tenant_id, resource_urn, baseline_urn, candidate_urn, ladder_json, max_proportion_delta, max_latency_p95_ms_delta, auto_promote, status, current_step, last_step_at, failure_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		p.ID, p.TenantID, p.ResourceURN, p.BaselineURN, p.CandidateURN, string(ladder),
		p.MaxProportionDelta, p.MaxLatencyP95MsDelta, ap, string(p.Status), p.CurrentStep, nullTime(p.LastStepAt),
		p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *Store) GetPlan(ctx context.Context, id string) (*intelligence.CanaryPlan, error) {
	q := s.ph(`SELECT id, tenant_id, resource_urn, baseline_urn, candidate_urn, ladder_json, max_proportion_delta, max_latency_p95_ms_delta, auto_promote, status, current_step, last_step_at, failure_reason, created_at, updated_at FROM canary_plans WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	return scanPlan(row)
}

func (s *Store) ListPlans(ctx context.Context, tenant string, status intelligence.CanaryStatus) ([]intelligence.CanaryPlan, error) {
	args := []any{tenant}
	where := "tenant_id = ?"
	if status != "" {
		where += " AND status = ?"
		args = append(args, string(status))
	}
	q := s.ph(fmt.Sprintf(`SELECT id, tenant_id, resource_urn, baseline_urn, candidate_urn, ladder_json, max_proportion_delta, max_latency_p95_ms_delta, auto_promote, status, current_step, last_step_at, failure_reason, created_at, updated_at FROM canary_plans WHERE %s ORDER BY created_at DESC LIMIT 200`, where))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.CanaryPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListActive returns plans that aren't in a terminal state — used by the
// control loop.
func (s *Store) ListActive(ctx context.Context) ([]intelligence.CanaryPlan, error) {
	q := s.ph(`SELECT id, tenant_id, resource_urn, baseline_urn, candidate_urn, ladder_json, max_proportion_delta, max_latency_p95_ms_delta, auto_promote, status, current_step, last_step_at, failure_reason, created_at, updated_at FROM canary_plans WHERE status IN ('draft','ramping','holding','paused') LIMIT 500`)
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.CanaryPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ApplyDecision persists the state machine's verdict.
func (s *Store) ApplyDecision(ctx context.Context, planID string, d canary.Decision) error {
	now := time.Now().UTC()
	if d.NewStep < 0 {
		q := s.ph(`UPDATE canary_plans SET status = ?, failure_reason = ?, updated_at = ? WHERE id = ?`)
		_, err := s.DB.ExecContext(ctx, q, string(d.NewStatus), d.FailureReason, now, planID)
		return err
	}
	q := s.ph(`UPDATE canary_plans SET status = ?, current_step = ?, last_step_at = ?, failure_reason = ?, updated_at = ? WHERE id = ?`)
	_, err := s.DB.ExecContext(ctx, q, string(d.NewStatus), d.NewStep, now, d.FailureReason, now, planID)
	return err
}

// IngestObservation merges a single window into the time-series.
func (s *Store) IngestObservation(ctx context.Context, o intelligence.CanaryObservation) error {
	if s.IsPostgres {
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO canary_observations(plan_id, variant, window_start, window_end, successes, failures, latency_p50_ms, latency_p95_ms, latency_p99_ms)
             VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
             ON CONFLICT (plan_id, variant, window_start) DO UPDATE SET
               successes = canary_observations.successes + EXCLUDED.successes,
               failures = canary_observations.failures + EXCLUDED.failures,
               latency_p50_ms = GREATEST(canary_observations.latency_p50_ms, EXCLUDED.latency_p50_ms),
               latency_p95_ms = GREATEST(canary_observations.latency_p95_ms, EXCLUDED.latency_p95_ms),
               latency_p99_ms = GREATEST(canary_observations.latency_p99_ms, EXCLUDED.latency_p99_ms)`,
			o.PlanID, o.Variant, o.WindowStart, o.WindowEnd,
			o.Successes, o.Failures, o.LatencyP50Ms, o.LatencyP95Ms, o.LatencyP99Ms)
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO canary_observations(plan_id, variant, window_start, window_end, successes, failures, latency_p50_ms, latency_p95_ms, latency_p99_ms)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(plan_id, variant, window_start) DO UPDATE SET
           successes = successes + excluded.successes,
           failures = failures + excluded.failures,
           latency_p50_ms = MAX(latency_p50_ms, excluded.latency_p50_ms),
           latency_p95_ms = MAX(latency_p95_ms, excluded.latency_p95_ms),
           latency_p99_ms = MAX(latency_p99_ms, excluded.latency_p99_ms)`,
		o.PlanID, o.Variant, o.WindowStart, o.WindowEnd,
		o.Successes, o.Failures, o.LatencyP50Ms, o.LatencyP95Ms, o.LatencyP99Ms)
	return err
}

// AggregateSince returns the summed counts + max p95 latency for the
// given variant since `since`.
func (s *Store) AggregateSince(ctx context.Context, planID, variant string, since time.Time) (canary.Observation, error) {
	q := s.ph(`SELECT COALESCE(SUM(successes),0), COALESCE(SUM(failures),0), COALESCE(MAX(latency_p95_ms),0) FROM canary_observations WHERE plan_id = ? AND variant = ? AND window_end >= ?`)
	row := s.DB.QueryRowContext(ctx, q, planID, variant, since)
	var o canary.Observation
	err := row.Scan(&o.Successes, &o.Failures, &o.P95LatencyMs)
	return o, err
}

type rowScanner interface{ Scan(...any) error }

func scanPlan(row rowScanner) (*intelligence.CanaryPlan, error) {
	var p intelligence.CanaryPlan
	var ladder string
	var lastStep sql.NullTime
	var ap int
	if err := row.Scan(&p.ID, &p.TenantID, &p.ResourceURN, &p.BaselineURN, &p.CandidateURN,
		&ladder, &p.MaxProportionDelta, &p.MaxLatencyP95MsDelta, &ap, &p.Status,
		&p.CurrentStep, &lastStep, &p.FailureReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(ladder), &p.Ladder)
	p.AutoPromote = ap == 1
	if lastStep.Valid {
		p.LastStepAt = lastStep.Time
	}
	return &p, nil
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
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
