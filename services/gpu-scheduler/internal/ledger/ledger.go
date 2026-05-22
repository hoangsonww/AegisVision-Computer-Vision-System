// Package ledger is the heart of gpu-scheduler.
//
// Per ADR-0003 and the Matroid scar from the architecture doc §12: the
// scheduler must NOT trust `nvidia-smi free memory`. The ledger holds the
// authoritative `physical_total − Σ(reserved_by_resident_models)` view, and
// every Place / Release / Profile call updates it atomically.
//
// Production deployments persist the ledger in Redis (Cluster) with a
// per-tenant + per-GPU key; this in-process implementation is the canonical
// reference for the algorithm and the unit-test target.
package ledger

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MIGProfile names follow NVIDIA conventions ("1g.10gb", "2g.20gb", ...).
// We model the slice count + VRAM cap explicitly so the ledger doesn't have
// to parse the name at decision time.
type MIGProfile struct {
	Name      string
	VRAMBytes int64
	// SlicesUsed reflects the share of the physical GPU this profile takes.
	// Sum of slices across active reservations on one GPU must not exceed
	// the GPU's TotalSlices.
	SlicesUsed int
}

// GPU is one device the scheduler owns.
type GPU struct {
	ID            string // stable name, e.g. "gpu-0" or the K8s nvidia.com/gpu-X label
	SKU           string // e.g. "L4", "L40S", "H100"
	TotalVRAM     int64
	TotalSlices   int    // for MIG-capable GPUs; 1 for whole-card sharing
	NUMANode      int
	NVLinkGroup   string // optional; pods with NVLink advantage prefer same group
	Tenant        string // when set, GPU is dedicated to this tenant
}

// Reservation is a placement decision recorded in the ledger.
type Reservation struct {
	ID         string
	GPU        string
	Model      string
	ModelVer   string
	Profile    MIGProfile
	HoldFor    time.Duration // hint; not enforced by the ledger directly
	StartedAt  time.Time
	TenantID   string
	PipelineID string
}

// Ledger is goroutine-safe. The mutex is held only for the duration of a
// math step; the hot path (Place) is O(GPUs) which is fine — production
// fleets have tens of GPUs per node and thousands per cluster.
type Ledger struct {
	mu     sync.Mutex
	gpus   map[string]*GPU
	resv   map[string]*Reservation
	byGPU  map[string]map[string]*Reservation
	clock  func() time.Time
}

// New returns an empty Ledger.
func New() *Ledger {
	return &Ledger{
		gpus:  map[string]*GPU{},
		resv:  map[string]*Reservation{},
		byGPU: map[string]map[string]*Reservation{},
		clock: func() time.Time { return time.Now().UTC() },
	}
}

