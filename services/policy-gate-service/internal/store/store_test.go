package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/policy-gate-service/internal/store"
)

func TestGate_OpenDecideIsIdempotent(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	g := intelligence.Gate{
		Request: intelligence.GateRequest{
			ID: "g-1", SessionID: "s-1", TenantID: "t-1",
			ToolName: "promote_model", ActionSummary: "promote v9 to primary",
			ArgumentsJSON: `{"model_id":"yolo-v9"}`, BlastRadius: "high",
			RequestedAt: time.Now().UTC(),
		},
	}
	if _, err := s.Open(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), "g-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != intelligence.GateDecisionPending {
		t.Fatalf("decision: %s", got.Decision)
	}
	pending, _ := s.ListPending(context.Background(), "t-1", 10)
	if len(pending) != 1 {
		t.Fatalf("pending: %d", len(pending))
	}
	_, changed, err := s.Decide(context.Background(), "g-1", intelligence.GateDecisionApproved, "alice", "approved per RFC")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	// Second decision is a no-op.
	_, changed2, _ := s.Decide(context.Background(), "g-1", intelligence.GateDecisionRejected, "bob", "")
	if changed2 {
		t.Fatal("second decision should not flip")
	}
	final, _ := s.Get(context.Background(), "g-1")
	if final.Decision != intelligence.GateDecisionApproved {
		t.Fatalf("final decision: %s", final.Decision)
	}
}

func TestGate_RejectsInvalidDecision(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	if _, _, err := s.Decide(context.Background(), "x", "garbage", "alice", ""); err == nil {
		t.Fatal("expected rejection of bad decision")
	}
}
