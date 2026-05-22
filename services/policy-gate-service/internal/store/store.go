// Gate persistence. Append-only-with-decision-mutation semantics: the
// request fields are immutable after creation; only decision_*, decided_*
// can be updated. This is enforced by Decide() — there is no general
// UPDATE entry point.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
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

// Open creates a new gate row with decision=pending.
func (s *Store) Open(ctx context.Context, g intelligence.Gate) (string, error) {
	if g.Request.ID == "" {
		return "", errors.New("gate id required")
	}
	if g.Request.RequestedAt.IsZero() {
		g.Request.RequestedAt = time.Now().UTC()
	}
	q := s.ph(`INSERT INTO policy_gates(id, session_id, tenant_id, tool_name, action_summary, arguments_json, blast_radius, decision, requested_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		g.Request.ID, g.Request.SessionID, g.Request.TenantID, g.Request.ToolName,
		g.Request.ActionSummary, g.Request.ArgumentsJSON, g.Request.BlastRadius,
		g.Request.RequestedAt)
	return g.Request.ID, err
}

// Get returns the current state.
func (s *Store) Get(ctx context.Context, id string) (*intelligence.Gate, error) {
	q := s.ph(`SELECT id, session_id, tenant_id, tool_name, action_summary, arguments_json, blast_radius,
        decision, decided_by, decision_reason, requested_at, decided_at FROM policy_gates WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	return scanGate(row)
}

// List returns pending gates for a tenant (most-recent first). limit clamps
// to 200.
func (s *Store) ListPending(ctx context.Context, tenant string, limit int) ([]intelligence.Gate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.ph(fmt.Sprintf(`SELECT id, session_id, tenant_id, tool_name, action_summary, arguments_json, blast_radius,
        decision, decided_by, decision_reason, requested_at, decided_at
        FROM policy_gates WHERE tenant_id = ? AND decision = 'pending'
        ORDER BY requested_at DESC LIMIT %d`, limit))
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.Gate
	for rows.Next() {
		g, err := scanGate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// Decide resolves a pending gate. Returns (gate, true) if state changed,
// (gate, false) if the gate was already non-pending (idempotent).
func (s *Store) Decide(ctx context.Context, id string, decision intelligence.GateDecision, decidedBy, reason string) (*intelligence.Gate, bool, error) {
	if decision != intelligence.GateDecisionApproved && decision != intelligence.GateDecisionRejected && decision != intelligence.GateDecisionExpired {
		return nil, false, errors.New("invalid decision")
	}
	now := time.Now().UTC()
	q := s.ph(`UPDATE policy_gates SET decision = ?, decided_by = ?, decision_reason = ?, decided_at = ?
        WHERE id = ? AND decision = 'pending'`)
	res, err := s.DB.ExecContext(ctx, q, string(decision), decidedBy, reason, now, id)
	if err != nil {
		return nil, false, err
	}
	g, gerr := s.Get(ctx, id)
	if gerr != nil {
		return nil, false, gerr
	}
	n, _ := res.RowsAffected()
	return g, n > 0, nil
}

type rowScanner interface{ Scan(...any) error }

func scanGate(row rowScanner) (*intelligence.Gate, error) {
	var g intelligence.Gate
	var decidedAt sql.NullTime
	if err := row.Scan(
		&g.Request.ID, &g.Request.SessionID, &g.Request.TenantID, &g.Request.ToolName,
		&g.Request.ActionSummary, &g.Request.ArgumentsJSON, &g.Request.BlastRadius,
		&g.Decision, &g.DecidedBy, &g.DecisionReason, &g.Request.RequestedAt, &decidedAt,
	); err != nil {
		return nil, err
	}
	if decidedAt.Valid {
		g.DecidedAt = decidedAt.Time
	}
	return &g, nil
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
