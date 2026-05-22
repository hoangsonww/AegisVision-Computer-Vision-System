package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/dataset-service/internal/store"
)

func TestDataset_CreateAndList(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.CreateDataset(context.Background(), store.Dataset{ID: "ds-1", TenantID: "t-1", Name: "dock-clips", Kind: "clips"})
	_ = s.CreateDataset(context.Background(), store.Dataset{ID: "ds-2", TenantID: "t-1", Name: "dock-images", Kind: "images"})
	_ = s.CreateDataset(context.Background(), store.Dataset{ID: "ds-3", TenantID: "t-other", Name: "x", Kind: "images"})

	ls, err := s.ListDatasets(context.Background(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 2 {
		t.Fatalf("want 2, got %d", len(ls))
	}
}

func TestDataset_VersionsImmutable(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.CreateDataset(context.Background(), store.Dataset{ID: "ds-1", TenantID: "t-1", Name: "n", Kind: "images"})
	if err := s.CreateVersion(context.Background(), store.Version{
		ID: "v-1", DatasetID: "ds-1", Semver: "1.0.0", ItemCount: 1000, ArtifactURI: "s3://bucket/m.json", Immutable: true,
	}); err != nil {
		t.Fatal(err)
	}
	vs, _ := s.ListVersions(context.Background(), "ds-1")
	if len(vs) != 1 || !vs[0].Immutable {
		t.Fatalf("versions: %+v", vs)
	}
}
