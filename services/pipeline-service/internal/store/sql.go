// SQL-backed pipeline store. Postgres in production, SQLite in tests.
//
// Cross-driver placeholders: pgx uses $1..$N, sqlite uses ?. We render the
// placeholders at query-construction time based on the driver.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed *.sql
var migrationsFS embed.FS

// MigrateUp applies the pipeline-service migrations to db. Call on startup.
func MigrateUp(ctx context.Context, db *sql.DB) error {
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

// SQLStore implements Store against database/sql.
type SQLStore struct {
	DB       *sql.DB
	Postgres bool // selects placeholder dialect
}

// NewSQLStore constructs the store. If postgres is true placeholders render
// as $N, otherwise as ?.
func NewSQLStore(db *sql.DB, postgres bool) *SQLStore {
	return &SQLStore{DB: db, Postgres: postgres}
}

func (s *SQLStore) Create(p *controlv1.Pipeline) (*controlv1.Pipeline, error) {
	if p.GetId() == "" {
		return nil, errors.New("pipeline: id is required")
	}
	if p.Status == controlv1.PipelineStatus_PIPELINE_STATUS_UNSPECIFIED {
		p.Status = controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT
	}
	now := time.Now().UTC()
	if p.Audit == nil {
		p.Audit = &commonv1.AuditFields{}
	}
	p.Audit.CreatedAt = timestamppb.New(now)
	p.Audit.UpdatedAt = timestamppb.New(now)
	p.Audit.Version = 1

	labels, _ := json.Marshal(p.GetLabels())
	q := s.placeholders(`INSERT INTO pipelines
        (id, tenant_id, project_id, name, description, labels_json, status,
         current_revision, dag_json, version, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(context.Background(), q,
		p.GetId(), p.GetTenant().GetId(), p.GetProject().GetId(),
		p.GetName(), p.GetDescription(), string(labels), int(p.Status),
		p.GetCurrentRevision(), p.GetDagJson(), p.Audit.Version, now, now,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *SQLStore) Get(id string) (*controlv1.Pipeline, error) {
	q := s.placeholders(`SELECT id, tenant_id, project_id, name, description,
        labels_json, status, current_revision, dag_json, version,
        created_at, updated_at FROM pipelines WHERE id = ?`)
	row := s.DB.QueryRowContext(context.Background(), q, id)
	return s.scan(row)
}

func (s *SQLStore) UpdateStatus(id string, status controlv1.PipelineStatus) (*controlv1.Pipeline, error) {
	now := time.Now().UTC()
	q := s.placeholders(`UPDATE pipelines
        SET status = ?, updated_at = ?, version = version + 1
        WHERE id = ?`)
	res, err := s.DB.ExecContext(context.Background(), q, int(status), now, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(id)
}

func (s *SQLStore) List(f ListFilter, qry PageQuery) (PageResult, error) {
	if qry.PageSize <= 0 {
		qry.PageSize = 50
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, tenant_id, project_id, name, description,
        labels_json, status, current_revision, dag_json, version,
        created_at, updated_at FROM pipelines WHERE 1=1`)
	args := []any{}
	if f.TenantID != "" {
		sb.WriteString(` AND tenant_id = ?`)
		args = append(args, f.TenantID)
	}
	if f.ProjectID != "" {
		sb.WriteString(` AND project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if f.NameContains != "" {
		sb.WriteString(` AND name LIKE ?`)
		args = append(args, "%"+f.NameContains+"%")
	}
	if len(f.StatusIn) > 0 {
		sb.WriteString(` AND status IN (`)
		for i, st := range f.StatusIn {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("?")
			args = append(args, int(st))
		}
		sb.WriteString(`)`)
	}
	if qry.AfterID != "" {
		sb.WriteString(` AND id > ?`)
		args = append(args, qry.AfterID)
	}
	sb.WriteString(` ORDER BY id ASC LIMIT ?`)
	args = append(args, qry.PageSize+1) // peek for next-page

	rows, err := s.DB.QueryContext(context.Background(), s.placeholders(sb.String()), args...)
	if err != nil {
		return PageResult{}, err
	}
	defer rows.Close()
	out := []*controlv1.Pipeline{}
	for rows.Next() {
		p, err := s.scanRow(rows)
		if err != nil {
			return PageResult{}, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return PageResult{}, err
	}
	next := ""
	if len(out) > qry.PageSize {
		// Drop the peeked extra row; the previous one is the cursor.
		out = out[:qry.PageSize]
		next = out[len(out)-1].GetId()
	}
	return PageResult{Items: out, NextAfter: next, Total: -1}, nil
}

func (s *SQLStore) scan(row *sql.Row) (*controlv1.Pipeline, error) {
	var (
		id, tenantID, projectID, name, desc, labelsJSON, dagJSON string
		status                                                   int
		currentRevision, version                                 int64
		createdAt, updatedAt                                     time.Time
	)
	err := row.Scan(&id, &tenantID, &projectID, &name, &desc, &labelsJSON,
		&status, &currentRevision, &dagJSON, &version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assemble(id, tenantID, projectID, name, desc, labelsJSON, dagJSON,
		status, currentRevision, version, createdAt, updatedAt), nil
}

func (s *SQLStore) scanRow(rows *sql.Rows) (*controlv1.Pipeline, error) {
	var (
		id, tenantID, projectID, name, desc, labelsJSON, dagJSON string
		status                                                   int
		currentRevision, version                                 int64
		createdAt, updatedAt                                     time.Time
	)
	if err := rows.Scan(&id, &tenantID, &projectID, &name, &desc, &labelsJSON,
		&status, &currentRevision, &dagJSON, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return assemble(id, tenantID, projectID, name, desc, labelsJSON, dagJSON,
		status, currentRevision, version, createdAt, updatedAt), nil
}

func assemble(id, tenantID, projectID, name, desc, labelsJSON, dagJSON string,
	status int, rev, version int64, createdAt, updatedAt time.Time) *controlv1.Pipeline {
	var labels []string
	_ = json.Unmarshal([]byte(labelsJSON), &labels)
	return &controlv1.Pipeline{
		Id:              id,
		Tenant:          &commonv1.TenantRef{Id: tenantID},
		Project:         &commonv1.ProjectRef{Id: projectID, TenantId: tenantID},
		Name:            name,
		Description:     desc,
		Labels:          labels,
		Status:          controlv1.PipelineStatus(status),
		CurrentRevision: rev,
		DagJson:         dagJSON,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(createdAt),
			UpdatedAt: timestamppb.New(updatedAt),
			Version:   version,
		},
	}
}

// placeholders renames "?" to "$1..$N" when Postgres is configured.
func (s *SQLStore) placeholders(q string) string {
	if !s.Postgres {
		return q
	}
	var out strings.Builder
	out.Grow(len(q) + 8)
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

