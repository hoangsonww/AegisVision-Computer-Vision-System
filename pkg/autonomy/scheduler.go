package autonomy

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Fire is what the scheduler delivers to a registered handler.
type Fire struct {
	ScheduleID string
	TenantID   string
	At         time.Time
	// Reason is "cron tick" for scheduled fires, "signal:<subject>" for
	// signal-driven, or "manual" for ad-hoc.
	Reason string
}

// Handler receives a Fire. Returning an error is logged and counted; the
// scheduler does not retry — the next tick is its own event.
type Handler func(ctx context.Context, f Fire) error

// Entry is one schedule registered with the Scheduler.
type Entry struct {
	ID       string
	TenantID string
	Cron     Cron
	// MaxJitter caps the random delay applied to each fire to spread load.
	// Defaults to 0 (no jitter).
	MaxJitter time.Duration
	// PreventOverlap=true makes the scheduler skip a tick when the prior
	// handler hasn't returned yet.
	PreventOverlap bool

	handler  Handler
	running  atomic.Bool
}

// Scheduler runs registered Entries until Close().
type Scheduler struct {
	mu     sync.Mutex
	rng    *rand.Rand
	wg     sync.WaitGroup
	cancel context.CancelFunc
	now    func() time.Time
	// Skipped/fired counters for tests + metrics.
	Fired   atomic.Int64
	Skipped atomic.Int64
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		now: time.Now,
	}
}

// SetNow overrides the time source (for tests).
func (s *Scheduler) SetNow(f func() time.Time) { s.mu.Lock(); s.now = f; s.mu.Unlock() }

// Register starts a goroutine that fires the handler at the cron's
// ticks. The supplied ctx scopes the entry's lifetime.
func (s *Scheduler) Register(ctx context.Context, e *Entry, h Handler) {
	e.handler = h
	s.wg.Add(1)
	go s.loop(ctx, e)
}

// Close cancels all entries and waits for them to settle.
func (s *Scheduler) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) loop(ctx context.Context, e *Entry) {
	defer s.wg.Done()
	for {
		now := s.now()
		next := e.Cron.Next(now)
		if next.IsZero() {
			return // unreachable schedule — exit cleanly
		}
		delay := next.Sub(now)
		if e.MaxJitter > 0 {
			s.mu.Lock()
			j := time.Duration(s.rng.Int63n(int64(e.MaxJitter)))
			s.mu.Unlock()
			delay += j
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		// Overlap guard.
		if e.PreventOverlap && e.running.Load() {
			s.Skipped.Add(1)
			continue
		}
		e.running.Store(true)
		fire := Fire{
			ScheduleID: e.ID,
			TenantID:   e.TenantID,
			At:         s.now(),
			Reason:     "cron tick",
		}
		go func() {
			defer e.running.Store(false)
			_ = e.handler(ctx, fire)
		}()
		s.Fired.Add(1)
	}
}

// SignalRouter dispatches signal-triggered fires to registered handlers
// keyed by subject prefix. The autonomy-orchestrator wires this to its
// bus subscription.
type SignalRouter struct {
	mu       sync.RWMutex
	handlers map[string]Handler // subject prefix → handler
}

func NewSignalRouter() *SignalRouter { return &SignalRouter{handlers: map[string]Handler{}} }

func (r *SignalRouter) Register(subjectPrefix string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[subjectPrefix] = h
}

// Route finds the longest-prefix match for subject and invokes its handler.
// Returns true if a handler accepted the signal.
func (r *SignalRouter) Route(ctx context.Context, subject string, tenantID, scheduleID string) bool {
	r.mu.RLock()
	var bestPrefix string
	var best Handler
	for p, h := range r.handlers {
		if startsWith(subject, p) && len(p) > len(bestPrefix) {
			bestPrefix = p
			best = h
		}
	}
	r.mu.RUnlock()
	if best == nil {
		return false
	}
	_ = best(ctx, Fire{
		ScheduleID: scheduleID, TenantID: tenantID,
		At: time.Now(), Reason: "signal:" + subject,
	})
	return true
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
