// Training-job store. Jobs are submitted to an external orchestrator
// (Argo Workflows, Ray, Kubeflow) — this service tracks lifecycle + metrics.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
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

const (
	StatePending    = "pending"
	StateRunning    = "running"
	StateSucceeded  = "succeeded"
	StateFailed     = "failed"
	StateCancelled  = "cancelled"
)

var ErrNotFound = errors.New("training: not found")

type Job struct {
	ID, TenantID, Name, BaseModel, DatasetID, DatasetVer string
	State                                                string
	EpochsDone, EpochsTotal                              int
	MetricsJSON, ArtifactURI, LastError                  string
	SubmittedAt                                          time.Time
	FinishedAt                                           sql.NullTime
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

func (s *Store) Submit(ctx context.Context, j Job) error {
	now := time.Now().UTC()
	if j.State == "" {
		j.State = StatePending
	}
	if j.MetricsJSON == "" {
		j.MetricsJSON = "{}"
	}
	q := s.ph(`INSERT INTO training_jobs(id, tenant_id, name, base_model, dataset_id, dataset_ver,
        state, epochs_done, epochs_total, metrics_json, artifact_uri, last_error, submitted_at, finished_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`)
	_, err := s.DB.ExecContext(ctx, q,
		j.ID, j.TenantID, j.Name, j.BaseModel, j.DatasetID, j.DatasetVer,
		j.State, j.EpochsDone, j.EpochsTotal, j.MetricsJSON, j.ArtifactURI, j.LastError, now)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (*Job, error) {
	q := s.ph(`SELECT id, tenant_id, name, base_model, dataset_id, dataset_ver, state,
        epochs_done, epochs_total, metrics_json, artifact_uri, last_error,
        submitted_at, finished_at FROM training_jobs WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var j Job
	err := row.Scan(&j.ID, &j.TenantID, &j.Name, &j.BaseModel, &j.DatasetID, &j.DatasetVer,
		&j.State, &j.EpochsDone, &j.EpochsTotal, &j.MetricsJSON, &j.ArtifactURI, &j.LastError,
		&j.SubmittedAt, &j.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &j, err
}

// Update transitions job state and updates progress.
func (s *Store) Update(ctx context.Context, id string, state string, epochsDone int, metrics, artifact, lastErr string) error {
	now := time.Now().UTC()
	finishedAt := sql.NullTime{}
	if state == StateSucceeded || state == StateFailed || state == StateCancelled {
		finishedAt = sql.NullTime{Time: now, Valid: true}
	}
	q := s.ph(`UPDATE training_jobs
        SET state = ?, epochs_done = ?, metrics_json = ?, artifact_uri = ?, last_error = ?, finished_at = ?
        WHERE id = ?`)
	res, err := s.DB.ExecContext(ctx, q, state, epochsDone, metrics, artifact, lastErr, finishedAt, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) List(ctx context.Context, tenant string) ([]Job, error) {
	q := s.ph(`SELECT id, tenant_id, name, base_model, dataset_id, dataset_ver, state,
        epochs_done, epochs_total, metrics_json, artifact_uri, last_error,
        submitted_at, finished_at FROM training_jobs WHERE tenant_id = ? ORDER BY submitted_at DESC`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.TenantID, &j.Name, &j.BaseModel, &j.DatasetID, &j.DatasetVer,
			&j.State, &j.EpochsDone, &j.EpochsTotal, &j.MetricsJSON, &j.ArtifactURI, &j.LastError,
			&j.SubmittedAt, &j.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
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
