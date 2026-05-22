package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/pkg/store/sqldb"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/aegisvision/services/event-service/internal/store"
)

func TestPersistent_AppendAndList(t *testing.T) {
	db, err := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.MigratePersistent(context.Background(), db, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ps := store.NewPersistent(db, false, false)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		ev := &dataplanev1.Event{
			Id: id(i), TenantId: "t-1", StreamId: "s-1",
			Kind:        dataplanev1.EventKind_EVENT_KIND_RULE_FIRED,
			Severity:    commonv1.Severity_SEVERITY_HIGH,
			FrameSeq:    int64(i),
			CaptureTime: timestamppb.New(now.Add(time.Duration(i) * time.Second)),
			EmittedAt:   timestamppb.New(now.Add(time.Duration(i) * time.Second)),
			Summary:     "dwell",
			Score:       0.92,
		}
		if err := ps.Append(context.Background(), ev); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	got, err := ps.List(context.Background(), "t-1", "s-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5, got %d", len(got))
	}
	// most recent first
	if got[0].GetFrameSeq() != 4 {
		t.Fatalf("ordering: first frame_seq=%d", got[0].GetFrameSeq())
	}
}

func TestPersistent_TenantIsolation(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigratePersistent(context.Background(), db, false)
	ps := store.NewPersistent(db, false, false)

	for _, tenant := range []string{"t-a", "t-b"} {
		_ = ps.Append(context.Background(), &dataplanev1.Event{
			Id: "e-" + tenant, TenantId: tenant, StreamId: "s",
			EmittedAt: timestamppb.Now(), CaptureTime: timestamppb.Now(),
		})
	}
	got, _ := ps.List(context.Background(), "t-a", "", 10)
	if len(got) != 1 || got[0].GetTenantId() != "t-a" {
		t.Fatalf("cross-tenant leak: %v", got)
	}
}

func id(i int) string { return "evt-" + string(rune('0'+i)) }
