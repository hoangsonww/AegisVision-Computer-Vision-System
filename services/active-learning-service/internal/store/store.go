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

func (s *Store) UpsertPolicy(ctx context.Context, p intelligence.LearningPolicy) error {
	div, _ := json.Marshal(p.DiversityClasses)
	if s.IsPostgres {
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO learning_policies(id, tenant_id, model_id, strategy, sample_rate, min_uncertainty, max_uncertainty, diversity_classes, daily_budget, snapshot_ttl_days)
             VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)
             ON CONFLICT (id) DO UPDATE SET model_id=EXCLUDED.model_id, strategy=EXCLUDED.strategy, sample_rate=EXCLUDED.sample_rate,
               min_uncertainty=EXCLUDED.min_uncertainty, max_uncertainty=EXCLUDED.max_uncertainty,
               diversity_classes=EXCLUDED.diversity_classes, daily_budget=EXCLUDED.daily_budget, snapshot_ttl_days=EXCLUDED.snapshot_ttl_days`,
			p.ID, p.TenantID, p.ModelID, string(p.Strategy), p.SampleRate, p.MinUncertainty, p.MaxUncertainty,
			string(div), p.DailyBudget, p.SnapshotTTLDays)
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO learning_policies(id, tenant_id, model_id, strategy, sample_rate, min_uncertainty, max_uncertainty, diversity_classes, daily_budget, snapshot_ttl_days)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(id) DO UPDATE SET model_id=excluded.model_id, strategy=excluded.strategy, sample_rate=excluded.sample_rate,
           min_uncertainty=excluded.min_uncertainty, max_uncertainty=excluded.max_uncertainty,
           diversity_classes=excluded.diversity_classes, daily_budget=excluded.daily_budget, snapshot_ttl_days=excluded.snapshot_ttl_days`,
		p.ID, p.TenantID, p.ModelID, string(p.Strategy), p.SampleRate, p.MinUncertainty, p.MaxUncertainty,
		string(div), p.DailyBudget, p.SnapshotTTLDays)
	return err
}

func (s *Store) GetPolicy(ctx context.Context, id string) (*intelligence.LearningPolicy, error) {
	q := s.ph(`SELECT id, tenant_id, model_id, strategy, sample_rate, min_uncertainty, max_uncertainty, diversity_classes, daily_budget, snapshot_ttl_days FROM learning_policies WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var p intelligence.LearningPolicy
	var divRaw string
	if err := row.Scan(&p.ID, &p.TenantID, &p.ModelID, &p.Strategy, &p.SampleRate, &p.MinUncertainty, &p.MaxUncertainty, &divRaw, &p.DailyBudget, &p.SnapshotTTLDays); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(divRaw), &p.DiversityClasses)
	return &p, nil
}

func (s *Store) ListPoliciesByModel(ctx context.Context, tenant, modelID string) ([]intelligence.LearningPolicy, error) {
	q := s.ph(`SELECT id, tenant_id, model_id, strategy, sample_rate, min_uncertainty, max_uncertainty, diversity_classes, daily_budget, snapshot_ttl_days FROM learning_policies WHERE tenant_id = ? AND model_id = ?`)
	rows, err := s.DB.QueryContext(ctx, q, tenant, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.LearningPolicy
	for rows.Next() {
		var p intelligence.LearningPolicy
		var divRaw string
		if err := rows.Scan(&p.ID, &p.TenantID, &p.ModelID, &p.Strategy, &p.SampleRate, &p.MinUncertainty, &p.MaxUncertainty, &divRaw, &p.DailyBudget, &p.SnapshotTTLDays); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(divRaw), &p.DiversityClasses)
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- Samples ---

func (s *Store) InsertSample(ctx context.Context, sm intelligence.Sample) error {
	if sm.SampledAt.IsZero() {
		sm.SampledAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO learning_samples(id, tenant_id, model_id, policy_id, snapshot_urn, predicted_class, predicted_confidence, uncertainty, status, annotation_task_id, sampled_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`)
	_, err := s.DB.ExecContext(ctx, q, sm.ID, sm.TenantID, sm.ModelID, sm.PolicyID, sm.SnapshotURN,
		sm.PredictedClass, sm.PredictedConfidence, sm.Uncertainty, string(sm.Status), sm.SampledAt)
	return err
}

func (s *Store) AssignSample(ctx context.Context, sampleID, annotationTaskID string) error {
	q := s.ph(`UPDATE learning_samples SET status = 'assigned', annotation_task_id = ? WHERE id = ? AND status = 'queued'`)
	_, err := s.DB.ExecContext(ctx, q, annotationTaskID, sampleID)
	return err
}

func (s *Store) MarkLabeled(ctx context.Context, sampleID, finalClass string) error {
	q := s.ph(`UPDATE learning_samples SET status = 'labeled', final_class = ?, labeled_at = ? WHERE id = ?`)
	_, err := s.DB.ExecContext(ctx, q, finalClass, time.Now().UTC(), sampleID)
	return err
}

func (s *Store) ListSamples(ctx context.Context, tenant, statusFilter string, limit int) ([]intelligence.Sample, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{tenant}
	where := "tenant_id = ?"
	if statusFilter != "" {
		where += " AND status = ?"
		args = append(args, statusFilter)
	}
	q := s.ph(fmt.Sprintf(`SELECT id, tenant_id, model_id, policy_id, snapshot_urn, predicted_class, predicted_confidence, uncertainty, status, annotation_task_id, final_class, sampled_at, labeled_at FROM learning_samples WHERE %s ORDER BY sampled_at DESC LIMIT %d`, where, limit))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.Sample
	for rows.Next() {
		var sm intelligence.Sample
		var labeledAt sql.NullTime
		if err := rows.Scan(&sm.ID, &sm.TenantID, &sm.ModelID, &sm.PolicyID, &sm.SnapshotURN,
			&sm.PredictedClass, &sm.PredictedConfidence, &sm.Uncertainty, &sm.Status,
			&sm.AnnotationTaskID, &sm.FinalClass, &sm.SampledAt, &labeledAt); err != nil {
			return nil, err
		}
		if labeledAt.Valid {
			sm.LabeledAt = labeledAt.Time
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// CountToday returns the number of samples queued/assigned/labeled for the
// given policy in the last 24h (used as an idempotent budget check on top
// of the in-memory Budgeter).
func (s *Store) CountToday(ctx context.Context, policyID string) (int, error) {
	since := time.Now().Add(-24 * time.Hour).UTC()
	q := s.ph(`SELECT COUNT(1) FROM learning_samples WHERE policy_id = ? AND sampled_at >= ?`)
	var n int
	err := s.DB.QueryRowContext(ctx, q, policyID, since).Scan(&n)
	return n, err
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
