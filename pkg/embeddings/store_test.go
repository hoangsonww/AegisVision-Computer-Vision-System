package embeddings_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/embeddings"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/pkg/store/sqldb"
)

func TestStore_UpsertAndSearch_SQLite(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	s := embeddings.New(db, "sqlite")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := llm.NewFakeClient()
	// Ingest three docs with distinct content.
	docs := []intelligence.Doc{
		{ID: "d1", Kind: intelligence.DocKindADR, Title: "ADR 0014", Body: "Bounded autonomy: agents must request a gate before consequential actions", Tags: []string{"adr"}},
		{ID: "d2", Kind: intelligence.DocKindRunbook, Title: "GPU saturation", Body: "If gpu_seconds rises above quota, page oncall and check MIG profiles", Tags: []string{"gpu", "runbook"}},
		{ID: "d3", Kind: intelligence.DocKindRunbook, Title: "Stream camera ingest", Body: "Camera URL refused by stream-manager; check RTSP credentials in Vault", Tags: []string{"stream"}},
	}
	for _, d := range docs {
		v, err := embeddings.One(context.Background(), c, "bge", "", d.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Upsert(context.Background(), d, v); err != nil {
			t.Fatal(err)
		}
	}
	// Query that should hit the bounded-autonomy doc.
	qv, _ := embeddings.One(context.Background(), c, "bge", "", "Bounded autonomy: agents must request a gate before consequential actions")
	hits, err := s.Search(context.Background(), "", qv, 3, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Doc.ID != "d1" {
		t.Fatalf("expected d1 first, got %s (score=%f)", hits[0].Doc.ID, hits[0].Score)
	}
	// Same string → cosine should be very close to 1.
	if hits[0].Score < 0.99 {
		t.Fatalf("self-similarity should be ~1, got %f", hits[0].Score)
	}
}

func TestChunk_RespectsBudget(t *testing.T) {
	body := "Para one is here.\n\nPara two has multiple sentences. Each is short. Together they should still chunk cleanly.\n\nPara three is a single long paragraph that exceeds the artificially small budget we are passing in so the chunker has to break it into multiple windows; this validates the sentence-split fallback path inside the chunker."
	chunks := embeddings.Chunk(body, 80, 10)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		// Allow some slack — chunk doesn't hard-trim; it just stops adding.
		if len([]rune(c)) > 240 {
			t.Fatalf("chunk too big: %d runes\n%s", len([]rune(c)), c)
		}
	}
}

func TestStore_TagsFilter_SQLite(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	s := embeddings.New(db, "sqlite")
	_ = s.Migrate(context.Background())
	c := llm.NewFakeClient()
	d1 := intelligence.Doc{ID: "x", Kind: intelligence.DocKindADR, Title: "x", Body: "alpha beta gamma", Tags: []string{"phase:4"}}
	d2 := intelligence.Doc{ID: "y", Kind: intelligence.DocKindADR, Title: "y", Body: "alpha beta gamma", Tags: []string{"phase:1"}}
	v1, _ := embeddings.One(context.Background(), c, "bge", "", d1.Body)
	v2, _ := embeddings.One(context.Background(), c, "bge", "", d2.Body)
	_ = s.Upsert(context.Background(), d1, v1)
	_ = s.Upsert(context.Background(), d2, v2)
	qv, _ := embeddings.One(context.Background(), c, "bge", "", "alpha beta gamma")
	hits, _ := s.Search(context.Background(), "", qv, 5, nil, []string{"phase:4"})
	if len(hits) != 1 || hits[0].Doc.ID != "x" {
		t.Fatalf("tag filter failed: %+v", hits)
	}
}
