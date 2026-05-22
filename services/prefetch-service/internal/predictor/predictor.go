// Package predictor implements time-of-week tenant model demand
// prediction (ADR-0026).
//
// The model is intentionally simple: a 7×24 grid of (day-of-week, hour)
// → exponentially-weighted moving average of distinct (tenant, model)
// pairs observed in that hour. Predictions for `now + horizon` come from
// the grid cell that `now + horizon` lands in.
//
// We deliberately avoid pulling in a learning library here. Linear EMA
// over a tiny grid is well-behaved, has constant memory, and produces
// recommendations stable enough for a cache warmer. If we needed sharper
// predictions, the right next step is a per-tenant ARMA model — not a
// neural net.
package predictor

import (
	"sort"
	"sync"
	"time"
)

// Key identifies a (tenant, model) target the predictor tracks.
type Key struct {
	Tenant string
	Model  string
}

type cell struct {
	demand map[Key]float64 // EMA of count
}

// Predictor is goroutine-safe.
type Predictor struct {
	mu     sync.Mutex
	grid   [7][24]*cell
	alpha  float64 // EMA factor (0..1); higher = more reactive
	now    func() time.Time
}

// New constructs a predictor with the given EMA alpha (0.2 is a good
// starting point — 5-week half-life on hourly cells).
func New(alpha float64) *Predictor {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.2
	}
	p := &Predictor{alpha: alpha, now: time.Now}
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			p.grid[d][h] = &cell{demand: map[Key]float64{}}
		}
	}
	return p
}

// SetNow is exposed for tests.
func (p *Predictor) SetNow(f func() time.Time) { p.mu.Lock(); p.now = f; p.mu.Unlock() }

// Observe records that (tenant, model) had n events in the cell
// containing t. Call once per epoch (e.g. once per hour, summing all
// observations for that hour).
func (p *Predictor) Observe(t time.Time, k Key, n float64) {
	d, h := slot(t)
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.grid[d][h]
	prev := c.demand[k]
	c.demand[k] = (1-p.alpha)*prev + p.alpha*n
}

// Bump is a convenience: increments the current cell's running total
// (use when you want to accumulate live + apply EMA later).
func (p *Predictor) Bump(t time.Time, k Key) {
	d, h := slot(t)
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.grid[d][h]
	c.demand[k] = c.demand[k] + 1
}

// Predict returns the top-N (Key, expected demand) pairs for the
// horizon-ahead cell of `now`.
func (p *Predictor) Predict(horizon time.Duration, topN int) []Prediction {
	t := p.now().Add(horizon)
	d, h := slot(t)
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.grid[d][h]
	out := make([]Prediction, 0, len(c.demand))
	for k, v := range c.demand {
		out = append(out, Prediction{Key: k, Expected: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Expected > out[j].Expected })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// Prediction is one row of the recommendation list.
type Prediction struct {
	Key      Key
	Expected float64
}

func slot(t time.Time) (int, int) {
	t = t.UTC()
	return int(t.Weekday()), t.Hour()
}
