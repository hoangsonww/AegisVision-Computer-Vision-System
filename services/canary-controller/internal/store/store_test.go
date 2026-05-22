package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/canary"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/canary-controller/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db, false)
}

func TestPlan_CRUDAndApplyDecision(t *testing.T) {
	s := openStore(t)
	plan := intelligence.CanaryPlan{
		ID: "p-1", TenantID: "t-1",
		ResourceURN: "model:yolo", BaselineURN: "model:yolo-v8", CandidateURN: "model:yolo-v9",
		Ladder: []intelligence.CanaryStep{
			{CandidatePercent: 5, HoldSeconds: 60, MinSamples: 1000},
			{CandidatePercent: 25, HoldSeconds: 60, MinSamples: 1000},
		},
		MaxProportionDelta: 0.005,
		Status:             intelligence.CanaryStatusRamping,
		LastStepAt:         time.Now().UTC(),
	}
	if err := s.CreatePlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPlan(context.Background(), "p-1")
	if err != nil || got.Status != intelligence.CanaryStatusRamping {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := s.ApplyDecision(context.Background(), "p-1", canary.Decision{
		NewStatus: intelligence.CanaryStatusRolledBack, NewStep: -1, FailureReason: "proportion_regression",
	}); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetPlan(context.Background(), "p-1")
	if got2.Status != intelligence.CanaryStatusRolledBack || got2.FailureReason != "proportion_regression" {
		t.Fatalf("apply: %+v", got2)
	}
}

func TestObservation_AggregateSince(t *testing.T) {
	s := openStore(t)
	start := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Minute)
	for i := 0; i < 5; i++ {
		if err := s.IngestObservation(context.Background(), intelligence.CanaryObservation{
			PlanID: "p-1", Variant: "candidate",
			WindowStart: start.Add(time.Duration(i) * time.Minute),
			WindowEnd:   start.Add(time.Duration(i+1) * time.Minute),
			Successes:   1000, Failures: 5, LatencyP95Ms: int64(100 + i*10),
		}); err != nil {
			t.Fatal(err)
		}
	}
	agg, err := s.AggregateSince(context.Background(), "p-1", "candidate", start)
	if err != nil {
		t.Fatal(err)
	}
	if agg.Successes != 5000 || agg.Failures != 25 || agg.P95LatencyMs != 140 {
		t.Fatalf("agg: %+v", agg)
	}
}

func TestObservation_IdempotentMerge(t *testing.T) {
	s := openStore(t)
	w := time.Now().UTC().Truncate(time.Minute)
	for i := 0; i < 3; i++ {
		if err := s.IngestObservation(context.Background(), intelligence.CanaryObservation{
			PlanID: "p-1", Variant: "baseline",
			WindowStart: w, WindowEnd: w.Add(time.Minute),
			Successes: 100, Failures: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	agg, _ := s.AggregateSince(context.Background(), "p-1", "baseline", w)
	if agg.Successes != 300 || agg.Failures != 3 {
		t.Fatalf("merge: %+v", agg)
	}
}
