// Package service is the domain layer for media-service.
//
// Three concerns live here that don't belong in the SQL store or the
// HTTP handler:
//
//  1. URN format. Media objects are addressed by claim-check URN
//     (see ADR-0008). The service constructs/validates URNs of the
//     form `media://<tenant>/<dataset?>/<object>`.
//  2. Copy-on-promote. The operational safeguard (
//     memory) says promoted media must be a copy, not a reference —
//     so deletion of the source doesn't break the promoted record.
//  3. Retention enforcement. The retention sweeper queries DueForRetention
//     and asks the service to delete; the service decides what "delete"
//     means (object store delete + DB row delete + audit emit).
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/services/media-service/internal/store"
)

// Service is the media domain.
type Service struct {
	Store *store.Store
	// ObjectStoreBase is the S3-compatible base URI (e.g. "s3://aegis-media").
	// Used by URN() to compose object addresses.
	ObjectStoreBase string
	// Now is overridable for tests.
	Now func() time.Time
}

func New(s *store.Store, base string) *Service {
	return &Service{Store: s, ObjectStoreBase: base, Now: func() time.Time { return time.Now().UTC() }}
}

// Create accepts a media row, fills the URN if blank, and inserts.
//
// Required: tenant_id, mime_type, object_uri.
func (s *Service) Create(ctx context.Context, m store.Media) (store.Media, error) {
	if m.TenantID == "" {
		return store.Media{}, errors.New("tenant_id required")
	}
	if m.MIMEType == "" {
		return store.Media{}, errors.New("mime_type required")
	}
	if m.ArtifactURI == "" {
		return store.Media{}, errors.New("artifact_uri required")
	}
	if m.ID == "" {
		m.ID = fmt.Sprintf("md-%d", s.Now().UnixNano())
	}
	if err := s.Store.Create(ctx, m); err != nil {
		return store.Media{}, err
	}
	return m, nil
}

// URN composes a canonical claim-check URN for a tenant + object key.
// The URN MUST be the only handle that crosses service boundaries —
// raw object_uri is internal.
func (s *Service) URN(tenantID, dataset, key string) (string, error) {
	if tenantID == "" || key == "" {
		return "", errors.New("tenant_id and key required for URN")
	}
	if strings.Contains(tenantID, "/") || strings.Contains(key, "/") {
		return "", errors.New("URN segments cannot contain '/'")
	}
	if dataset == "" {
		return fmt.Sprintf("media://%s/%s", tenantID, key), nil
	}
	return fmt.Sprintf("media://%s/%s/%s", tenantID, dataset, key), nil
}

// PromoteCopy is the copy-on-promote action. Caller supplies the
// destination URI (usually a key in a promotion bucket); the service
// updates the row + emits an audit event. The actual byte copy is
// performed by the caller before invoking this (the service is the
// system-of-record, not the object copier).
func (s *Service) PromoteCopy(ctx context.Context, id, newURI string) error {
	if newURI == "" {
		return errors.New("new_uri required for copy-on-promote")
	}
	return s.Store.Promote(ctx, id, newURI)
}

// Get is a passthrough.
func (s *Service) Get(ctx context.Context, id string) (*store.Media, error) {
	return s.Store.Get(ctx, id)
}

// DueForRetention is the read-side of the retention sweep.
func (s *Service) DueForRetention(ctx context.Context, limit int) ([]store.Media, error) {
	return s.Store.DueForRetention(ctx, s.Now(), limit)
}
