// SQL-backed stream store. See pipeline-service/internal/store/sql.go for
// rationale on placeholder dialects + embed; the implementation mirrors
// that one.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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

type SQLStore struct {
	DB       *sql.DB
	Postgres bool
}

func NewSQLStore(db *sql.DB, postgres bool) *SQLStore { return &SQLStore{DB: db, Postgres: postgres} }

func (s *SQLStore) Create(in *controlv1.CreateStreamRequest) (*controlv1.Stream, error) {
	if in.GetName() == "" {
		return nil, errors.New("stream: name is required")
	}
	if in.GetTenant().GetId() == "" {
		return nil, errors.New("stream: tenant.id is required")
	}
	if in.GetProject().GetId() == "" {
		return nil, errors.New("stream: project.id is required")
	}
	if in.GetUrl() == "" {
		return nil, errors.New("stream: url is required")
	}
	redacted := redactURL(in.GetUrl())
	id := "stream-" + strings.ToLower(strings.ReplaceAll(in.GetName(), " ", "-"))
	now := time.Now().UTC()

	q := s.placeholders(`INSERT INTO streams
        (id, tenant_id, project_id, camera_id, site_id, name, protocol,
         url_redacted, health, observed_fps, observed_kbps, version,
         created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 1, ?, ?)`)
	_, err := s.DB.ExecContext(context.Background(), q,
		id, in.GetTenant().GetId(), in.GetProject().GetId(),
		in.GetCameraId(), in.GetSiteId(), in.GetName(),
		int(in.GetProtocol()), redacted,
		int(controlv1.StreamHealth_STREAM_HEALTH_DISCONNECTED),
		now, now,
	)
	if err != nil {
		return nil, err
	}
	return &controlv1.Stream{
		Id:          id,
		Tenant:      in.GetTenant(),
		Project:     in.GetProject(),
		CameraId:    in.GetCameraId(),
		SiteId:      in.GetSiteId(),
		Name:        in.GetName(),
		Protocol:    in.GetProtocol(),
		UrlRedacted: redacted,
		Health:      controlv1.StreamHealth_STREAM_HEALTH_DISCONNECTED,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(now),
			UpdatedAt: timestamppb.New(now),
			Version:   1,
		},
	}, nil
}

func (s *SQLStore) Get(id string) (*controlv1.Stream, error) {
	q := s.placeholders(`SELECT id, tenant_id, project_id, camera_id, site_id, name,
        protocol, url_redacted, health, observed_fps, observed_kbps, version,
        created_at, updated_at FROM streams WHERE id = ?`)
	row := s.DB.QueryRowContext(context.Background(), q, id)
	return s.scan(row)
}

func (s *SQLStore) Delete(id string) error {
	q := s.placeholders(`DELETE FROM streams WHERE id = ?`)
	res, err := s.DB.ExecContext(context.Background(), q, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) UpdateHealth(id string, h controlv1.StreamHealth, fps float64, kbps int64) (*controlv1.Stream, error) {
	now := time.Now().UTC()
	q := s.placeholders(`UPDATE streams
        SET health=?, observed_fps=?, observed_kbps=?, last_frame_at=?, updated_at=?, version=version+1
        WHERE id=?`)
	res, err := s.DB.ExecContext(context.Background(), q, int(h), fps, kbps, now, now, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(id)
}

func (s *SQLStore) List(tenantID, projectID, siteID string, health []controlv1.StreamHealth, pageSize int, after string) ([]*controlv1.Stream, string, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, tenant_id, project_id, camera_id, site_id, name,
        protocol, url_redacted, health, observed_fps, observed_kbps, version,
        created_at, updated_at FROM streams WHERE tenant_id = ?`)
	args := []any{tenantID}
	if projectID != "" {
		sb.WriteString(` AND project_id = ?`)
		args = append(args, projectID)
	}
	if siteID != "" {
		sb.WriteString(` AND site_id = ?`)
		args = append(args, siteID)
	}
	if len(health) > 0 {
		sb.WriteString(` AND health IN (`)
		for i, h := range health {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("?")
			args = append(args, int(h))
		}
		sb.WriteString(`)`)
	}
	if after != "" {
		sb.WriteString(` AND id > ?`)
		args = append(args, after)
	}
	sb.WriteString(` ORDER BY id ASC LIMIT ?`)
	args = append(args, pageSize+1)
	rows, err := s.DB.QueryContext(context.Background(), s.placeholders(sb.String()), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []*controlv1.Stream{}
	for rows.Next() {
		st, err := s.scanRow(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > pageSize {
		out = out[:pageSize]
		next = out[len(out)-1].GetId()
	}
	return out, next, nil
}

func (s *SQLStore) scan(row *sql.Row) (*controlv1.Stream, error) {
	var (
		id, tenantID, projectID, cameraID, siteID, name, urlRedacted string
		protocol, health                                             int
		observedFPS                                                  float64
		observedKbps, version                                        int64
		createdAt, updatedAt                                         time.Time
	)
	err := row.Scan(&id, &tenantID, &projectID, &cameraID, &siteID, &name,
		&protocol, &urlRedacted, &health, &observedFPS, &observedKbps,
		&version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assembleStream(id, tenantID, projectID, cameraID, siteID, name,
		protocol, urlRedacted, health, observedFPS, observedKbps, version,
		createdAt, updatedAt), nil
}

func (s *SQLStore) scanRow(rows *sql.Rows) (*controlv1.Stream, error) {
	var (
		id, tenantID, projectID, cameraID, siteID, name, urlRedacted string
		protocol, health                                             int
		observedFPS                                                  float64
		observedKbps, version                                        int64
		createdAt, updatedAt                                         time.Time
	)
	if err := rows.Scan(&id, &tenantID, &projectID, &cameraID, &siteID, &name,
		&protocol, &urlRedacted, &health, &observedFPS, &observedKbps,
		&version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return assembleStream(id, tenantID, projectID, cameraID, siteID, name,
		protocol, urlRedacted, health, observedFPS, observedKbps, version,
		createdAt, updatedAt), nil
}

func assembleStream(id, tenantID, projectID, cameraID, siteID, name string,
	protocol int, urlRedacted string, health int, fps float64,
	kbps, version int64, createdAt, updatedAt time.Time) *controlv1.Stream {
	return &controlv1.Stream{
		Id:           id,
		Tenant:       &commonv1.TenantRef{Id: tenantID},
		Project:      &commonv1.ProjectRef{Id: projectID, TenantId: tenantID},
		CameraId:     cameraID,
		SiteId:       siteID,
		Name:         name,
		Protocol:     controlv1.StreamProtocol(protocol),
		UrlRedacted:  urlRedacted,
		Health:       controlv1.StreamHealth(health),
		ObservedFps:  fps,
		ObservedKbps: kbps,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(createdAt),
			UpdatedAt: timestamppb.New(updatedAt),
			Version:   version,
		},
	}
}

func (s *SQLStore) placeholders(q string) string {
	if !s.Postgres {
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

// redactURL strips embedded credentials and query strings. Mirrors the
// in-memory implementation so behavior is identical across backends.
func redactURLSQL(in string) string { return redactURL(in) }

var _ = url.Parse // pin reference so the import stays even if redactURL moves
