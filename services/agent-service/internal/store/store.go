// Persistence for agent sessions + per-step trail.
//
// We persist enough to:
//
//   - resume a session after policy-gate-service approves a gate
//     (the session's last messages + tool calls are reconstructed from steps)
//   - audit every step (input -> step -> result -> next step)
//   - bound per-tenant usage via aggregate queries
//
// Steps are append-only; session status is monotonic — we never "un-fail"
// a session.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/agent"
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

func (s *Store) CreateSession(ctx context.Context, sess intelligence.AgentSession) error {
	now := time.Now().UTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	sess.UpdatedAt = now
	q := s.ph(`INSERT INTO agent_sessions(id, tenant_id, role, goal, status, step_count, max_steps, gate_id, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		sess.ID, sess.TenantID, string(sess.Role), sess.Goal, string(sess.Status),
		sess.StepCount, sess.MaxSteps, sess.CreatedAt, sess.UpdatedAt)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*intelligence.AgentSession, string, error) {
	q := s.ph(`SELECT id, tenant_id, role, goal, status, step_count, max_steps, gate_id, created_at, updated_at FROM agent_sessions WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var sess intelligence.AgentSession
	var gateID string
	if err := row.Scan(&sess.ID, &sess.TenantID, &sess.Role, &sess.Goal, &sess.Status,
		&sess.StepCount, &sess.MaxSteps, &gateID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		return nil, "", err
	}
	return &sess, gateID, nil
}

// SetStatus marks the session's status + optional gate id (only set when
// transitioning to "awaiting_gate").
func (s *Store) SetStatus(ctx context.Context, sessionID, status, gateID string) error {
	q := s.ph(`UPDATE agent_sessions SET status = ?, gate_id = ?, updated_at = ? WHERE id = ?`)
	_, err := s.DB.ExecContext(ctx, q, status, gateID, time.Now().UTC(), sessionID)
	return err
}

func (s *Store) AppendStep(ctx context.Context, sessionID string, st agent.Step) error {
	pj, _ := json.Marshal(st.Payload)
	if st.CreatedAt.IsZero() {
		st.CreatedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO agent_steps(session_id, idx, kind, summary, payload_json, gate_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, sessionID, st.Index, st.Kind, st.Summary, string(pj), st.GateID, st.CreatedAt)
	if err != nil {
		return err
	}
	// Best-effort step count bump (raceless because of session-id stickiness).
	_, _ = s.DB.ExecContext(ctx, s.ph(`UPDATE agent_sessions SET step_count = ?, updated_at = ? WHERE id = ?`), st.Index+1, time.Now().UTC(), sessionID)
	return nil
}

func (s *Store) ListSteps(ctx context.Context, sessionID string) ([]intelligence.AgentStep, error) {
	q := s.ph(`SELECT idx, kind, summary, payload_json, gate_id, created_at FROM agent_steps WHERE session_id = ? ORDER BY idx ASC`)
	rows, err := s.DB.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.AgentStep
	for rows.Next() {
		var st intelligence.AgentStep
		if err := rows.Scan(&st.Index, &st.Kind, &st.Summary, &st.PayloadJSON, &st.GateID, &st.CreatedAt); err != nil {
			return nil, err
		}
		st.SessionID = sessionID
		st.ID = fmt.Sprintf("%s/%d", sessionID, st.Index)
		out = append(out, st)
	}
	return out, rows.Err()
}

// ListSessions for a tenant, most-recent first.
func (s *Store) ListSessions(ctx context.Context, tenant string, limit int) ([]intelligence.AgentSession, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.ph(fmt.Sprintf(`SELECT id, tenant_id, role, goal, status, step_count, max_steps, created_at, updated_at FROM agent_sessions WHERE tenant_id = ? ORDER BY created_at DESC LIMIT %d`, limit))
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.AgentSession
	for rows.Next() {
		var ses intelligence.AgentSession
		if err := rows.Scan(&ses.ID, &ses.TenantID, &ses.Role, &ses.Goal, &ses.Status, &ses.StepCount, &ses.MaxSteps, &ses.CreatedAt, &ses.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ses)
	}
	return out, rows.Err()
}

// Sink adapts Store to pkg/agent.SessionSink so the loop can persist.
type Sink struct{ S *Store }

func (a Sink) AppendStep(ctx context.Context, sessionID string, st agent.Step) error {
	return a.S.AppendStep(ctx, sessionID, st)
}

func (a Sink) SetStatus(ctx context.Context, sessionID, status string) error {
	return a.S.SetStatus(ctx, sessionID, status, "")
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
