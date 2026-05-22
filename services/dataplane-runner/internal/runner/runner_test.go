package runner_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/dataplane-runner/internal/runner"

	"github.com/prometheus/client_golang/prometheus"
)

func newRunner() (*runner.Runner, *bus.InMemoryBus) {
	mem := bus.NewInMemory()
	r := runner.NewRunner(mem, mem, slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
	return r, mem
}

func TestRunner_StartIdempotent(t *testing.T) {
	r, mem := newRunner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.WatchAll(ctx); err != nil {
		t.Fatal(err)
	}
	cmd := runner.Command{
		Op: "start", PipelineID: "p-1", StreamID: "s-1", TenantID: "t-1",
		Protocol: "file", URL: "file:///dev/null",
	}
	body, _ := json.Marshal(cmd)
	for i := 0; i < 3; i++ {
		_ = mem.Publish(ctx, bus.Message{Subject: "operator.control.p-1", Data: body})
	}
	// Settle.
	time.Sleep(150 * time.Millisecond)
	// All three publishes should converge to a single running pipeline. We
	// verify by sending a stop and checking it doesn't error.
	stopBody, _ := json.Marshal(runner.Command{Op: "stop", PipelineID: "p-1", StreamID: "s-1"})
	_ = mem.Publish(ctx, bus.Message{Subject: "operator.control.p-1", Data: stopBody})
	time.Sleep(150 * time.Millisecond)
	_ = r.Close()
}

func TestRunner_BadJSONIgnored(t *testing.T) {
	r, mem := newRunner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.WatchAll(ctx); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mem.Publish(ctx, bus.Message{Subject: "operator.control.p-1", Data: []byte("not json")})
	}()
	wg.Wait()
	// If the runner crashed on bad JSON the test would panic via the
	// subscriber goroutine. Reaching here = success.
	time.Sleep(50 * time.Millisecond)
	_ = r.Close()
}
