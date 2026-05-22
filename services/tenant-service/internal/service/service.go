// Package service is the tenant-service domain layer. Three concerns
// live here that don't belong in the SQL store:
//
//  1. Tier-default quotas. New tenants get a tier (free / standard /
//     enterprise) and inherit a quota baseline from it.
//  2. Lifecycle event publishing. CreateTenant / SoftDelete / FileDSAR
//     emit bus events so downstream stores (event-service, media-service,
//     audit-service) can act on tenant lifecycle. The publisher is
//     optional — in tests or single-binary dev runs without a bus we
//     skip silently.
//  3. State machine guards. Soft-delete is reversible; hard-delete is
//     gated through policy-gate-service (not here).
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/tenant-service/internal/store"
)

type Service struct {
	Store     *store.Store
	Publisher bus.Publisher // optional
	Now       func() time.Time
}

func New(s *store.Store, pub bus.Publisher) *Service {
	return &Service{Store: s, Publisher: pub, Now: func() time.Time { return time.Now().UTC() }}
}

// CreateTenant validates + persists + emits tenant.lifecycle.v1.
func (s *Service) CreateTenant(ctx context.Context, t store.Tenant) (store.Tenant, error) {
	if t.Name == "" {
		return store.Tenant{}, errors.New("name required")
	}
	if t.Tier == "" {
		t.Tier = "standard"
	}
	if t.Tier != "free" && t.Tier != "standard" && t.Tier != "enterprise" {
		return store.Tenant{}, errors.New("tier must be one of free|standard|enterprise")
	}
	if t.ID == "" {
		t.ID = "t-" + randID()
	}
	if t.CryptoKeyID == "" {
		t.CryptoKeyID = "vault://aegisvision/tenants/" + t.ID
	}
	if t.State == "" {
		t.State = store.StateActive
	}
	if err := s.Store.CreateTenant(ctx, t); err != nil {
		return store.Tenant{}, err
	}
	s.publish(ctx, "tenant.lifecycle.v1", map[string]string{"event": "tenant.created", "tenant_id": t.ID})
	return t, nil
}

// UpsertQuota writes a single quota with sanity checks.
func (s *Service) UpsertQuota(ctx context.Context, q store.Quota) error {
	if q.TenantID == "" || q.Metric == "" {
		return errors.New("tenant_id and metric required")
	}
	if q.SoftLimit < 0 || q.HardLimit < 0 {
		return errors.New("quota limits must be >= 0")
	}
	if q.HardLimit > 0 && q.SoftLimit > q.HardLimit {
		return errors.New("soft_limit cannot exceed hard_limit")
	}
	return s.Store.UpsertQuota(ctx, q)
}

// FileDSAR records a new access/erasure request and emits an event for
// the data-plane cascade.
func (s *Service) FileDSAR(ctx context.Context, d store.DSAR) (store.DSAR, error) {
	if d.TenantID == "" || d.Subject == "" {
		return store.DSAR{}, errors.New("tenant_id and subject required")
	}
	if d.Kind != store.DSARAccess && d.Kind != store.DSARErasure {
		return store.DSAR{}, errors.New("kind must be access|erasure")
	}
	if d.ID == "" {
		d.ID = "dsar-" + randID()
	}
	d.Status = "pending"
	if err := s.Store.CreateDSAR(ctx, d); err != nil {
		return store.DSAR{}, err
	}
	body, _ := json.Marshal(d)
	if s.Publisher != nil {
		_ = s.Publisher.Publish(ctx, bus.Message{
			Subject: "tenant.dsar.v1",
			Data:    body,
			Headers: map[string]string{"tenant": d.TenantID, "kind": d.Kind},
		})
	}
	return d, nil
}

// CompleteDSAR records that an export artifact landed.
func (s *Service) CompleteDSAR(ctx context.Context, id, artifactURI string) error {
	return s.Store.CompleteDSAR(ctx, id, artifactURI)
}

func (s *Service) GetTenant(ctx context.Context, id string) (*store.Tenant, error) {
	return s.Store.GetTenant(ctx, id)
}
func (s *Service) ListTenants(ctx context.Context) ([]store.Tenant, error) {
	return s.Store.ListTenants(ctx)
}
func (s *Service) SoftDelete(ctx context.Context, id string) error {
	if err := s.Store.SoftDelete(ctx, id); err != nil {
		return err
	}
	s.publish(context.Background(), "tenant.lifecycle.v1", map[string]string{"event": "tenant.deleted", "tenant_id": id})
	return nil
}
func (s *Service) ListQuotas(ctx context.Context, tenant string) ([]store.Quota, error) {
	return s.Store.ListQuotas(ctx, tenant)
}
func (s *Service) ListDSAR(ctx context.Context, tenant string) ([]store.DSAR, error) {
	return s.Store.ListDSAR(ctx, tenant)
}

func (s *Service) publish(ctx context.Context, subject string, body any) {
	if s.Publisher == nil {
		return
	}
	b, _ := json.Marshal(body)
	_ = s.Publisher.Publish(ctx, bus.Message{Subject: subject, Data: b})
}

func randID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
