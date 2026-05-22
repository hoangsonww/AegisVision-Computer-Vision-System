// HTTP Server-Sent Events stream for the console. Each SSE event is a JSON
// projection of the Event proto with an "ts" field set on the SSE side so
// clients can compute server-time delivery latency. The endpoint is
// tenant-bound — tenants are propagated from X-Tenant-Id like every other
// public endpoint.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
	"github.com/aegisvision/services/event-service/internal/store"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// SSE serves GET /v1/events:stream?stream_id=...
type SSE struct {
	Store store.Store
}

func (s *SSE) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			problem.Unauthorized("X-Tenant-Id is required").Write(w)
			return
		}
		streamID := r.URL.Query().Get("stream_id")

		flusher, ok := w.(http.Flusher)
		if !ok {
			problem.Internal("response writer does not support flushing").Write(w)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // Nginx hint
		w.WriteHeader(http.StatusOK)

		_, ch, cancel := s.Store.Subscribe(tenant, streamID, 256)
		defer cancel()

		// Heartbeat every 10s so intermediaries don't reap idle connections.
		hb := time.NewTicker(10 * time.Second)
		defer hb.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-hb.C:
				_, _ = fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
				flusher.Flush()
			case ev, ok := <-ch:
				if !ok {
					return
				}
				writeSSEEvent(w, flusher, ev)
			}
		}
	})
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev *dataplanev1.Event) {
	payload := map[string]any{
		"id":           ev.GetId(),
		"tenant_id":    ev.GetTenantId(),
		"pipeline_id":  ev.GetPipelineId(),
		"stream_id":    ev.GetStreamId(),
		"kind":         ev.GetKind().String(),
		"severity":     ev.GetSeverity().String(),
		"frame_seq":    ev.GetFrameSeq(),
		"capture_time": tsRFC3339(ev.GetCaptureTime()),
		"emitted_at":   tsRFC3339(ev.GetEmittedAt()),
		"server_ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"summary":      ev.GetSummary(),
		"rule_id":      ev.GetRuleId(),
		"zone_id":      ev.GetZoneId(),
		"track_id":     ev.GetTrackId(),
		"class_label":  ev.GetClassLabel(),
		"score":        ev.GetScore(),
		"bbox":         map[string]float32{"x": ev.GetBboxX(), "y": ev.GetBboxY(), "w": ev.GetBboxW(), "h": ev.GetBboxH()},
	}
	body, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\n", "event")
	_, _ = fmt.Fprintf(w, "id: %s\n", ev.GetId())
	_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
	flusher.Flush()
}

func tsRFC3339(t interface {
	AsTime() time.Time
}) string {
	if t == nil {
		return ""
	}
	return t.AsTime().UTC().Format(time.RFC3339Nano)
}
