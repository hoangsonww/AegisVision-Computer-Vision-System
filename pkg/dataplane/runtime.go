// Pipeline runtime: wires operators into a linear chain with bounded
// channels between them. Returns the head of the chain (the channel the
// source feeds into) and a wait function that blocks until every operator
// exits or ctx is canceled.
package dataplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Pipeline is a configured chain of operators ready to run.
type Pipeline struct {
	ID         string
	Operators  []Operator
	BufferSize int // bounded channel capacity between operators

	Logger  *slog.Logger
	Metrics *Metrics
}

// Run executes every operator in its own goroutine. Returns the SOURCE
// channel (write here to feed the chain) and a function that blocks until
// every operator has returned. The caller closes the source channel to
// initiate graceful end-of-stream; cancelling ctx is the abort path.
func (p *Pipeline) Run(ctx context.Context) (chan<- *FrameEnvelope, func() error) {
	if p.BufferSize <= 0 {
		p.BufferSize = 32
	}
	if p.Logger == nil {
		p.Logger = slog.Default()
	}

	// One channel per inter-operator edge; the first edge is the source.
	chans := make([]chan *FrameEnvelope, len(p.Operators)+1)
	for i := range chans {
		chans[i] = make(chan *FrameEnvelope, p.BufferSize)
	}

	var wg sync.WaitGroup
	errs := make([]error, len(p.Operators))

	for i, op := range p.Operators {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(chans[i+1])
			in := chans[i]
			out := chans[i+1]

			// Wrap the input channel to record queue depth + per-op latency.
			instr := make(chan *FrameEnvelope, p.BufferSize)
			go func() {
				defer close(instr)
				for env := range in {
					if p.Metrics != nil {
						p.Metrics.QueueDepth.WithLabelValues(p.ID, op.Name()).Set(float64(len(in)))
					}
					instr <- env
				}
			}()

			// Wrap the output channel to record processed counter + frame timestamp.
			outwrap := make(chan *FrameEnvelope, p.BufferSize)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for env := range outwrap {
					if p.Metrics != nil {
						p.Metrics.FramesProcessed.WithLabelValues(p.ID, op.Name()).Inc()
					}
					out <- env
				}
			}()

			start := time.Now()
			err := op.Run(ctx, instr, outwrap)
			close(outwrap)
			<-done
			if p.Metrics != nil {
				p.Metrics.OperatorLatency.WithLabelValues(p.ID, op.Name()).Observe(time.Since(start).Seconds())
			}
			if err != nil && !errors.Is(err, ErrUpstreamClosed) {
				errs[i] = fmt.Errorf("operator %s: %w", op.Name(), err)
				p.Logger.Error("operator failed",
					slog.String("operator", op.Name()),
					slog.String("err", err.Error()),
				)
			}
		}()
	}

	// Drain the tail so the final close propagates cleanly.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range chans[len(p.Operators)] {
		}
	}()

	wait := func() error {
		wg.Wait()
		for _, e := range errs {
			if e != nil {
				return e
			}
		}
		return nil
	}
	return chans[0], wait
}
