package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/canary-controller/internal/loop"
	"github.com/aegisvision/services/canary-controller/internal/store"
)

func setup(t *testing.T) (*store.Store, *bus.InMemoryBus) {
	t.Helper()
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db, false), bus.NewInMemory()
}

func TestReconciler_RollsBackOnRegression(t *testing.T) {
	s, b := setup(t)
	plan := intelligence.CanaryPlan{
		ID: "p-1", TenantID: "t-1",
		ResourceURN: "model:m", BaselineURN: "model:m-old", CandidateURN: "model:m-new",
		Ladder: []intelligence.CanaryStep{{CandidatePercent: 5, HoldSeconds: 60, MinSamples: 1000}},
		MaxProportionDelta: 0.005,
		Status:             intelligence.CanaryStatusRamping,
		LastStepAt:         time.Now().UTC(),
	}
	if err := s.CreatePlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	// Baseline solid, candidate clearly worse.
	w := time.Now().Add(-1 * time.Minute).UTC()
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "baseline", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 9800, Failures: 200,
	})
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "candidate", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 9500, Failures: 500,
	})
	r := &loop.Reconciler{Store: s, Bus: b, Lookback: 5 * time.Minute}
	changes, err := r.Tick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if changes != 1 {
		t.Fatalf("expected 1 change, got %d", changes)
	}
	got, _ := s.GetPlan(context.Background(), "p-1")
	if got.Status != intelligence.CanaryStatusRolledBack {
		t.Fatalf("status: %s", got.Status)
	}
}

func TestReconciler_AdvancesWhenHealthy(t *testing.T) {
	s, b := setup(t)
	plan := intelligence.CanaryPlan{
		ID: "p-1", TenantID: "t-1",
		ResourceURN: "model:m", BaselineURN: "model:m-old", CandidateURN: "model:m-new",
		Ladder: []intelligence.CanaryStep{
			{CandidatePercent: 5, HoldSeconds: 1, MinSamples: 1000},
			{CandidatePercent: 25, HoldSeconds: 1, MinSamples: 1000},
		},
		MaxProportionDelta: 0.01,
		Status:             intelligence.CanaryStatusRamping,
		LastStepAt:         time.Now().Add(-1 * time.Minute).UTC(),
	}
	_ = s.CreatePlan(context.Background(), plan)
	w := time.Now().Add(-1 * time.Minute).UTC()
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "baseline", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 9800, Failures: 200,
	})
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "candidate", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 9790, Failures: 210,
	})
	r := &loop.Reconciler{Store: s, Bus: b}
	if _, err := r.Tick(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPlan(context.Background(), "p-1")
	if got.CurrentStep != 1 {
		t.Fatalf("expected advance to step 1, got step %d (status %s)", got.CurrentStep, got.Status)
	}
}

func TestReconciler_NoChangeOnInsufficientSamples(t *testing.T) {
	s, b := setup(t)
	plan := intelligence.CanaryPlan{
		ID: "p-1", TenantID: "t-1",
		ResourceURN: "model:m", BaselineURN: "model:m-old", CandidateURN: "model:m-new",
		Ladder: []intelligence.CanaryStep{{CandidatePercent: 5, HoldSeconds: 60, MinSamples: 1000}},
		MaxProportionDelta: 0.005,
		Status:             intelligence.CanaryStatusHolding,
		CurrentStep:        0,
		LastStepAt:         time.Now().UTC(),
	}
	_ = s.CreatePlan(context.Background(), plan)
	// Far too few samples.
	w := time.Now().Add(-1 * time.Minute).UTC()
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "baseline", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 50, Failures: 1,
	})
	_ = s.IngestObservation(context.Background(), intelligence.CanaryObservation{
		PlanID: "p-1", Variant: "candidate", WindowStart: w, WindowEnd: w.Add(time.Minute),
		Successes: 30, Failures: 0,
	})
	r := &loop.Reconciler{Store: s, Bus: b}
	changes, _ := r.Tick(context.Background(), time.Now().UTC())
	if changes != 0 {
		t.Fatalf("expected 0 changes, got %d", changes)
	}
}
