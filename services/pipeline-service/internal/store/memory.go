// Package store is the persistence boundary for pipeline-service.
//
// The Phase 0 skeleton uses an in-memory implementation with the same
// interface a future Postgres-backed store will satisfy. The cursor-based
// listing is wired here because pagination semantics are easiest to validate
// against the same data structure that holds the items.
package store

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("pipeline: not found")

// ListFilter narrows a ListPipelines query.
type ListFilter struct {
	TenantID     string
	ProjectID    string
	NameContains string
	StatusIn     []controlv1.PipelineStatus
}

// PageQuery describes one page request after the cursor has been decoded.
type PageQuery struct {
	PageSize int
	// AfterID is the LastID from the decoded cursor; empty for the first page.
	AfterID string
}

// PageResult carries the items plus the cursor parameters for the next page.
type PageResult struct {
	Items     []*controlv1.Pipeline
	NextAfter string // empty when no more pages
	Total     int64  // -1 if not counted
}

// Store is the persistence interface pipeline-service depends on. Swap in a
// Postgres implementation by writing one that satisfies this interface; do
// not add Postgres-specific signatures here.
type Store interface {
	Create(p *controlv1.Pipeline) (*controlv1.Pipeline, error)
	Get(id string) (*controlv1.Pipeline, error)
	List(f ListFilter, q PageQuery) (PageResult, error)
	UpdateStatus(id string, status controlv1.PipelineStatus) (*controlv1.Pipeline, error)
}

// MemoryStore is goroutine-safe and good enough for unit tests and the
// walking skeleton. It orders results by pipeline ID for deterministic
// pagination — same property a Postgres ORDER BY primary key gives us.
type MemoryStore struct {
	mu        sync.RWMutex
	pipelines map[string]*controlv1.Pipeline
	now       func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		pipelines: make(map[string]*controlv1.Pipeline),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (m *MemoryStore) Create(p *controlv1.Pipeline) (*controlv1.Pipeline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.pipelines[p.GetId()]; exists {
		return nil, errors.New("pipeline: id already exists")
	}
	now := timestamppb.New(m.now())
	if p.Audit == nil {
		p.Audit = &commonv1.AuditFields{}
	}
	p.Audit.CreatedAt = now
	p.Audit.UpdatedAt = now
	p.Audit.Version = 1
	if p.Status == controlv1.PipelineStatus_PIPELINE_STATUS_UNSPECIFIED {
		p.Status = controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT
	}
	m.pipelines[p.GetId()] = p
	return p, nil
}

func (m *MemoryStore) Get(id string) (*controlv1.Pipeline, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pipelines[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (m *MemoryStore) UpdateStatus(id string, status controlv1.PipelineStatus) (*controlv1.Pipeline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pipelines[id]
	if !ok {
		return nil, ErrNotFound
	}
	p.Status = status
	if p.Audit == nil {
		p.Audit = &commonv1.AuditFields{}
	}
	p.Audit.UpdatedAt = timestamppb.New(m.now())
	p.Audit.Version++
	return p, nil
}

func (m *MemoryStore) List(f ListFilter, q PageQuery) (PageResult, error) {
	m.mu.RLock()
	ids := make([]string, 0, len(m.pipelines))
	for id := range m.pipelines {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	sort.Strings(ids)

	var out []*controlv1.Pipeline
	startScan := q.AfterID == ""

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range ids {
		if !startScan {
			if id == q.AfterID {
				startScan = true
			}
			continue
		}
		p := m.pipelines[id]
		if !matchesFilter(p, f) {
			continue
		}
		out = append(out, p)
		if len(out) == q.PageSize {
			break
		}
	}

	next := ""
	if len(out) == q.PageSize {
		next = out[len(out)-1].GetId()
	}
	return PageResult{Items: out, NextAfter: next, Total: -1}, nil
}

func matchesFilter(p *controlv1.Pipeline, f ListFilter) bool {
	if f.TenantID != "" && p.GetTenant().GetId() != f.TenantID {
		return false
	}
	if f.ProjectID != "" && p.GetProject().GetId() != f.ProjectID {
		return false
	}
	if f.NameContains != "" && !strings.Contains(p.GetName(), f.NameContains) {
		return false
	}
	if len(f.StatusIn) > 0 {
		ok := false
		for _, s := range f.StatusIn {
			if p.GetStatus() == s {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Seed populates the store with one example pipeline so the walking skeleton
// has something to list out of the box. Returns the pipeline ID.
func (m *MemoryStore) Seed(tenantID, projectID, name string) string {
	id := "p-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	_, _ = m.Create(&controlv1.Pipeline{
		Id:              id,
		Tenant:          &commonv1.TenantRef{Id: tenantID},
		Project:         &commonv1.ProjectRef{Id: projectID, TenantId: tenantID},
		Name:            name,
		Description:     "seeded by pipeline-service for the walking skeleton",
		Labels:          []string{"seed", "skeleton"},
		Status:          controlv1.PipelineStatus_PIPELINE_STATUS_VALIDATED,
		CurrentRevision: 1,
		DagJson:         `{"nodes":[{"id":"detect","type":"model.infer"}]}`,
	})
	return id
}
