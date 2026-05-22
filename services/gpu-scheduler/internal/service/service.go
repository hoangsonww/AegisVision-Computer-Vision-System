// Package service wraps the GPU reservation ledger with the bookkeeping
// callers expect across HTTP and bus paths: a request id for every
// placement, structured outcomes, and an audit-friendly view of the
// reservation table.
//
// The ledger itself (internal/ledger) is the source of truth for VRAM
// accounting per ADR-0003 (MIG default, reservation-ledger over
// nvidia-smi). This package exists so the HTTP layer and any future
// gRPC layer share one validation + logging path.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aegisvision/services/gpu-scheduler/internal/ledger"
)

type Service struct {
	Ledger *ledger.Ledger
	Now    func() time.Time
}

func New(l *ledger.Ledger) *Service {
	return &Service{Ledger: l, Now: func() time.Time { return time.Now().UTC() }}
}

// Outcome is the structured response a Place call produces.
type Outcome struct {
	Reservation *ledger.Reservation `json:"reservation"`
	PlacedAt    time.Time           `json:"placed_at"`
}

// Place validates the request, asks the ledger, and shapes the response.
func (s *Service) Place(_ context.Context, req ledger.PlacementRequest) (*Outcome, error) {
	if req.TenantID == "" {
		return nil, errors.New("tenant_id required")
	}
	if req.Model == "" {
		return nil, errors.New("model required")
	}
	if req.Profile.VRAMBytes <= 0 {
		return nil, errors.New("profile.vram_bytes must be > 0")
	}
	r, err := s.Ledger.Place(req)
	if err != nil {
		return nil, fmt.Errorf("place: %w", err)
	}
	return &Outcome{Reservation: r, PlacedAt: s.Now()}, nil
}

// Release is a thin wrapper for symmetry with Place.
func (s *Service) Release(_ context.Context, id string) error {
	if id == "" {
		return errors.New("reservation_id required")
	}
	return s.Ledger.Release(id)
}

// Snapshot exposes the current ledger view (read-only).
func (s *Service) Snapshot(_ context.Context) ledger.Snapshot {
	return s.Ledger.Snapshot()
}
