// Tenant + quota + DSAR store. The crypto_key_id column is the handle to a
// Vault transit key; deletion of the key crypto-shreds every encrypted byte
// belonging to the tenant (ADR-0014 + the architecture doc's privacy chapter).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
)

//go:embed *.sql
var migrationsFS embed.FS

func MigrateUp(ctx context.Context, db *sql.DB) error {
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

var ErrNotFound = errors.New("tenant: not found")

const (
	TierFree       = "free"
	TierStandard   = "standard"
	TierEnterprise = "enterprise"
)
const (
	StateActive    = "active"
	StateSuspended = "suspended"
	StateDeleted   = "deleted"
)
const (
	DSARAccess   = "access"
	DSARErasure  = "erasure"
)

type Tenant struct {
	ID, Name, Tier, CryptoKeyID, State string
	CreatedAt, UpdatedAt               time.Time
	DeletedAt                          sql.NullTime
}

type Quota struct {
	TenantID, Metric    string
	SoftLimit, HardLimit int64
	UpdatedAt           time.Time
}

type DSAR struct {
	ID, TenantID, Subject, Kind, Status, RequestedBy, ArtifactURI string
	RequestedAt                                                   time.Time
	CompletedAt                                                   sql.NullTime
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

// ----------- Tenant CRUD -----------

func (s *Store) CreateTenant(ctx context.Context, t Tenant) error {
	now := time.Now().UTC()
	if t.Tier == "" {
		t.Tier = TierStandard
	}
	if t.State == "" {
		t.State = StateActive
	}
	q := s.ph(`INSERT INTO tenants(id, name, tier, crypto_key_id, state, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, t.ID, t.Name, t.Tier, t.CryptoKeyID, t.State, now, now)
	if err != nil {
		return err
	}
	return s.applyTierDefaults(ctx, t.ID, t.Tier, now)
}

func (s *Store) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	q := s.ph(`SELECT id, name, tier, crypto_key_id, state, created_at, updated_at, deleted_at
        FROM tenants WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Tier, &t.CryptoKeyID, &t.State, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	q := s.ph(`SELECT id, name, tier, crypto_key_id, state, created_at, updated_at, deleted_at FROM tenants ORDER BY name`)
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Tier, &t.CryptoKeyID, &t.State, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SoftDelete marks the tenant as deleted and starts the erasure clock. The
// actual crypto-shred is the responsibility of an Erasure DSAR (separate
// workflow) — soft-delete merely freezes access.
func (s *Store) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	q := s.ph(`UPDATE tenants SET state = ?, deleted_at = ?, updated_at = ? WHERE id = ?`)
	res, err := s.DB.ExecContext(ctx, q, StateDeleted, now, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------- Quotas -----------

func (s *Store) UpsertQuota(ctx context.Context, q Quota) error {
	now := time.Now().UTC()
	if s.IsPostgres {
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO quotas(tenant_id, metric, soft_limit, hard_limit, updated_at) VALUES ($1,$2,$3,$4,$5)
             ON CONFLICT (tenant_id, metric) DO UPDATE SET soft_limit=EXCLUDED.soft_limit, hard_limit=EXCLUDED.hard_limit, updated_at=EXCLUDED.updated_at`,
			q.TenantID, q.Metric, q.SoftLimit, q.HardLimit, now)
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO quotas(tenant_id, metric, soft_limit, hard_limit, updated_at) VALUES (?, ?, ?, ?, ?)
         ON CONFLICT(tenant_id, metric) DO UPDATE SET soft_limit=excluded.soft_limit, hard_limit=excluded.hard_limit, updated_at=excluded.updated_at`,
		q.TenantID, q.Metric, q.SoftLimit, q.HardLimit, now)
	return err
}

func (s *Store) ListQuotas(ctx context.Context, tenant string) ([]Quota, error) {
	q := s.ph(`SELECT tenant_id, metric, soft_limit, hard_limit, updated_at FROM quotas WHERE tenant_id = ?`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Quota
	for rows.Next() {
		var q Quota
		if err := rows.Scan(&q.TenantID, &q.Metric, &q.SoftLimit, &q.HardLimit, &q.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *Store) GetQuota(ctx context.Context, tenant, metric string) (*Quota, error) {
	q := s.ph(`SELECT tenant_id, metric, soft_limit, hard_limit, updated_at FROM quotas WHERE tenant_id = ? AND metric = ?`)
	var qu Quota
	err := s.DB.QueryRowContext(ctx, q, tenant, metric).Scan(&qu.TenantID, &qu.Metric, &qu.SoftLimit, &qu.HardLimit, &qu.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &qu, err
}

// applyTierDefaults seeds the canonical quotas for a tenant tier. Quotas are
// best-effort soft + hard pairs; the platform's rate limiter consults the
// hard limit for 429 responses.
func (s *Store) applyTierDefaults(ctx context.Context, tenant, tier string, now time.Time) error {
	defaults := tierDefaults(tier)
	for metric, limits := range defaults {
		if err := s.UpsertQuota(ctx, Quota{
			TenantID: tenant, Metric: metric,
			SoftLimit: limits[0], HardLimit: limits[1], UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func tierDefaults(tier string) map[string][2]int64 {
	switch tier {
	case TierFree:
		return map[string][2]int64{
			"pipelines":              {2, 5},
			"streams":                {3, 5},
			"events_per_minute":      {600, 1200},
			"inferences_per_second":  {5, 10},
			"gpus":                   {0, 0},
			"storage_bytes":          {1 << 30, 5 << 30},  // 1GB soft, 5GB hard
		}
	case TierEnterprise:
		return map[string][2]int64{
			"pipelines":              {500, 1000},
			"streams":                {2000, 5000},
			"events_per_minute":      {1_000_000, 5_000_000},
			"inferences_per_second":  {10_000, 50_000},
			"gpus":                   {50, 200},
			"storage_bytes":          {1 << 50, 10 << 50}, // 1PB soft, 10PB hard
		}
	default: // standard
		return map[string][2]int64{
			"pipelines":              {50, 200},
			"streams":                {100, 500},
			"events_per_minute":      {60_000, 300_000},
			"inferences_per_second":  {500, 2000},
			"gpus":                   {4, 16},
			"storage_bytes":          {100 << 30, 1 << 40}, // 100GB soft, 1TB hard
		}
	}
}

// ----------- DSAR (GDPR right-of-access + right-to-erasure) -----------

func (s *Store) CreateDSAR(ctx context.Context, d DSAR) error {
	now := time.Now().UTC()
	if d.Status == "" {
		d.Status = "pending"
	}
	q := s.ph(`INSERT INTO dsar_requests(id, tenant_id, subject, kind, status, requested_by, requested_at, artifact_uri)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, d.ID, d.TenantID, d.Subject, d.Kind, d.Status, d.RequestedBy, now, d.ArtifactURI)
	return err
}

func (s *Store) CompleteDSAR(ctx context.Context, id, artifactURI string) error {
	now := time.Now().UTC()
	q := s.ph(`UPDATE dsar_requests SET status = 'completed', completed_at = ?, artifact_uri = ? WHERE id = ?`)
	res, err := s.DB.ExecContext(ctx, q, now, artifactURI, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListDSAR(ctx context.Context, tenant string) ([]DSAR, error) {
	q := s.ph(`SELECT id, tenant_id, subject, kind, status, requested_by, requested_at, completed_at, artifact_uri
        FROM dsar_requests WHERE tenant_id = ? ORDER BY requested_at DESC`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DSAR
	for rows.Next() {
		var d DSAR
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Subject, &d.Kind, &d.Status, &d.RequestedBy, &d.RequestedAt, &d.CompletedAt, &d.ArtifactURI); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ph(q string) string {
	if !s.IsPostgres {
		return q
	}
	var out strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			fmt.Fprintf(&out, "$%d", n)
		} else {
			out.WriteByte(q[i])
		}
	}
	return out.String()
}
