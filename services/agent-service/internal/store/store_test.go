package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/agent"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/agent-service/internal/store"
)

func TestStore_SessionAndSteps(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	if err := s.CreateSession(context.Background(), intelligence.AgentSession{
		ID: "sess-1", TenantID: "t-1", Role: intelligence.AgentRoleOperator,
		Goal: "investigate", Status: intelligence.SessionStatusActive, MaxSteps: 12,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_ = s.AppendStep(context.Background(), "sess-1", agent.Step{
			Index: i, Kind: "tool_call", Summary: "step",
		})
	}
	steps, _ := s.ListSteps(context.Background(), "sess-1")
	if len(steps) != 3 {
		t.Fatalf("steps: %d", len(steps))
	}
	sess, gateID, err := s.GetSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.StepCount != 3 || gateID != "" {
		t.Fatalf("after appends: step_count=%d gate=%q", sess.StepCount, gateID)
	}
	if err := s.SetStatus(context.Background(), "sess-1", "awaiting_gate", "g-1"); err != nil {
		t.Fatal(err)
	}
	sess2, gateID2, _ := s.GetSession(context.Background(), "sess-1")
	if sess2.Status != "awaiting_gate" || gateID2 != "g-1" {
		t.Fatalf("status/gate: %s/%s", sess2.Status, gateID2)
	}
}

func TestStore_ListSessions_TenantScope(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	for _, tid := range []string{"t-1", "t-1", "t-2"} {
		_ = s.CreateSession(context.Background(), intelligence.AgentSession{
			ID:       "sess-" + tid + "-" + randID(),
			TenantID: tid, Role: intelligence.AgentRoleOperator,
			Goal: "x", Status: intelligence.SessionStatusActive, MaxSteps: 12,
		})
	}
	out, _ := s.ListSessions(context.Background(), "t-1", 10)
	if len(out) != 2 {
		t.Fatalf("expected 2 sessions for t-1, got %d", len(out))
	}
}

var counter int

func randID() string {
	counter++
	return string(rune('a' + counter))
}
