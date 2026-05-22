// HTTP search endpoint over the recent-events ring. This is the GET
// counterpart of the SSE stream: agent-service and the operator console
// use it to fetch the last N events without holding open a live stream.
//
// For production ClickHouse-backed deployments the same handler can sit
// in front of the PersistentStore — the Store interface owns the impl.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/event-service/internal/store"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// Search serves GET /v1/events.
//
// Query parameters:
//   - q            optional free-text — matched substring-wise against
//                  summary, class_label, rule_id (case-insensitive).
//   - stream_id    filter by stream
//   - limit        max items returned, clamped to [1, 500], default 50
type Search struct{ Store store.Store }

func (s *Search) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			problem.Unauthorized("X-Tenant-Id is required").Write(w)
			return
		}
		streamID := r.URL.Query().Get("stream_id")
		q := r.URL.Query().Get("q")
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
				if limit > 500 {
					limit = 500
				}
			}
		}
		events := s.Store.List(tenant, streamID, limit)
		if q != "" {
			events = filterByQuery(events, q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": eventsToJSON(events),
		})
	})
}

func filterByQuery(events []*dataplanev1.Event, q string) []*dataplanev1.Event {
	if q == "" {
		return events
	}
	ql := toLower(q)
	out := events[:0]
	for _, ev := range events {
		if containsFold(ev.GetSummary(), ql) ||
			containsFold(ev.GetClassLabel(), ql) ||
			containsFold(ev.GetRuleId(), ql) {
			out = append(out, ev)
		}
	}
	return out
}

func eventsToJSON(events []*dataplanev1.Event) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		out = append(out, map[string]any{
			"id":           ev.GetId(),
			"tenant_id":    ev.GetTenantId(),
			"pipeline_id":  ev.GetPipelineId(),
			"stream_id":    ev.GetStreamId(),
			"kind":         ev.GetKind().String(),
			"severity":     ev.GetSeverity().String(),
			"frame_seq":    ev.GetFrameSeq(),
			"capture_time": tsRFC3339(ev.GetCaptureTime()),
			"emitted_at":   tsRFC3339(ev.GetEmittedAt()),
			"summary":      ev.GetSummary(),
			"rule_id":      ev.GetRuleId(),
			"class_label":  ev.GetClassLabel(),
			"score":        ev.GetScore(),
		})
	}
	return out
}

// containsFold is the case-insensitive equivalent of strings.Contains. We
// avoid importing strings here because the rest of the package doesn't
// need it — keep the import set tight.
func containsFold(haystack, needleLower string) bool {
	hl := toLower(haystack)
	if len(needleLower) > len(hl) {
		return false
	}
	for i := 0; i+len(needleLower) <= len(hl); i++ {
		if hl[i:i+len(needleLower)] == needleLower {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