// RegisterGPU adds or replaces a GPU in the ledger. Idempotent.
func (l *Ledger) RegisterGPU(g GPU) error {
	if g.ID == "" {
		return errors.New("ledger: GPU.ID is required")
	}
	if g.TotalVRAM <= 0 {
		return errors.New("ledger: TotalVRAM must be positive")
	}
	if g.TotalSlices <= 0 {
		g.TotalSlices = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gpus[g.ID] = &g
	if _, ok := l.byGPU[g.ID]; !ok {
		l.byGPU[g.ID] = map[string]*Reservation{}
	}
	return nil
}

// PlacementRequest is what the inference-router asks for.
type PlacementRequest struct {
	Model         string
	ModelVersion  string
	TenantID      string
	PipelineID    string
	Profile       MIGProfile // requested MIG/VRAM footprint
	PreferNVLink  string     // optional NVLink group preference
	PreferSKU     string     // optional, e.g. "L4"
	HoldFor       time.Duration
}

// ErrNoCapacity is returned when no GPU can satisfy the request.
var ErrNoCapacity = errors.New("ledger: no capacity")

// Place picks a GPU and reserves the requested slice. Returns the reservation
// or ErrNoCapacity. The placement preference order:
//
//	1. Tenant-locked GPU matches first.
//	2. Same NVLink group as a preference if set.
//	3. Best-fit by remaining VRAM (smallest sufficient bucket).
//	4. Round-robin between equal candidates by GPU ID lexicographic order.
func (l *Ledger) Place(req PlacementRequest) (*Reservation, error) {
	if req.Model == "" {
		return nil, errors.New("ledger: Model required")
	}
	if req.Profile.VRAMBytes <= 0 {
		return nil, errors.New("ledger: Profile.VRAMBytes required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	type cand struct {
		gpu      *GPU
		freeVRAM int64
		freeSlot int
	}
	var cands []cand
	for _, g := range l.gpus {
		if g.Tenant != "" && g.Tenant != req.TenantID {
			continue
		}
		if req.PreferSKU != "" && g.SKU != req.PreferSKU {
			continue
		}
		usedVRAM := int64(0)
		usedSlots := 0
		for _, r := range l.byGPU[g.ID] {
			usedVRAM += r.Profile.VRAMBytes
			usedSlots += r.Profile.SlicesUsed
		}
		freeVRAM := g.TotalVRAM - usedVRAM
		freeSlots := g.TotalSlices - usedSlots
		if freeVRAM < req.Profile.VRAMBytes {
			continue
		}
		if freeSlots < req.Profile.SlicesUsed {
			continue
		}
		cands = append(cands, cand{g, freeVRAM, freeSlots})
	}
	if len(cands) == 0 {
		return nil, ErrNoCapacity
	}
	sort.SliceStable(cands, func(i, j int) bool {
		// NVLink preference takes precedence.
		if req.PreferNVLink != "" {
			ai := cands[i].gpu.NVLinkGroup == req.PreferNVLink
			aj := cands[j].gpu.NVLinkGroup == req.PreferNVLink
			if ai != aj {
				return ai
			}
		}
		// Best fit by free VRAM (smaller = tighter packing).
		if cands[i].freeVRAM != cands[j].freeVRAM {
			return cands[i].freeVRAM < cands[j].freeVRAM
		}
		return cands[i].gpu.ID < cands[j].gpu.ID
	})
	g := cands[0].gpu
	r := &Reservation{
		ID:         newResID(req.Model, g.ID),
		GPU:        g.ID,
		Model:      req.Model,
		ModelVer:   req.ModelVersion,
		Profile:    req.Profile,
		HoldFor:    req.HoldFor,
		StartedAt:  l.clock(),
		TenantID:   req.TenantID,
		PipelineID: req.PipelineID,
	}
	l.resv[r.ID] = r
	l.byGPU[g.ID][r.ID] = r
	return r, nil
}

// Release frees a reservation. Idempotent.
func (l *Ledger) Release(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.resv[id]
	if !ok {
		return nil
	}
	delete(l.resv, id)
	if m, ok := l.byGPU[r.GPU]; ok {
		delete(m, id)
	}
	return nil
}

// FreeVRAM returns the live free VRAM on a GPU.
func (l *Ledger) FreeVRAM(gpuID string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	g, ok := l.gpus[gpuID]
	if !ok {
		return 0, fmt.Errorf("ledger: unknown gpu %q", gpuID)
	}
	used := int64(0)
	for _, r := range l.byGPU[gpuID] {
		used += r.Profile.VRAMBytes
	}
	return g.TotalVRAM - used, nil
}

// Snapshot returns a copy of the current state for /v1/scheduler/state.
type Snapshot struct {
	GPUs         []*GPU
	Reservations []*Reservation
}

// Snapshot returns a deep-enough copy that callers can safely render it.
func (l *Ledger) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	gs := make([]*GPU, 0, len(l.gpus))
	for _, g := range l.gpus {
		cp := *g
		gs = append(gs, &cp)
	}
	rs := make([]*Reservation, 0, len(l.resv))
	for _, r := range l.resv {
		cp := *r
		rs = append(rs, &cp)
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].ID < gs[j].ID })
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	return Snapshot{GPUs: gs, Reservations: rs}
}

func newResID(model, gpu string) string {
	return strings.ToLower(strings.ReplaceAll(model+"-on-"+gpu, " ", "-")) +
		"-" + fmt.Sprintf("%x", time.Now().UnixNano())
}
