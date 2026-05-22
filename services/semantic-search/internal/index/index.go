// In-memory vector index for semantic-search. The production deployment
// pushes vectors to Qdrant (see Qdrant interface in this file); this
// in-memory implementation is the algorithmic reference + the unit-test
// target. KNN uses brute-force cosine distance — fine for tests; in
// production Qdrant handles HNSW indexing.
package index

import (
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
)

// Item is one indexed vector with metadata.
type Item struct {
	ID       string
	TenantID string
	Vector   []float32
	Payload  map[string]string // arbitrary key/value for filters
}

// SearchResult is one nearest-neighbor hit.
type SearchResult struct {
	ID       string
	Score    float32 // 1.0 = identical (cosine similarity)
	Payload  map[string]string
}

// Filter narrows the search universe by tenant + arbitrary payload k=v match.
type Filter struct {
	TenantID string
	Equal    map[string]string
}

// Index is goroutine-safe.
type Index struct {
	mu    sync.RWMutex
	items map[string]*Item
	dim   int
}

func New() *Index { return &Index{items: map[string]*Item{}} }

// Upsert inserts or replaces an item. The first Upsert pins the index's
// dimensionality; subsequent Upserts with the wrong dimensionality fail.
func (idx *Index) Upsert(it Item) error {
	if it.ID == "" || it.TenantID == "" {
		return errors.New("index: ID and TenantID are required")
	}
	if len(it.Vector) == 0 {
		return errors.New("index: empty vector")
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.dim == 0 {
		idx.dim = len(it.Vector)
	}
	if len(it.Vector) != idx.dim {
		return errors.New("index: dimension mismatch")
	}
	idx.items[it.ID] = &it
	return nil
}

// Delete removes an item; no-op if absent.
func (idx *Index) Delete(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.items, id)
}

// Size returns the current item count.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.items)
}

// Search returns the top-k nearest items by cosine similarity, filtered.
func (idx *Index) Search(query []float32, k int, f Filter) ([]SearchResult, error) {
	if k <= 0 {
		k = 10
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.dim != 0 && len(query) != idx.dim {
		return nil, errors.New("index: query dimension mismatch")
	}

	type scored struct {
		it    *Item
		score float32
	}
	var hits []scored
	for _, it := range idx.items {
		if f.TenantID != "" && it.TenantID != f.TenantID {
			continue
		}
		if !payloadMatches(it.Payload, f.Equal) {
			continue
		}
		s := cosine(query, it.Vector)
		hits = append(hits, scored{it: it, score: s})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > k {
		hits = hits[:k]
	}
	out := make([]SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchResult{ID: h.it.ID, Score: h.score, Payload: copyPayload(h.it.Payload)})
	}
	return out, nil
}

func payloadMatches(p, want map[string]string) bool {
	for k, v := range want {
		if p[k] != v {
			return false
		}
	}
	return true
}

func copyPayload(p map[string]string) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// cosine returns cosine similarity in [-1, 1]; 1.0 = identical direction.
func cosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// _ pin imports needed across the package even if future edits drop them.
var _ = strings.TrimSpace
