package ledger_test

import (
	"errors"
	"testing"
	"time"

	"github.com/aegisvision/services/gpu-scheduler/internal/ledger"
)

func gigs(n int64) int64 { return n * 1024 * 1024 * 1024 }

func TestPlace_BestFitByFreeVRAM(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "h100", SKU: "H100", TotalVRAM: gigs(80), TotalSlices: 7})
	_ = l.RegisterGPU(ledger.GPU{ID: "l4", SKU: "L4", TotalVRAM: gigs(24), TotalSlices: 4})

	r, err := l.Place(ledger.PlacementRequest{
		Model: "yolo", ModelVersion: "1",
		Profile: ledger.MIGProfile{Name: "1g.10gb", VRAMBytes: gigs(10), SlicesUsed: 1},
		HoldFor: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Best fit chooses the smaller-free GPU (L4, 24GB) over the H100 (80GB).
	if r.GPU != "l4" {
		t.Fatalf("want l4, got %q", r.GPU)
	}
}

func TestPlace_RespectsTenantLock(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "L4", TotalVRAM: gigs(24), TotalSlices: 4, Tenant: "t-x"})
	_, err := l.Place(ledger.PlacementRequest{
		Model: "x", TenantID: "t-other",
		Profile: ledger.MIGProfile{VRAMBytes: gigs(1), SlicesUsed: 1},
	})
	if !errors.Is(err, ledger.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity for cross-tenant, got %v", err)
	}
	r, err := l.Place(ledger.PlacementRequest{
		Model: "x", TenantID: "t-x",
		Profile: ledger.MIGProfile{VRAMBytes: gigs(1), SlicesUsed: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.GPU != "g0" {
		t.Fatalf("tenant-locked GPU: got %q", r.GPU)
	}
}

func TestPlace_OutOfCapacityWhenVRAMUsed(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "L4", TotalVRAM: gigs(10), TotalSlices: 1})
	if _, err := l.Place(ledger.PlacementRequest{
		Model: "a", Profile: ledger.MIGProfile{VRAMBytes: gigs(6), SlicesUsed: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Place(ledger.PlacementRequest{
		Model: "b", Profile: ledger.MIGProfile{VRAMBytes: gigs(6), SlicesUsed: 1},
	}); !errors.Is(err, ledger.ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity, got %v", err)
	}
}

func TestRelease_FreesUpCapacity(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "L4", TotalVRAM: gigs(10), TotalSlices: 1})
	r, err := l.Place(ledger.PlacementRequest{
		Model: "a", Profile: ledger.MIGProfile{VRAMBytes: gigs(10), SlicesUsed: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Place(ledger.PlacementRequest{
		Model: "b", Profile: ledger.MIGProfile{VRAMBytes: gigs(1), SlicesUsed: 1},
	}); !errors.Is(err, ledger.ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity, got %v", err)
	}
	if err := l.Release(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Place(ledger.PlacementRequest{
		Model: "b", Profile: ledger.MIGProfile{VRAMBytes: gigs(1), SlicesUsed: 1},
	}); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestPlace_NVLinkPreferenceWins(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "H100", TotalVRAM: gigs(80), TotalSlices: 1, NVLinkGroup: "rack-A"})
	_ = l.RegisterGPU(ledger.GPU{ID: "g1", SKU: "H100", TotalVRAM: gigs(80), TotalSlices: 1, NVLinkGroup: "rack-B"})
	r, err := l.Place(ledger.PlacementRequest{
		Model: "vlm", Profile: ledger.MIGProfile{VRAMBytes: gigs(40), SlicesUsed: 1},
		PreferNVLink: "rack-B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.GPU != "g1" {
		t.Fatalf("nvlink preference ignored: chose %q", r.GPU)
	}
}

func TestFreeVRAM_Accounting(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "L4", TotalVRAM: gigs(24), TotalSlices: 4})
	r, _ := l.Place(ledger.PlacementRequest{Model: "x", Profile: ledger.MIGProfile{VRAMBytes: gigs(10), SlicesUsed: 1}})
	free, err := l.FreeVRAM("g0")
	if err != nil {
		t.Fatal(err)
	}
	if want := gigs(14); free != want {
		t.Fatalf("free vram: want %d, got %d", want, free)
	}
	_ = r
}

func TestSnapshot_Deterministic(t *testing.T) {
	l := ledger.New()
	_ = l.RegisterGPU(ledger.GPU{ID: "g0", SKU: "L4", TotalVRAM: gigs(24), TotalSlices: 4})
	_ = l.RegisterGPU(ledger.GPU{ID: "g1", SKU: "L4", TotalVRAM: gigs(24), TotalSlices: 4})
	s := l.Snapshot()
	if len(s.GPUs) != 2 {
		t.Fatalf("want 2 gpus, got %d", len(s.GPUs))
	}
	if s.GPUs[0].ID != "g0" || s.GPUs[1].ID != "g1" {
		t.Fatalf("snapshot not sorted: %+v", s.GPUs)
	}
}
