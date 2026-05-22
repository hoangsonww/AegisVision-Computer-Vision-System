package ingest_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aegisvision/pkg/embeddings"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/knowledge-service/internal/ingest"
)

func TestIngestDir_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	adr := filepath.Join(dir, "adr")
	rb := filepath.Join(dir, "runbooks")
	_ = os.Mkdir(adr, 0o755)
	_ = os.Mkdir(rb, 0o755)
	if err := os.WriteFile(filepath.Join(adr, "0099-test.md"), []byte("# Test ADR\n\nBody of the test ADR\n\nMore body explains the trade-off.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rb, "test.md"), []byte("# GPU Saturation\n\nIf gpu_seconds rises above quota, page oncall.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	st := embeddings.New(db, "sqlite")
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := llm.NewFakeClient()
	ing := ingest.New(st, c, "bge")
	n, err := ing.IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", n)
	}
	// Search should turn up the runbook on a relevant query.
	qv, _ := embeddings.One(context.Background(), c, "bge", "", "If gpu_seconds rises above quota, page oncall.")
	hits, _ := st.Search(context.Background(), "", qv, 3, []intelligence.DocKind{intelligence.DocKindRunbook}, nil)
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].Doc.Kind != intelligence.DocKindRunbook {
		t.Fatalf("expected runbook, got %s", hits[0].Doc.Kind)
	}
}
