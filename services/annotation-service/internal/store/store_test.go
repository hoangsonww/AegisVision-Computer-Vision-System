package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/annotation-service/internal/store"
)

func TestAnnotation_CreateAndReview(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)

	a := store.Annotation{
		ID: "ann-1", TenantID: "t-1", DatasetID: "ds-1", ItemID: "img-1",
		Annotator: "alice", Label: "person",
		BBoxX: 0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.4,
	}
	if err := s.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := s.Review(context.Background(), "ann-1", store.ReviewApproved, "bob"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListByItem(context.Background(), "ds-1", "img-1")
	if len(got) != 1 || got[0].ReviewStatus != store.ReviewApproved || got[0].Reviewer != "bob" {
		t.Fatalf("review didn't stick: %+v", got)
	}
}

func TestAnnotation_RejectsInvalidReviewStatus(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.Create(context.Background(), store.Annotation{ID: "x", DatasetID: "ds", ItemID: "i", Label: "l"})
	if err := s.Review(context.Background(), "x", "bogus", "r"); err == nil {
		t.Fatal("want error for invalid status")
	}
}
