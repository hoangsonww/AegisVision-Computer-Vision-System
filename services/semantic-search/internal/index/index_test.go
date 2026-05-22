package index_test

import (
	"testing"

	"github.com/aegisvision/services/semantic-search/internal/index"
)

func TestIndex_KNNCosine(t *testing.T) {
	idx := index.New()
	_ = idx.Upsert(index.Item{ID: "a", TenantID: "t", Vector: []float32{1, 0, 0}})
	_ = idx.Upsert(index.Item{ID: "b", TenantID: "t", Vector: []float32{0.9, 0.1, 0}})
	_ = idx.Upsert(index.Item{ID: "c", TenantID: "t", Vector: []float32{0, 1, 0}})
	hits, err := idx.Search([]float32{1, 0, 0}, 2, index.Filter{TenantID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].ID != "a" || hits[1].ID != "b" {
		t.Fatalf("ranking wrong: %+v", hits)
	}
}

func TestIndex_TenantFilter(t *testing.T) {
	idx := index.New()
	_ = idx.Upsert(index.Item{ID: "a", TenantID: "t-1", Vector: []float32{1, 0}})
	_ = idx.Upsert(index.Item{ID: "b", TenantID: "t-2", Vector: []float32{1, 0}})
	hits, _ := idx.Search([]float32{1, 0}, 10, index.Filter{TenantID: "t-1"})
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("tenant filter broken: %+v", hits)
	}
}

func TestIndex_PayloadFilter(t *testing.T) {
	idx := index.New()
	_ = idx.Upsert(index.Item{ID: "a", TenantID: "t", Vector: []float32{1, 0}, Payload: map[string]string{"class": "person"}})
	_ = idx.Upsert(index.Item{ID: "b", TenantID: "t", Vector: []float32{1, 0}, Payload: map[string]string{"class": "vehicle"}})
	hits, _ := idx.Search([]float32{1, 0}, 10, index.Filter{TenantID: "t", Equal: map[string]string{"class": "person"}})
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("payload filter broken: %+v", hits)
	}
}

func TestIndex_DimensionMismatch(t *testing.T) {
	idx := index.New()
	_ = idx.Upsert(index.Item{ID: "a", TenantID: "t", Vector: []float32{1, 0, 0}})
	if err := idx.Upsert(index.Item{ID: "b", TenantID: "t", Vector: []float32{1, 0}}); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
	if _, err := idx.Search([]float32{1, 0}, 5, index.Filter{TenantID: "t"}); err == nil {
		t.Fatal("expected search dimension mismatch error")
	}
}
