// Package metrics is the RED-style metrics helper every service should use.
//
// The aim is uniformity: any operator looking at any service should see the
// same metric names, labels, and bucket layout. The dashboards in
// /observability ship with this contract baked in.
//
//	aegis_requests_total{service,method,route,status}
//	aegis_request_duration_seconds{service,method,route}
//	aegis_inflight_requests{service,method,route}
//
// Per-service custom metrics belong in the service's own registry, layered
// on top of the platform registry.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RED bundles the three RED metrics and binds them to a service name.
type RED struct {
	Service string

	Total    *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	Inflight *prometheus.GaugeVec
}

// New constructs a RED metrics bundle for the named service and registers it
// with the supplied registry. Use a fresh registry per service so tests can
// instantiate the platform in parallel without colliding on global state.
func New(service string, reg prometheus.Registerer) *RED {
	r := &RED{
		Service: service,
		Total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aegis_requests_total",
			Help: "Total number of completed requests.",
		}, []string{"service", "method", "route", "status"}),
		Duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aegis_request_duration_seconds",
			Help:    "Request latency in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"service", "method", "route"}),
		Inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aegis_inflight_requests",
			Help: "Number of in-flight requests.",
		}, []string{"service", "method", "route"}),
	}
	reg.MustRegister(r.Total, r.Duration, r.Inflight)
	return r
}

// Handler returns the Prometheus scrape endpoint for the given registry.
func Handler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
}

// RouteFromPattern is the recommended routeFn for HTTPMiddleware when the
// caller uses Go 1.22+ http.ServeMux (which sets r.Pattern to the matched
// "METHOD /path" pattern). Falls back to the URL path if Pattern is empty
// — e.g. for requests routed by a non-ServeMux handler.
func RouteFromPattern(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.URL.Path
}

// HTTPMiddleware records RED metrics for every request flowing through it.
// `routeFn` extracts a stable route label from the request (e.g. the matched
// chi/mux pattern); without a stable label, high-cardinality URLs would
// explode the time-series.
func (r *RED) HTTPMiddleware(routeFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			route := routeFn(req)
			method := req.Method

			r.Inflight.WithLabelValues(r.Service, method, route).Inc()
			defer r.Inflight.WithLabelValues(r.Service, method, route).Dec()

			rec := &statusRecorder{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(rec, req)
			elapsed := time.Since(start).Seconds()

			r.Duration.WithLabelValues(r.Service, method, route).Observe(elapsed)
			r.Total.WithLabelValues(r.Service, method, route, strconv.Itoa(rec.status)).Inc()
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.status = code
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush passes through to the underlying ResponseWriter if it supports it.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
