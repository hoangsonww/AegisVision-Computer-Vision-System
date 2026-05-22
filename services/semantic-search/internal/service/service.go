// Package service wraps the in-memory index with tenant scoping and
// validation. The index is purely k-NN; this layer enforces that
// tenants cannot search each other's items.
package service

import (
	"context"
	"errors"

	"github.com/aegisvision/services/semantic-search/internal/index"
)

type Service struct {
	Idx *index.Index
}

func New(idx *index.Index) *Service { return &Service{Idx: idx} }

// Upsert validates + writes.
func (s *Service) Upsert(_ context.Context, it index.Item) error {
	if it.ID == "" {
		return errors.New("id required")
	}
	if it.TenantID == "" {
		return errors.New("tenant_id required")
	}
	if len(it.Vector) == 0 {
		return errors.New("vector required")
	}
	return s.Idx.Upsert(it)
}

// Delete is a passthrough.
func (s *Service) Delete(_ context.Context, id string) { s.Idx.Delete(id) }

// Search enforces tenant scoping by forcing the filter's tenant_id.
func (s *Service) Search(_ context.Context, tenant string, query []float32, k int, payload map[string]string) ([]index.SearchResult, error) {
	if tenant == "" {
		return nil, errors.New("tenant_id required")
	}
	if k <= 0 || k > 200 {
		k = 10
	}
	return s.Idx.Search(query, k, index.Filter{TenantID: tenant, Equal: payload})
}

// Size returns index cardinality.
func (s *Service) Size() int { return s.Idx.Size() }
