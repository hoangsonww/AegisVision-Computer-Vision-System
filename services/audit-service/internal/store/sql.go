// Audit store. Append-only by design — audit records do not get UPDATEd.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
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

// Record mirrors middleware.AuditRecord JSON shape.
type Record struct {
	ID, Service, TenantID, RequestID  string
	Method, Path, Outcome             string
	Status                            int
	UserAgent, RemoteIP, Verb, Resource string
	At                                time.Time
	Raw                               string
}

type Store struct {
	DB       *sql.DB
	Postgres bool
}

func New(db *sql.DB, postgres bool) *Store { return &Store{DB: db, Postgres: postgres} }

func (s *Store) Append(ctx context.Context, r Record) error {
	q := s.ph(`INSERT INTO audit_records(id, at, service, tenant_id, request_id, method, path,
        status, outcome, user_agent, remote_ip, verb, resource, raw_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		r.ID, r.At, r.Service, r.TenantID, r.RequestID, r.Method, r.Path,
		r.Status, r.Outcome, r.UserAgent, r.RemoteIP, r.Verb, r.Resource, r.Raw)
	return err
}

// AppendJSON parses a JSON-encoded audit record and appends it.
func (s *Store) AppendJSON(ctx context.Context, body []byte) error {
	var r struct {
		ID        string    `json:"id"`
		At        time.Time `json:"at"`
		Service   string    `json:"service"`
		TenantID  string    `json:"tenant_id"`
		RequestID string    `json:"request_id"`
		Method    string    `json:"method"`
		Path      string    `json:"path"`
		Status    int       `json:"status"`
		Outcome   string    `json:"outcome"`
		UserAgent string    `json:"user_agent"`
		RemoteIP  string    `json:"remote_ip"`
		Verb      string    `json:"verb"`
		Resource  string    `json:"resource"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return err
	}
	if r.ID == "" {
		return fmt.Errorf("audit: missing id")
	}
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	return s.Append(ctx, Record{
		ID: r.ID, At: r.At, Service: r.Service,
		TenantID: r.TenantID, RequestID: r.RequestID,
		Method: r.Method, Path: r.Path, Status: r.Status,
		Outcome: r.Outcome, UserAgent: r.UserAgent, RemoteIP: r.RemoteIP,
		Verb: r.Verb, Resource: r.Resource, Raw: string(body),
	})
}

// List returns the most recent N records matching the filter.
func (s *Store) List(ctx context.Context, tenantID, outcome string, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 100
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, at, service, tenant_id, request_id, method, path,
        status, outcome, user_agent, remote_ip, verb, resource, raw_json
        FROM audit_records WHERE 1=1`)
	args := []any{}
	if tenantID != "" {
		sb.WriteString(` AND tenant_id = ?`)
		args = append(args, tenantID)
	}
	if outcome != "" {
		sb.WriteString(` AND outcome = ?`)
		args = append(args, outcome)
	}
	sb.WriteString(` ORDER BY at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, s.ph(sb.String()), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.At, &r.Service, &r.TenantID, &r.RequestID,
			&r.Method, &r.Path, &r.Status, &r.Outcome, &r.UserAgent, &r.RemoteIP,
			&r.Verb, &r.Resource, &r.Raw); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
