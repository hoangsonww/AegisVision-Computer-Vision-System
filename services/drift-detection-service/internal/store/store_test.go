package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/drift-detection-service/internal/store"
)

func TestStore_PolicyReferenceSignalRoundtrip(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	p := intelligence.DriftPolicy{
		ID: "p-1", TenantID: "t-1", ModelID: "yolo-v9",
		Method: intelligence.DivergenceJS,
		WarnThreshold: 0.1, CriticalThreshold: 0.3,
		WindowSeconds: 900, MinSamples: 500,
	}
	if err := s.UpsertPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	ps, _ := s.ListPoliciesByModel(context.Background(), "t-1", "yolo-v9")
	if len(ps) != 1 || ps[0].Method != intelligence.DivergenceJS {
		t.Fatalf("policies: %+v", ps)
	}
	ref := intelligence.DriftReference{
		ID: "r-1", TenantID: "t-1", ModelID: "yolo-v9",
		Distribution: intelligence.Distribution{Classes: map[string]float64{"person": 0.6, "vehicle": 0.4}, Samples: 1000},
		CapturedAt:   time.Now().UTC(),
	}
	if err := s.UpsertReference(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetReference(context.Background(), "t-1", "yolo-v9")
	if err != nil {
		t.Fatal(err)
	}
	if got.Distribution.Classes["person"] != 0.6 {
		t.Fatalf("ref: %+v", got)
	}
	sig := intelligence.DriftSignal{
		ID: "s-1", TenantID: "t-1", ModelID: "yolo-v9",
		Method: intelligence.DivergenceJS, Divergence: 0.42, Severity: "critical",
		Observed: intelligence.Distribution{Classes: map[string]float64{"vehicle": 1.0}, Samples: 700},
	}
	if err := s.InsertSignal(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	sigs, _ := s.ListSignals(context.Background(), "t-1", time.Now().Add(-1*time.Hour), 10)
	if len(sigs) != 1 || sigs[0].Severity != "critical" {
		t.Fatalf("signals: %+v", sigs)
	}
}
