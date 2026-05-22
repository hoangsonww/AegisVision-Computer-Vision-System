package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/media-service/internal/store"
)

func TestMedia_PromoteSurvivesRetention(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)

	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	_ = s.Create(context.Background(), store.Media{
		ID: "m-1", TenantID: "t-1", Kind: "clip", ArtifactURI: "s3://b/k.mp4", MIMEType: "video/mp4",
		RetentionAt: sql.NullTime{Time: yesterday, Valid: true},
	})

	// Before promote, due-for-retention picks it up.
	due, _ := s.DueForRetention(context.Background(), time.Now().UTC(), 10)
	if len(due) != 1 {
		t.Fatalf("want 1 due, got %d", len(due))
	}

	// Promote it (copy-on-promote into training dataset URI).
	if err := s.Promote(context.Background(), "m-1", "s3://train/m-1.mp4"); err != nil {
		t.Fatal(err)
	}

	// After promote, retention sweeper should NOT pick it up — operational safeguard.
	due, _ = s.DueForRetention(context.Background(), time.Now().UTC(), 10)
	if len(due) != 0 {
		t.Fatalf("promoted media must survive retention: got %d due", len(due))
	}
}
