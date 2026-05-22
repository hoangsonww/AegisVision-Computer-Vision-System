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

func (s *Store) UpsertSchedule(ctx context.Context, sc intelligence.Schedule) error {
	now := time.Now().UTC()
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = now
	}
	sc.UpdatedAt = now
	paused := 0
	if sc.Paused {
		paused = 1
	}
	q := s.ph(`INSERT INTO autonomy_schedules(id, tenant_id, role, trigger, cron_expression, signal_subject, max_steps, max_autonomy_tier, paused, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET role = excluded.role, trigger = excluded.trigger,
          cron_expression = excluded.cron_expression, signal_subject = excluded.signal_subject,
          max_steps = excluded.max_steps, max_autonomy_tier = excluded.max_autonomy_tier,
          paused = excluded.paused, updated_at = excluded.updated_at`)
	if s.IsPostgres {
		q = strings.ReplaceAll(q, "excluded.", "EXCLUDED.")
	}
	_, err := s.DB.ExecContext(ctx, q,
		sc.ID, sc.TenantID, string(sc.Role), string(sc.Trigger), sc.CronExpression, sc.SignalSubject,
		sc.MaxSteps, sc.MaxAutonomyTier, paused, sc.CreatedAt, sc.UpdatedAt)
	return err
}

func (s *Store) ListSchedules(ctx context.Context) ([]intelligence.Schedule, error) {
	q := s.ph(`SELECT id, tenant_id, role, trigger, cron_expression, signal_subject, max_steps, max_autonomy_tier, paused, created_at, updated_at FROM autonomy_schedules ORDER BY tenant_id, id LIMIT 1000`)
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.Schedule
	for rows.Next() {
		var sc intelligence.Schedule
		var paused int
		if err := rows.Scan(&sc.ID, &sc.TenantID, &sc.Role, &sc.Trigger, &sc.CronExpression, &sc.SignalSubject,
			&sc.MaxSteps, &sc.MaxAutonomyTier, &paused, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		sc.Paused = paused == 1
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *Store) GetSchedule(ctx context.Context, id string) (*intelligence.Schedule, error) {
	q := s.ph(`SELECT id, tenant_id, role, trigger, cron_expression, signal_subject, max_steps, max_autonomy_tier, paused, created_at, updated_at FROM autonomy_schedules WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var sc intelligence.Schedule
	var paused int
	if err := row.Scan(&sc.ID, &sc.TenantID, &sc.Role, &sc.Trigger, &sc.CronExpression, &sc.SignalSubject,
		&sc.MaxSteps, &sc.MaxAutonomyTier, &paused, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return nil, err
	}
	sc.Paused = paused == 1
	return &sc, nil
}

func (s *Store) InsertRun(ctx context.Context, r intelligence.AutonomyRun) error {
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO autonomy_runs(id, schedule_id, tenant_id, role, trigger, status, session_id, trigger_reason, started_at, gate_id)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, r.ID, r.ScheduleID, r.TenantID, string(r.Role), string(r.Trigger),
		string(r.Status), r.SessionID, r.TriggerReason, r.StartedAt, r.GateID)
	return err
}

func (s *Store) UpdateRunStatus(ctx context.Context, id string, status intelligence.RunStatus, sessionID, gateID string) error {
	finished := sql.NullTime{}
	if status == intelligence.RunStatusCompleted || status == intelligence.RunStatusFailed || status == intelligence.RunStatusSkipped {
		finished = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	}
	q := s.ph(`UPDATE autonomy_runs SET status = ?, session_id = COALESCE(NULLIF(?, ''), session_id), gate_id = COALESCE(NULLIF(?, ''), gate_id), finished_at = ? WHERE id = ?`)
	_, err := s.DB.ExecContext(ctx, q, string(status), sessionID, gateID, finished, id)
	return err
}

func (s *Store) ListRuns(ctx context.Context, tenant string, limit int) ([]intelligence.AutonomyRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.ph(fmt.Sprintf(`SELECT id, schedule_id, tenant_id, role, trigger, status, session_id, trigger_reason, started_at, finished_at, gate_id FROM autonomy_runs WHERE tenant_id = ? ORDER BY started_at DESC LIMIT %d`, limit))
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.AutonomyRun
	for rows.Next() {
		var r intelligence.AutonomyRun
		var finished sql.NullTime
		if err := rows.Scan(&r.ID, &r.ScheduleID, &r.TenantID, &r.Role, &r.Trigger, &r.Status, &r.SessionID, &r.TriggerReason, &r.StartedAt, &finished, &r.GateID); err != nil {
			return nil, err
		}
		if finished.Valid {
			r.FinishedAt = finished.Time
		}
		out = append(out, r)
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
