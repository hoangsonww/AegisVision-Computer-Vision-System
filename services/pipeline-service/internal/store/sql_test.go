package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"github.com/aegisvision/services/pipeline-service/internal/store"
)

func newSQLStore(t *testing.T) *store.SQLStore {
	t.Helper()
	db, err := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.NewSQLStore(db, false)
}

func TestSQL_CreateAndGet(t *testing.T) {
	s := newSQLStore(t)
	p := &controlv1.Pipeline{
		Id:      "p-1",
		Tenant:  &commonv1.TenantRef{Id: "t-1"},
		Project: &commonv1.ProjectRef{Id: "proj-1", TenantId: "t-1"},
		Name:    "dock-watch",
		Labels:  []string{"safety", "dock"},
		DagJson: `{"nodes":[]}`,
	}
	got, err := s.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT {
		t.Fatalf("status: %v", got.GetStatus())
	}

	fetched, err := s.Get("p-1")
	if err != nil {
		t.Fatal(err)
	}
	if fetched.GetName() != "dock-watch" {
		t.Fatalf("name: %q", fetched.GetName())
	}
	if got, want := fetched.GetLabels(), []string{"safety", "dock"}; len(got) != 2 || got[0] != want[0] {
		t.Fatalf("labels round-trip: %v", got)
	}
}

func TestSQL_UpdateStatusBumpsVersion(t *testing.T) {
	s := newSQLStore(t)
	_, _ = s.Create(&controlv1.Pipeline{
		Id: "p-1", Tenant: &commonv1.TenantRef{Id: "t-1"},
		Project: &commonv1.ProjectRef{Id: "p", TenantId: "t-1"},
		Name:    "x",
	})
	updated, err := s.UpdateStatus("p-1", controlv1.PipelineStatus_PIPELINE_STATUS_RUNNING)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetStatus() != controlv1.PipelineStatus_PIPELINE_STATUS_RUNNING {
		t.Fatalf("status: %v", updated.GetStatus())
	}
	if updated.GetAudit().GetVersion() != 2 {
		t.Fatalf("version: want 2, got %d", updated.GetAudit().GetVersion())
	}
}

func TestSQL_ListPaginatesDeterministically(t *testing.T) {
	s := newSQLStore(t)
	for _, id := range []string{"p-a", "p-b", "p-c", "p-d"} {
		_, _ = s.Create(&controlv1.Pipeline{
			Id: id, Tenant: &commonv1.TenantRef{Id: "t-1"},
			Project: &commonv1.ProjectRef{Id: "proj-1", TenantId: "t-1"},
			Name:    id,
		})
	}
	p1, err := s.List(store.ListFilter{TenantID: "t-1"}, store.PageQuery{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Items) != 2 || p1.NextAfter == "" {
		t.Fatalf("page1: items=%d next=%q", len(p1.Items), p1.NextAfter)
	}
	p2, err := s.List(store.ListFilter{TenantID: "t-1"}, store.PageQuery{PageSize: 2, AfterID: p1.NextAfter})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Items) != 2 || p2.NextAfter != "" {
		t.Fatalf("page2: items=%d next=%q", len(p2.Items), p2.NextAfter)
	}
}

func TestSQL_UpdateStatusNotFound(t *testing.T) {
	s := newSQLStore(t)
	if _, err := s.UpdateStatus("missing", controlv1.PipelineStatus_PIPELINE_STATUS_RUNNING); err != store.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
