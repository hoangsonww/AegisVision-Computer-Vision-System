package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/tenant-service/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return store.New(db, false)
}

func TestTenant_CreateAppliesTierDefaults(t *testing.T) {
	s := newStore(t)
	if err := s.CreateTenant(context.Background(), store.Tenant{
		ID: "t-1", Name: "Acme", Tier: store.TierStandard, CryptoKeyID: "vault://aegisvision/tenants/t-1",
	}); err != nil {
		t.Fatal(err)
	}
	qs, err := s.ListQuotas(context.Background(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) < 5 {
		t.Fatalf("standard tier should seed ≥5 quotas, got %d", len(qs))
	}
}

func TestTenant_EnterpriseQuotasLargerThanStandard(t *testing.T) {
	s := newStore(t)
	_ = s.CreateTenant(context.Background(), store.Tenant{ID: "t-std", Name: "std", Tier: store.TierStandard, CryptoKeyID: "k"})
	_ = s.CreateTenant(context.Background(), store.Tenant{ID: "t-ent", Name: "ent", Tier: store.TierEnterprise, CryptoKeyID: "k"})
	std, _ := s.GetQuota(context.Background(), "t-std", "streams")
	ent, _ := s.GetQuota(context.Background(), "t-ent", "streams")
	if !(ent.HardLimit > std.HardLimit) {
		t.Fatalf("enterprise streams should exceed standard: ent=%d std=%d", ent.HardLimit, std.HardLimit)
	}
}

func TestTenant_SoftDeleteMarks(t *testing.T) {
	s := newStore(t)
	_ = s.CreateTenant(context.Background(), store.Tenant{ID: "t", Name: "x", Tier: store.TierFree, CryptoKeyID: "k"})
	_ = s.SoftDelete(context.Background(), "t")
	got, _ := s.GetTenant(context.Background(), "t")
	if got.State != store.StateDeleted || !got.DeletedAt.Valid {
		t.Fatalf("soft delete not applied: %+v", got)
	}
}

func TestDSAR_RequestAndComplete(t *testing.T) {
	s := newStore(t)
	_ = s.CreateTenant(context.Background(), store.Tenant{ID: "t", Name: "x", Tier: store.TierStandard, CryptoKeyID: "k"})
	if err := s.CreateDSAR(context.Background(), store.DSAR{
		ID: "dsar-1", TenantID: "t", Subject: "user@example.com", Kind: store.DSARAccess, RequestedBy: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteDSAR(context.Background(), "dsar-1", "s3://exports/dsar-1.zip"); err != nil {
		t.Fatal(err)
	}
	ls, _ := s.ListDSAR(context.Background(), "t")
	if len(ls) != 1 || ls[0].Status != "completed" || ls[0].ArtifactURI == "" {
		t.Fatalf("dsar lifecycle: %+v", ls)
	}
}
