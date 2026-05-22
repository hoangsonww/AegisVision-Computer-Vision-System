// Package server hosts the realtime-hub HTTP endpoints. Two transports
// are supported on top of the same hub.Hub:
//
//   GET /v1/events:ws        — WebSocket (the primary transport;
//                              consumed by the console + first-party
//                              SDKs).
//   GET /v1/realtime:stream  — Server-Sent Events (for clients on
//                              constrained networks where WS is blocked).
//   GET /v1/realtime/stats   — JSON: subscriber count + dropped counter.
//
// The hub itself drops messages on the floor when a subscriber's
// channel is full — both transports are "best-effort live"; the
// authoritative record is in event-service (ClickHouse).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/realtime-hub/internal/hub"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

// Counters bundles the gauges + counter main.go owns; passed in so the
// routes can update them without each route needing its own registry.
type Counters struct {
	Subscribers prometheus.Gauge
	Dropped     prometheus.Counter
}

// Mux returns the routed handler.
func Mux(h *hub.Hub, c Counters) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/events:ws", wsHandler(h, c))
	mux.Handle("GET /v1/realtime:stream", sseHandler(h))
	mux.Handle("GET /v1/realtime/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"subscribers": h.Subscribers(),
			"dropped":     h.Dropped(),
		})
	}))
	return mux
}

// wsHandler is the primary transport. Origins are intentionally
// permissive at this layer — production locks them down via the
// gateway's CORS policy.
func wsHandler(h *hub.Hub, c Counters) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			problem.Unauthorized("X-Tenant-Id required").Write(w)
			return
		}
		streamID := r.URL.Query().Get("stream_id")
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:   []string{"aegis-realtime.v1"},
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		s := h.Subscribe(hub.Filter{TenantID: tenant, StreamID: streamID}, 256)
		defer h.Unsubscribe(s)
		if c.Subscribers != nil {
			c.Subscribers.Inc()
			defer c.Subscribers.Dec()
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-s.Out():
				if !ok {
					return
				}
				body, _ := json.Marshal(eventJSON(ev))
				wctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, body)
				cancel()
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					return
				}
			}
		}
	})
}

// sseHandler is the fallback transport for clients that can't open
// WebSocket connections.
func sseHandler(h *hub.Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			problem.Internal("streaming not supported").Write(w)
			return
		}
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			problem.Unauthorized("tenant required").Write(w)
			return
		}
		sub := h.Subscribe(hub.Filter{
			TenantID: tenant,
			StreamID: r.URL.Query().Get("stream_id"),
		}, 256)
		defer h.Unsubscribe(sub)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		_, _ = fmt.Fprintf(w, ": connected sub=%d\n\n", sub.ID())
		flusher.Flush()

		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-keepalive.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			case ev, ok := <-sub.Out():
				if !ok {
					return
				}
				body, _ := json.Marshal(eventJSON(ev))
				if _, err := fmt.Fprintf(w, "event: detection\ndata: %s\n\n", body); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
}

func eventJSON(ev *dataplanev1.Event) map[string]any {
	return map[string]any{
		"id": ev.GetId(), "tenant_id": ev.GetTenantId(),
		"pipeline_id": ev.GetPipelineId(), "stream_id": ev.GetStreamId(),
		"kind":     ev.GetKind().String(),
		"severity": ev.GetSeverity().String(),
		"summary":  ev.GetSummary(),
		"rule_id":  ev.GetRuleId(),
		"emitted_at": ev.GetEmittedAt().AsTime().UTC().Format(time.RFC3339Nano),
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
