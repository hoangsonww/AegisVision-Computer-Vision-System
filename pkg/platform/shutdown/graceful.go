// Package shutdown coordinates graceful termination across a service.
//
// On SIGINT/SIGTERM the runner stops accepting new work, waits for in-flight
// work to finish (up to GracePeriod), and calls every registered Closer in
// LIFO order. This matches the Kubernetes preStop + terminationGracePeriod
// contract: SIGTERM arrives, we flip readiness to red, drain, then exit.
package shutdown

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Closer is anything that needs to release resources at shutdown — an HTTP
// server's Shutdown, a Kafka producer's Close, a Temporal client's Close, an
// OTel tracer provider's Shutdown.
type Closer func(context.Context) error

// Runner accumulates closers and runs the shutdown sequence on signal.
type Runner struct {
	mu          sync.Mutex
	closers     []namedCloser
	GracePeriod time.Duration
	Logger      *slog.Logger
}

type namedCloser struct {
	name   string
	closer Closer
}

// New returns a Runner with sensible defaults.
func New(logger *slog.Logger) *Runner {
	return &Runner{GracePeriod: 25 * time.Second, Logger: logger}
}

// Register adds a Closer. Closers run in LIFO order so dependencies that were
// constructed last are torn down first.
func (r *Runner) Register(name string, c Closer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closers = append(r.closers, namedCloser{name: name, closer: c})
}

// Wait blocks until SIGINT/SIGTERM (or ctx cancellation) and then runs every
// registered closer with the runner's GracePeriod budget. It returns the
// first non-nil error from any closer; the rest still run.
func (r *Runner) Wait(ctx context.Context) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	select {
	case s := <-sig:
		if r.Logger != nil {
			r.Logger.Info("shutdown signal received", slog.String("signal", s.String()))
		}
	case <-ctx.Done():
		if r.Logger != nil {
			r.Logger.Info("shutdown via context cancel")
		}
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), r.GracePeriod)
	defer cancel()

	r.mu.Lock()
	cs := make([]namedCloser, len(r.closers))
	copy(cs, r.closers)
	r.mu.Unlock()

	var firstErr error
	for i := len(cs) - 1; i >= 0; i-- {
		c := cs[i]
		err := c.closer(closeCtx)
		if err != nil && r.Logger != nil {
			r.Logger.Error("closer failed", slog.String("name", c.name), slog.String("err", err.Error()))
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if errors.Is(closeCtx.Err(), context.DeadlineExceeded) && firstErr == nil {
		firstErr = closeCtx.Err()
	}
	return firstErr
}
