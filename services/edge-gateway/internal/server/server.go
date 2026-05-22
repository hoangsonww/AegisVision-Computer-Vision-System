// Package server hosts the edge-gateway's HTTP surface — small but
// load-bearing endpoints camera-site operators and oncall use to:
//
//   POST /v1/forward            — accept a single event for forwarding.
//     The original raw-body form (subject in `?subject=`, payload as
//     the request body) is kept for backwards compatibility with the
//     local data-plane operators that already POST here. A JSON envelope
//     is also accepted: `{"subject": "...", "data": <opaque>}`.
//   POST /v1/forward:batch      — batch JSON variant.
//   GET  /v1/queue/stats        — depth, byte count, oldest item age.
//   POST /v1/queue:flush        — operator-triggered drain (use sparingly).
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
)

// Stats is the diagnostic view of the store-and-forward queue.
type Stats struct {
	Items            int   `json:"items"`
	Bytes            int64 `json:"bytes"`
	OldestAgeSeconds int64 `json:"oldest_age_seconds"`
}

// Forwarder is the contract the HTTP server uses to enqueue events.
// The real implementation in main.go writes to the disk-backed queue.
type Forwarder interface {
	// Forward enqueues one item under the given bus subject. Returns
	// the assigned sequence number, or an error on disk-full /
	// marshalling failure.
	Forward(ctx context.Context, subject string, payload []byte) (uint64, error)
	// Stats returns the queue's current depth + byte count + oldest age.
	Stats() Stats
	// FlushNow triggers an immediate drain attempt (operator-only).
	// Returns the number of items drained.
	FlushNow(ctx context.Context) (int, error)
}

// Mux builds the HTTP handler.
func Mux(fwd Forwarder) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/forward", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		var subject string
		var payload []byte
		if ct == "application/json" {
			var body forwardRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				problem.BadRequest("invalid JSON envelope").Write(w)
				return
			}
			subject = body.Subject
			payload = body.Data
		} else {
			subject = r.URL.Query().Get("subject")
			b, err := io.ReadAll(r.Body)
			if err != nil {
				problem.BadRequest("could not read body").Write(w)
				return
			}
			payload = b
		}
		if subject == "" {
			problem.BadRequest("subject required (envelope or ?subject=)").Write(w)
			return
		}
		seq, err := fwd.Forward(r.Context(), subject, payload)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.Header().Set("X-Aegis-Edge-Seq", strconv.FormatUint(seq, 10))
		w.WriteHeader(http.StatusAccepted)
	}))

	mux.Handle("POST /v1/forward:batch", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []forwardRequest
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			problem.BadRequest("expected array of forward requests").Write(w)
			return
		}
		accepted := 0
		for _, body := range batch {
			if body.Subject == "" {
				continue
			}
			if _, err := fwd.Forward(r.Context(), body.Subject, body.Data); err == nil {
				accepted++
			}
		}
		writeJSON(w, http.StatusAccepted, map[string]int{"accepted": accepted, "total": len(batch)})
	}))

	mux.Handle("GET /v1/queue/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, fwd.Stats())
	}))

	mux.Handle("POST /v1/queue:flush", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operator := r.Header.Get("X-Aegis-Subject")
		if operator == "" && logging.TenantID(r.Context()) == "" {
			problem.Forbidden("operator identity required").Write(w)
			return
		}
		n, err := fwd.FlushNow(r.Context())
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"drained": n})
	}))

	return mux
}

type forwardRequest struct {
	Subject string          `json:"subject"`
	Data    json.RawMessage `json:"data"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
