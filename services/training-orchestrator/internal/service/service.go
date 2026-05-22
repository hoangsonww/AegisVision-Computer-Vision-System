// Package service is the training-orchestrator domain layer. The
// orchestrator is responsible for scheduling training jobs against a
// dataset version; promotion of the resulting model is a separate,
// gated path through model-registry (ADR-0014/0017).
//
// State machine (per store):
//   pending → running → (succeeded | failed | cancelled)
// Terminal states are immutable.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aegisvision/services/training-orchestrator/internal/store"
)

type Service struct {
	Store *store.Store
	Now   func() time.Time
}

func New(s *store.Store) *Service {
	return &Service{Store: s, Now: func() time.Time { return time.Now().UTC() }}
}

// Submit validates and queues a job.
func (s *Service) Submit(ctx context.Context, j store.Job) (store.Job, error) {
	if j.TenantID == "" || j.Name == "" || j.BaseModel == "" || j.DatasetID == "" {
		return store.Job{}, errors.New("tenant_id, name, base_model, dataset_id required")
	}
	if j.EpochsTotal <= 0 {
		return store.Job{}, errors.New("epochs_total must be > 0")
	}
	if j.ID == "" {
		j.ID = fmt.Sprintf("tj-%d", s.Now().UnixNano())
	}
	j.State = store.StatePending
	if err := s.Store.Submit(ctx, j); err != nil {
		return store.Job{}, err
	}
	return j, nil
}

// Cancel transitions a job to "cancelled" only if it's still pending
// or running. Idempotent: cancelling a terminal job returns nil.
func (s *Service) Cancel(ctx context.Context, id string) error {
	j, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	switch j.State {
	case store.StateSucceeded, store.StateFailed, store.StateCancelled:
		return nil
	}
	return s.Store.Update(ctx, id, store.StateCancelled, j.EpochsDone, j.MetricsJSON, j.ArtifactURI, "")
}

// MarkRunning transitions pending → running. Refuses if the job is
// already terminal.
func (s *Service) MarkRunning(ctx context.Context, id string) error {
	j, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.State != store.StatePending {
		return fmt.Errorf("cannot run job in state %s", j.State)
	}
	return s.Store.Update(ctx, id, store.StateRunning, j.EpochsDone, j.MetricsJSON, j.ArtifactURI, "")
}

// CompleteSuccess records a successful finish.
func (s *Service) CompleteSuccess(ctx context.Context, id, artifact, metrics string, epochsDone int) error {
	return s.Store.Update(ctx, id, store.StateSucceeded, epochsDone, metrics, artifact, "")
}

// CompleteFailure records a failed finish with the error.
func (s *Service) CompleteFailure(ctx context.Context, id string, errMsg string, epochsDone int) error {
	return s.Store.Update(ctx, id, store.StateFailed, epochsDone, "{}", "", errMsg)
}

// UpdateProgress is the runner's general-purpose state update endpoint.
// State transitions are validated against the allowed set; an empty
// state preserves the current state (progress-only update).
func (s *Service) UpdateProgress(ctx context.Context, id, state string, epochsDone int, metrics, artifact, lastErr string) error {
	if state != "" {
		switch state {
		case store.StatePending, store.StateRunning, store.StateSucceeded, store.StateFailed, store.StateCancelled:
		default:
			return errors.New("invalid state")
		}
	} else {
		j, err := s.Store.Get(ctx, id)
		if err != nil {
			return err
		}
		state = j.State
	}
	return s.Store.Update(ctx, id, state, epochsDone, metrics, artifact, lastErr)
}

// ListByTenant returns all jobs for a tenant (most-recent first).
func (s *Service) ListByTenant(ctx context.Context, tenant string) ([]store.Job, error) {
	return s.Store.List(ctx, tenant)
}

func (s *Service) Get(ctx context.Context, id string) (*store.Job, error) { return s.Store.Get(ctx, id) }
