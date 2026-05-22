// In-memory store for Stream resources. The interface is intentionally
// narrow; a Phase 2 Postgres implementation drops in by satisfying it.
package store

import (
	"errors"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrNotFound = errors.New("stream: not found")

type Store interface {
	Create(in *controlv1.CreateStreamRequest) (*controlv1.Stream, error)
	Get(id string) (*controlv1.Stream, error)
	Delete(id string) error
	List(tenantID, projectID, siteID string, health []controlv1.StreamHealth, pageSize int, after string) ([]*controlv1.Stream, string, error)
	UpdateHealth(id string, h controlv1.StreamHealth, observedFPS float64, observedKbps int64) (*controlv1.Stream, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	streams map[string]*controlv1.Stream
	now     func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{streams: map[string]*controlv1.Stream{}, now: func() time.Time { return time.Now().UTC() }}
}

func (s *MemoryStore) Create(in *controlv1.CreateStreamRequest) (*controlv1.Stream, error) {
	if in.GetName() == "" {
		return nil, errors.New("stream: name is required")
	}
	if in.GetTenant().GetId() == "" {
		return nil, errors.New("stream: tenant.id is required")
	}
	if in.GetProject().GetId() == "" {
		return nil, errors.New("stream: project.id is required")
	}
	if in.GetUrl() == "" {
		return nil, errors.New("stream: url is required")
	}
	redacted := redactURL(in.GetUrl())

	id := "stream-" + strings.ToLower(strings.ReplaceAll(in.GetName(), " ", "-"))
	st := &controlv1.Stream{
		Id:           id,
		Tenant:       in.GetTenant(),
		Project:      in.GetProject(),
		CameraId:     in.GetCameraId(),
		SiteId:       in.GetSiteId(),
		Name:         in.GetName(),
		Protocol:     in.GetProtocol(),
		UrlRedacted:  redacted,
		Health:       controlv1.StreamHealth_STREAM_HEALTH_DISCONNECTED,
		ObservedFps:  0,
		ObservedKbps: 0,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(s.now()),
			UpdatedAt: timestamppb.New(s.now()),
			Version:   1,
		},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.streams[id]; exists {
		return nil, errors.New("stream: id already exists")
	}
	s.streams[id] = st
	return st, nil
}

func (s *MemoryStore) Get(id string) (*controlv1.Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.streams[id]
	if !ok {
		return nil, ErrNotFound
	}
	return st, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.streams[id]; !ok {
		return ErrNotFound
	}
	delete(s.streams, id)
	return nil
}

func (s *MemoryStore) List(tenantID, projectID, siteID string, health []controlv1.StreamHealth, pageSize int, after string) ([]*controlv1.Stream, string, error) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.streams))
	for id := range s.streams {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)

	var out []*controlv1.Stream
	startScan := after == ""
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range ids {
		if !startScan {
			if id == after {
				startScan = true
			}
			continue
		}
		st := s.streams[id]
		if tenantID != "" && st.GetTenant().GetId() != tenantID {
			continue
		}
		if projectID != "" && st.GetProject().GetId() != projectID {
			continue
		}
		if siteID != "" && st.GetSiteId() != siteID {
			continue
		}
		if len(health) > 0 {
			ok := false
			for _, h := range health {
				if st.GetHealth() == h {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, st)
		if pageSize > 0 && len(out) == pageSize {
			break
		}
	}
	next := ""
	if pageSize > 0 && len(out) == pageSize {
		next = out[len(out)-1].GetId()
	}
	return out, next, nil
}

func (s *MemoryStore) UpdateHealth(id string, h controlv1.StreamHealth, fps float64, kbps int64) (*controlv1.Stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.streams[id]
	if !ok {
		return nil, ErrNotFound
	}
	st.Health = h
	st.ObservedFps = fps
	st.ObservedKbps = kbps
	if st.Audit == nil {
		st.Audit = &commonv1.AuditFields{}
	}
	st.Audit.UpdatedAt = timestamppb.New(s.now())
	st.Audit.Version++
	st.LastFrameAt = timestamppb.New(s.now())
	return st, nil
}

// redactURL strips embedded credentials and query strings before returning a
// URL to a client. Cameras commonly use rtsp://user:pass@host/... — leaving
// those credentials in the API response was a chronic information-leak in
// CV platforms we modeled this after.
func redactURL(in string) string {
	u, err := url.Parse(in)
	if err != nil {
		return "redacted"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
