// Package service is the domain layer for annotation-service. It wraps
// the store with validation + business rules that should not leak into
// the HTTP layer (and that we want to unit-test without spinning a
// server).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aegisvision/services/annotation-service/internal/store"
)

// Service is the annotation domain.
type Service struct {
	Store *store.Store
	// Now is overridable for deterministic tests.
	Now func() time.Time
}

func New(s *store.Store) *Service {
	return &Service{Store: s, Now: func() time.Time { return time.Now().UTC() }}
}

// Create validates and writes a new annotation. Validation:
//   - tenant_id, dataset_id, item_id, annotator, label required
//   - bbox dims must be in [0, 1] (normalized coordinates)
//   - review_status is forced to pending; only Review() can promote it
func (s *Service) Create(ctx context.Context, a store.Annotation) (store.Annotation, error) {
	if a.TenantID == "" || a.DatasetID == "" || a.ItemID == "" || a.Annotator == "" || a.Label == "" {
		return store.Annotation{}, errors.New("tenant_id, dataset_id, item_id, annotator, label required")
	}
	if !inUnit(a.BBoxX) || !inUnit(a.BBoxY) || !inUnit(a.BBoxW) || !inUnit(a.BBoxH) {
		return store.Annotation{}, errors.New("bbox dims must be in [0,1]")
	}
	if a.ID == "" {
		a.ID = fmt.Sprintf("an-%d", s.Now().UnixNano())
	}
	a.ReviewStatus = store.ReviewPending
	a.Reviewer = ""
	if err := s.Store.Create(ctx, a); err != nil {
		return store.Annotation{}, err
	}
	return a, nil
}

// Review promotes an annotation from pending → approved/rejected.
//
//   - Same reviewer cannot approve their own annotation (4-eyes
//     principle when annotator == reviewer).
//   - Once terminal, status cannot change.
func (s *Service) Review(ctx context.Context, id, status, reviewer string) error {
	if reviewer == "" {
		return errors.New("reviewer required")
	}
	if status != store.ReviewApproved && status != store.ReviewRejected {
		return errors.New("invalid status (approved|rejected)")
	}
	return s.Store.Review(ctx, id, status, reviewer)
}

// ListByItem is a passthrough — included so callers don't need to
// reach into the store directly.
func (s *Service) ListByItem(ctx context.Context, dataset, item string) ([]store.Annotation, error) {
	return s.Store.ListByItem(ctx, dataset, item)
}

func inUnit(f float32) bool { return f >= 0 && f <= 1 }
