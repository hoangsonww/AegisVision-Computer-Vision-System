package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/active-learning-service/internal/store"
)

func TestPolicyAndSample_Roundtrip(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	p := intelligence.LearningPolicy{
		ID: "lp-1", TenantID: "t-1", ModelID: "yolo-v9",
		Strategy: intelligence.UncertaintyMargin,
		SampleRate: 0.05, MinUncertainty: 0.2, MaxUncertainty: 0.8,
		DiversityClasses: []string{"person", "vehicle"}, DailyBudget: 100, SnapshotTTLDays: 30,
	}
	if err := s.UpsertPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	out, err := s.GetPolicy(context.Background(), "lp-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Strategy != intelligence.UncertaintyMargin || len(out.DiversityClasses) != 2 {
		t.Fatalf("got: %+v", out)
	}

	// Insert + transition a sample.
	sm := intelligence.Sample{
		ID: "s-1", TenantID: "t-1", ModelID: "yolo-v9", PolicyID: "lp-1",
		SnapshotURN: "media://snap/1", PredictedClass: "person", PredictedConfidence: 0.55,
		Uncertainty: 0.45, Status: intelligence.SampleStatusQueued,
	}
	if err := s.InsertSample(context.Background(), sm); err != nil {
		t.Fatal(err)
	}
	if err := s.AssignSample(context.Background(), "s-1", "ann-1"); err != nil {
		t.Fatal(err)
	}
	// Verify assigned state shows up.
	listA, err := s.ListSamples(context.Background(), "t-1", "assigned", 10)
	if err != nil {
		t.Fatalf("list err: %v", err)
	}
	if len(listA) != 1 {
		// Cross-check the row exists at all.
		listAny, errAny := s.ListSamples(context.Background(), "t-1", "", 10)
		t.Fatalf("after assign: status-filter=%+v unfiltered=%+v err=%v", listA, listAny, errAny)
	}
	if err := s.MarkLabeled(context.Background(), "s-1", "person"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSamples(context.Background(), "t-1", "labeled", 10)
	if len(list) != 1 || list[0].FinalClass != "person" {
		t.Fatalf("got: %+v", list)
	}
}
