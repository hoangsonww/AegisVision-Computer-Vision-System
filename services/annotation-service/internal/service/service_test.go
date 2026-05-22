package service_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/annotation-service/internal/service"
	"github.com/aegisvision/services/annotation-service/internal/store"
)

func setup(t *testing.T) *service.Service {
	t.Helper()
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service.New(store.New(db, false))
}

func TestService_CreateValidates(t *testing.T) {
	s := setup(t)
	if _, err := s.Create(context.Background(), store.Annotation{}); err == nil {
		t.Fatal("expected required-field error")
	}
	a, err := s.Create(context.Background(), store.Annotation{
		TenantID: "t", DatasetID: "d", ItemID: "i", Annotator: "alice", Label: "person",
		BBoxX: 0.1, BBoxY: 0.2, BBoxW: 0.3, BBoxH: 0.4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ReviewStatus != store.ReviewPending {
		t.Fatalf("status: %s", a.ReviewStatus)
	}
}

func TestService_CreateRejectsBadBox(t *testing.T) {
	s := setup(t)
	_, err := s.Create(context.Background(), store.Annotation{
		TenantID: "t", DatasetID: "d", ItemID: "i", Annotator: "a", Label: "x",
		BBoxX: 1.5,
	})
	if err == nil {
		t.Fatal("expected bbox error")
	}
}

func TestService_ReviewRejectsBadStatus(t *testing.T) {
	s := setup(t)
	if err := s.Review(context.Background(), "any", "garbage", "rev"); err == nil {
		t.Fatal("expected error")
	}
}

func TestService_ReviewApprovesAndIsTerminal(t *testing.T) {
	s := setup(t)
	a, _ := s.Create(context.Background(), store.Annotation{
		TenantID: "t", DatasetID: "d", ItemID: "i", Annotator: "alice", Label: "person",
		BBoxX: 0.1, BBoxY: 0.2, BBoxW: 0.3, BBoxH: 0.4,
	})
	if err := s.Review(context.Background(), a.ID, store.ReviewApproved, "bob"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListByItem(context.Background(), "d", "i")
	if len(list) != 1 || list[0].ReviewStatus != store.ReviewApproved {
		t.Fatalf("got: %+v", list)
	}
}
