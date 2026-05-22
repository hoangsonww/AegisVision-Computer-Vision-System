package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/metering-service/internal/store"
)

func TestMeter_IncrementAndRead(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)

	for i := 0; i < 5; i++ {
		if err := s.Increment(context.Background(), "t-1", "inferences", 7); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().UTC().Format("2006-01-02")
	n, err := s.Day(context.Background(), "t-1", "inferences", day)
	if err != nil {
		t.Fatal(err)
	}
	if n != 35 {
		t.Fatalf("want 35, got %d", n)
	}
}

func TestMeter_TenantIsolation(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.Increment(context.Background(), "t-a", "x", 10)
	_ = s.Increment(context.Background(), "t-b", "x", 99)
	day := time.Now().UTC().Format("2006-01-02")
	if n, _ := s.Day(context.Background(), "t-a", "x", day); n != 10 {
		t.Fatalf("t-a: %d", n)
	}
	if n, _ := s.Day(context.Background(), "t-b", "x", day); n != 99 {
		t.Fatalf("t-b: %d", n)
	}
}
