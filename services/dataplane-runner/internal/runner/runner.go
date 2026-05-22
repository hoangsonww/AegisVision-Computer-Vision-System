// Package runner subscribes to operator.control.<pipeline_id>, materializes
// pipelines onto streams, and runs the operator DAG.
//
// One Pipeline runs per (pipeline_id, stream_id) tuple. Re-issuing a "start"
// command for the same tuple is a no-op (idempotent reconcile). A "stop"
// command cancels the pipeline's context.
//
// For the walking skeleton the operator chain is hard-coded; Phase 2 will
// read the DAG from the Pipeline resource and instantiate operators by name.
package runner

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	"github.com/aegisvision/pkg/dataplane/operators"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"

	"github.com/prometheus/client_golang/prometheus"
)

// Command is the JSON shape stream-manager publishes onto operator.control.*
type Command struct {
	Op         string `json:"op"`
	StreamID   string `json:"stream_id"`
	TenantID   string `json:"tenant_id"`
	PipelineID string `json:"pipeline_id"`
	Protocol   string `json:"protocol"`
	URL        string `json:"url_redacted"`
}

// Runner is the per-process pipeline dispatcher.
type Runner struct {
	Bus       bus.Subscriber
	Publisher bus.Publisher
	Logger    *slog.Logger
	Registry  *claimcheck.Registry
	Metrics   *dataplane.Metrics

	mu      sync.Mutex
	running map[string]context.CancelFunc // key = pipeline_id + ":" + stream_id
	subs    []bus.Subscription
}

// Watch subscribes to operator.control.<pipeline_id>. The runner can be told
// to watch multiple pipelines by calling Watch multiple times.
func (r *Runner) Watch(ctx context.Context, pipelineID string) error {
	if r.running == nil {
		r.running = map[string]context.CancelFunc{}
	}
	sub, err := r.Bus.Subscribe(ctx, "operator.control."+pipelineID, "runners-"+pipelineID, func(_ context.Context, m bus.Message) error {
		var cmd Command
		if err := json.Unmarshal(m.Data, &cmd); err != nil {
			r.Logger.Warn("bad control message", slog.String("err", err.Error()))
			return nil
		}
		r.handle(ctx, cmd)
		return nil
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.subs = append(r.subs, sub)
	r.mu.Unlock()
	return nil
}

// WatchAll subscribes to operator.control.* — i.e. every pipeline a single
// runner pool serves. Used by the walking-skeleton single-runner topology.
func (r *Runner) WatchAll(ctx context.Context) error {
	if r.running == nil {
		r.running = map[string]context.CancelFunc{}
	}
	sub, err := r.Bus.Subscribe(ctx, "operator.control.>", "runners", func(_ context.Context, m bus.Message) error {
		var cmd Command
		if err := json.Unmarshal(m.Data, &cmd); err != nil {
			r.Logger.Warn("bad control message", slog.String("err", err.Error()))
			return nil
		}
		r.handle(ctx, cmd)
		return nil
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.subs = append(r.subs, sub)
	r.mu.Unlock()
	return nil
}

// handle starts or stops a pipeline based on the command.
func (r *Runner) handle(parent context.Context, cmd Command) {
	key := cmd.PipelineID + ":" + cmd.StreamID
	switch cmd.Op {
	case "start":
		r.mu.Lock()
		if _, exists := r.running[key]; exists {
			r.mu.Unlock()
			r.Logger.Debug("pipeline already running", slog.String("key", key))
			return
		}
		ctx, cancel := context.WithCancel(parent)
		r.running[key] = cancel
		r.mu.Unlock()
		go r.runOne(ctx, cmd)
		r.Logger.Info("pipeline started",
			slog.String("pipeline_id", cmd.PipelineID),
			slog.String("stream_id", cmd.StreamID),
		)
	case "stop":
		r.mu.Lock()
		if cancel, ok := r.running[key]; ok {
			cancel()
			delete(r.running, key)
		}
		r.mu.Unlock()
		r.Logger.Info("pipeline stopped", slog.String("key", key))
	default:
		r.Logger.Warn("unknown op", slog.String("op", cmd.Op))
	}
}

// runOne builds and runs the operator chain for one (pipeline, stream) tuple.
// The walking-skeleton chain is fixed; Phase 2 reads the DAG from the
// pipeline-service Pipeline resource and instantiates operators by name.
func (r *Runner) runOne(ctx context.Context, cmd Command) {
	ring, err := r.Registry.Ensure(cmd.StreamID, 64)
	if err != nil {
		r.Logger.Error("ring ensure failed", slog.String("err", err.Error()))
		return
	}

	ops := []dataplane.Operator{
		operators.NewIngest(operators.IngestConfig{
			StreamID:   cmd.StreamID,
			PipelineID: cmd.PipelineID,
			Width:      640, Height: 360,
			FPS:   30,
			Total: 0, // unbounded; cancellation stops it
			Ring:  ring,
			Logger: r.Logger,
		}),
		operators.NewSampler(operators.SamplerConfig{SourceFPS: 30, TargetFPS: 15}),
		operators.NewDetect(operators.DetectConfig{
			Detector: operators.NewSyntheticDetector(),
			Ring:     ring,
			Logger:   r.Logger,
		}),
		operators.NewTracker(operators.TrackerConfig{IOUThreshold: 0.1}),
		operators.NewRule(operators.RuleConfig{
			Logger: r.Logger,
			Rules: []*operators.SpatialDwellRule{{
				RuleID:   "dwell-right-half",
				TenantID: cmd.TenantID,
				Zone:     operators.Zone{ID: "right-half", X: 0.5, Y: 0, W: 0.5, H: 1},
				Dwell:    2 * time.Second,
				Severity: commonv1.Severity_SEVERITY_HIGH,
			}},
		}),
		operators.NewEmit(operators.EmitConfig{Publisher: r.Publisher, Logger: r.Logger}),
	}

	p := &dataplane.Pipeline{
		ID:         cmd.PipelineID,
		Operators:  ops,
		BufferSize: 64,
		Logger:     r.Logger,
		Metrics:    r.Metrics,
	}
	source, wait := p.Run(ctx)

	go func() {
		<-ctx.Done()
		// Close the source so the ingest operator exits cleanly.
		defer func() { _ = recover() }()
		close(source)
	}()

	if err := wait(); err != nil && err != dataplane.ErrUpstreamClosed {
		r.Logger.Error("pipeline exited with error",
			slog.String("pipeline_id", cmd.PipelineID),
			slog.String("stream_id", cmd.StreamID),
			slog.String("err", err.Error()),
		)
	}
}

// Close drains every subscription. Cancel pipelines by canceling the context
// passed to Watch / WatchAll.
func (r *Runner) Close() error {
	r.mu.Lock()
	subs := r.subs
	r.subs = nil
	r.mu.Unlock()
	for _, s := range subs {
		_ = s.Drain()
	}
	return nil
}

// NewRunner constructs a Runner with the given dependencies and a fresh
// claim-check registry + Prometheus metrics.
func NewRunner(b bus.Subscriber, pub bus.Publisher, logger *slog.Logger, reg prometheus.Registerer) *Runner {
	return &Runner{
		Bus:       b,
		Publisher: pub,
		Logger:    logger,
		Registry:  claimcheck.NewRegistry(),
		Metrics:   dataplane.NewMetrics(reg),
		running:   map[string]context.CancelFunc{},
	}
}
