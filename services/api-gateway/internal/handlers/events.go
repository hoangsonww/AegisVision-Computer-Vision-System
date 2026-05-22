// SSE proxy: the api-gateway authenticates the caller, attaches the tenant
// header, and pipes the SSE bytes from event-service back to the client.
// Hop-by-hop headers are stripped; X-Request-Id is propagated.
package handlers

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
)

// EventStreamProxy proxies /v1/events:stream to the upstream event-service.
type EventStreamProxy struct {
	Upstream string // e.g. http://event-service:8090
	Client   *http.Client
}

func NewEventStreamProxy(upstream string) *EventStreamProxy {
	if upstream == "" {
		return &EventStreamProxy{Client: &http.Client{Timeout: 0}}
	}
	return &EventStreamProxy{
		Upstream: upstream,
		Client:   &http.Client{Timeout: 0},
	}
}

func (p *EventStreamProxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.Upstream == "" {
			problem.ServiceUnavailable("event-service not configured").Write(w)
			return
		}
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			problem.Unauthorized("X-Tenant-Id is required").Write(w)
			return
		}

		// Build the upstream URL with the same query parameters.
		upURL, err := url.Parse(p.Upstream)
		if err != nil {
			problem.Internal("invalid upstream URL").Write(w)
			return
		}
		upURL.Path = "/v1/events:stream"
		upURL.RawQuery = r.URL.RawQuery

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upURL.String(), nil)
		if err != nil {
			problem.Internal("upstream request build failed").Write(w)
			return
		}
		req.Header.Set("X-Tenant-Id", tenant)
		req.Header.Set("Accept", "text/event-stream")
		if rid := logging.RequestID(r.Context()); rid != "" {
			req.Header.Set("X-Request-Id", rid)
		}

		resp, err := p.Client.Do(req)
		if err != nil {
			problem.ServiceUnavailable("event-service unreachable").Write(w)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// Mirror the upstream status.
			body, _ := io.ReadAll(resp.Body)
			w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			problem.Internal("response writer does not support flushing").Write(w)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		br := bufio.NewReader(resp.Body)
		hb := time.NewTicker(10 * time.Second)
		defer hb.Stop()

		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			for {
				line, err := br.ReadBytes('\n')
				if len(line) > 0 {
					_, _ = w.Write(line)
					flusher.Flush()
				}
				if err != nil {
					return
				}
			}
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-readDone:
				return
			case <-hb.C:
				if _, err := fmt.Fprintf(w, ": gateway-keepalive %d\n\n", time.Now().Unix()); err != nil {
					if errors.Is(err, io.ErrClosedPipe) {
						return
					}
				}
				flusher.Flush()
			}
		}
	})
}
