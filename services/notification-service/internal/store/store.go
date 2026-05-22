// Endpoint and DLQ store for notification-service.
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

type Endpoint struct {
	ID, TenantID, URL, Secret string
	Enabled                   bool
	CreatedAt, UpdatedAt      time.Time
}

type DLQEntry struct {
	ID, EndpointID, TenantID string
	Body                     []byte
	Attempts                 int
	LastError                string
	FailedAt                 time.Time
}

type Store struct {
	DB       *sql.DB
	Postgres bool
}

func New(db *sql.DB, postgres bool) *Store { return &Store{DB: db, Postgres: postgres} }

var ErrNotFound = errors.New("endpoint: not found")

func (s *Store) CreateEndpoint(ctx context.Context, e Endpoint) error {
	now := time.Now().UTC()
	enabled := 1
	if !e.Enabled {
		enabled = 0
	}
	q := s.ph(`INSERT INTO webhook_endpoints(id, tenant_id, url, secret, enabled, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, e.ID, e.TenantID, e.URL, e.Secret, enabled, now, now)
	return err
}

func (s *Store) ListByTenant(ctx context.Context, tenant string) ([]Endpoint, error) {
	q := s.ph(`SELECT id, tenant_id, url, secret, enabled, created_at, updated_at
        FROM webhook_endpoints WHERE tenant_id = ? AND enabled = 1`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Endpoint
	for rows.Next() {
		var e Endpoint
		var en int
		if err := rows.Scan(&e.ID, &e.TenantID, &e.URL, &e.Secret, &en, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Enabled = en != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) RecordDLQ(ctx context.Context, e DLQEntry) error {
	q := s.ph(`INSERT INTO webhook_dlq(id, endpoint_id, tenant_id, body, attempts, last_error, failed_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		e.ID, e.EndpointID, e.TenantID, string(e.Body), e.Attempts, e.LastError, e.FailedAt)
	return err
}

func (s *Store) ph(q string) string {
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
