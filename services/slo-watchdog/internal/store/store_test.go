package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/slo-watchdog/internal/store"
)

func TestStore_TargetsAndSignals(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	tgt := intelligence.SLOTarget{
		ID: "slo-1", TenantID: "t-1", Name: "api_success",
		PromQL: "sum(rate(success[__WINDOW__])) / sum(rate(total[__WINDOW__]))",
		Target: 0.999, RollingWindowSeconds: 28 * 24 * 3600,
	}
	if err := s.UpsertTarget(context.Background(), tgt); err != nil {
		t.Fatal(err)
	}
	active, _ := s.ListActiveTargets(context.Background())
	if len(active) != 1 {
		t.Fatalf("active: %d", len(active))
	}
	sig := intelligence.SLOSignal{
		ID: "ss-1", TenantID: "t-1", TargetID: "slo-1",
		BurnRate1h: 20, BurnRate6h: 8, BurnRate24h: 2,
		Severity: "critical",
	}
	if err := s.InsertSignal(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	sigs, _ := s.ListSignals(context.Background(), "t-1", time.Now().Add(-1*time.Hour), 10)
	if len(sigs) != 1 || sigs[0].Severity != "critical" {
		t.Fatalf("signals: %+v", sigs)
	}
}
