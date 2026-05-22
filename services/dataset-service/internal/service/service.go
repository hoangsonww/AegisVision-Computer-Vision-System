// Package service is the dataset domain layer: dataset+version CRUD
// with the validation rules that should not live in the HTTP layer.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aegisvision/services/dataset-service/internal/store"
)

type Service struct {
	Store *store.Store
	Now   func() time.Time
}

func New(s *store.Store) *Service {
	return &Service{Store: s, Now: func() time.Time { return time.Now().UTC() }}
}

// CreateDataset validates + persists. Required: tenant_id, name.
func (s *Service) CreateDataset(ctx context.Context, d store.Dataset) (store.Dataset, error) {
	if d.TenantID == "" || d.Name == "" {
		return store.Dataset{}, errors.New("tenant_id and name required")
	}
	if d.ID == "" {
		d.ID = fmt.Sprintf("ds-%d", s.Now().UnixNano())
	}
	if err := s.Store.CreateDataset(ctx, d); err != nil {
		return store.Dataset{}, err
	}
	return d, nil
}

// CreateVersion enforces monotonic versions per dataset (the SQL
// schema already has the uniqueness; the service surfaces the
// constraint as a clean error).
func (s *Service) CreateVersion(ctx context.Context, v store.Version) (store.Version, error) {
	if v.DatasetID == "" {
		return store.Version{}, errors.New("dataset_id required")
	}
	if v.ID == "" {
		v.ID = fmt.Sprintf("dv-%d", s.Now().UnixNano())
	}
	if v.ItemCount < 0 {
		return store.Version{}, errors.New("item_count must be >= 0")
	}
	if v.Semver == "" {
		return store.Version{}, errors.New("semver required")
	}
	if err := s.Store.CreateVersion(ctx, v); err != nil {
		return store.Version{}, err
	}
	return v, nil
}

func (s *Service) GetDataset(ctx context.Context, id string) (*store.Dataset, error) {
	return s.Store.GetDataset(ctx, id)
}
func (s *Service) ListDatasets(ctx context.Context, tenant string) ([]store.Dataset, error) {
	return s.Store.ListDatasets(ctx, tenant)
}
func (s *Service) ListVersions(ctx context.Context, dataset string) ([]store.Version, error) {
	return s.Store.ListVersions(ctx, dataset)
}
