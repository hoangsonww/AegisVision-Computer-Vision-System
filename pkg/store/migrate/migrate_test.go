package migrate_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/aegisvision/pkg/store/migrate"
	"github.com/aegisvision/pkg/store/sqldb"
)

func TestApply_AppliesAllAndIsIdempotent(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_init.sql":     {Data: []byte(`CREATE TABLE widgets(id TEXT PRIMARY KEY, name TEXT)`)},
		"0002_add_color.sql": {Data: []byte(`ALTER TABLE widgets ADD COLUMN color TEXT`)},
	}
	db, err := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms, err := migrate.LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(ms); got != 2 {
		t.Fatalf("want 2 migrations, got %d", got)
	}

	for i := 0; i < 2; i++ {
		if err := migrate.Apply(context.Background(), db, ms); err != nil {
			t.Fatalf("apply #%d: %v", i, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO widgets(id,name,color) VALUES('w1','hub','red')`); err != nil {
		t.Fatalf("schema not applied: %v", err)
	}
}

func TestApply_RejectsDuplicateVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte(`CREATE TABLE a(x INT)`)},
		"0001_b.sql": {Data: []byte(`CREATE TABLE b(x INT)`)},
	}
	if _, err := migrate.LoadFS(fsys); err == nil {
		t.Fatal("expected duplicate-version error")
	}
}
