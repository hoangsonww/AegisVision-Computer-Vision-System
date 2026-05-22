package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/cost-accounting/internal/store"
)

func TestCost_UpsertMerges(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	for i := 0; i < 3; i++ {
		_ = s.Upsert(context.Background(), store.Line{
			TenantID: "t-1", Day: "2026-05-21", LineItem: "gpu_seconds",
			Quantity: 100, UnitCostUSD: 0.0005, TotalUSD: 0.05,
		})
	}
	total, err := s.TenantTotal(context.Background(), "t-1", "2026-05-21", "2026-05-21")
	if err != nil {
		t.Fatal(err)
	}
	if total < 0.14 || total > 0.16 {
		t.Fatalf("total: %f", total)
	}
}

func TestCost_BreakdownByLineItem(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.Upsert(context.Background(), store.Line{TenantID: "t-1", Day: "2026-05-21", LineItem: "gpu_seconds", TotalUSD: 10})
	_ = s.Upsert(context.Background(), store.Line{TenantID: "t-1", Day: "2026-05-21", LineItem: "cpu", TotalUSD: 2})
	br, _ := s.Breakdown(context.Background(), "t-1", "2026-05-21", "2026-05-21")
	if br["gpu_seconds"] != 10 || br["cpu"] != 2 {
		t.Fatalf("breakdown: %+v", br)
	}
}
