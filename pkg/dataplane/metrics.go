// Data-plane RED-style metrics. The cardinality is bounded by (pipeline,
// operator); we deliberately do NOT label by stream because at 10k streams
// per cluster the time series explodes.
package dataplane

import "github.com/prometheus/client_golang/prometheus"

// Metrics is the bundle of data-plane operator metrics. Construct once per
// runtime and pass it to every operator-hosting goroutine.
type Metrics struct {
	FramesProcessed *prometheus.CounterVec
	FramesDropped   *prometheus.CounterVec
	OperatorLatency *prometheus.HistogramVec
	QueueDepth      *prometheus.GaugeVec
}

// NewMetrics registers the bundle with the supplied registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		FramesProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aegis_dataplane_frames_processed_total",
			Help: "Frames that left an operator successfully.",
		}, []string{"pipeline", "operator"}),
		FramesDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aegis_dataplane_frames_dropped_total",
			Help: "Frames an operator dropped (sampler, oversubscribe).",
		}, []string{"pipeline", "operator", "reason"}),
		OperatorLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aegis_dataplane_operator_seconds",
			Help:    "Per-operator processing time per frame.",
			Buckets: []float64{.0005, .001, .002, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"pipeline", "operator"}),
		QueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aegis_dataplane_queue_depth",
			Help: "Current depth of the bounded channel feeding an operator.",
		}, []string{"pipeline", "operator"}),
	}
	reg.MustRegister(m.FramesProcessed, m.FramesDropped, m.OperatorLatency, m.QueueDepth)
	return m
}
