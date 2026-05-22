package autonomy_test

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aegisvision/pkg/autonomy"
	"github.com/aegisvision/pkg/intelligence"
)

func TestParseCron_Valid(t *testing.T) {
	cases := []string{
		"0 0 * * * *",
		"*/15 * * * * *",
		"0 30 12 1 1 *",
		"0 0,15,30,45 * * * *",
	}
	for _, s := range cases {
		if _, err := autonomy.ParseCron(s); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
}

func TestParseCron_Invalid(t *testing.T) {
	bad := []string{
		"0 0 * * *",        // 5 fields
		"foo bar baz x y z",
		"99 0 * * * *",     // sec out of range
		"*/0 * * * * *",    // zero step
	}
	for _, s := range bad {
		if _, err := autonomy.ParseCron(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestCron_NextEveryMinute(t *testing.T) {
	c, _ := autonomy.ParseCron("0 * * * * *")
	// 12:34:56 → next at-tick should be 12:35:00.
	now := time.Date(2026, 5, 21, 12, 34, 56, 0, time.UTC)
	next := c.Next(now)
	want := time.Date(2026, 5, 21, 12, 35, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestCron_NextEveryFiveSeconds(t *testing.T) {
	c, _ := autonomy.ParseCron("*/5 * * * * *")
	now := time.Date(2026, 5, 21, 12, 34, 3, 0, time.UTC)
	next := c.Next(now)
	want := time.Date(2026, 5, 21, 12, 34, 5, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestScheduler_FiresThenCloses(t *testing.T) {
	s := autonomy.NewScheduler()
	c, _ := autonomy.ParseCron("*/1 * * * * *")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fires atomic.Int64
	s.Register(ctx, &autonomy.Entry{ID: "e1", TenantID: "t-1", Cron: c}, func(_ context.Context, _ autonomy.Fire) error {
		fires.Add(1)
		return nil
	})
	// Allow ~2 seconds for at least one fire.
	time.Sleep(1500 * time.Millisecond)
	cancel()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer closeCancel()
	_ = s.Close(closeCtx)
	if fires.Load() == 0 {
		t.Fatal("expected at least one fire")
	}
}

func TestScheduler_PreventOverlap(t *testing.T) {
	s := autonomy.NewScheduler()
	c, _ := autonomy.ParseCron("*/1 * * * * *")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fires atomic.Int64
	s.Register(ctx, &autonomy.Entry{ID: "e1", TenantID: "t-1", Cron: c, PreventOverlap: true}, func(_ context.Context, _ autonomy.Fire) error {
		fires.Add(1)
		time.Sleep(3 * time.Second) // hold longer than tick interval
		return nil
	})
	time.Sleep(2200 * time.Millisecond)
	cancel()
	_ = s.Close(context.Background())
	if fires.Load() != 1 {
		t.Fatalf("expected exactly 1 fire (overlap-skipped), got %d", fires.Load())
	}
	if s.Skipped.Load() < 1 {
		t.Fatalf("expected at least 1 skip recorded, got %d", s.Skipped.Load())
	}
}

func TestKL_SymmetricAtSameDistribution(t *testing.T) {
	p := map[string]float64{"a": 0.5, "b": 0.3, "c": 0.2}
	if d := autonomy.KL(p, p); d > 1e-6 {
		t.Fatalf("KL(p,p) should be ~0, got %f", d)
	}
}

func TestJS_BoundedZeroToOne(t *testing.T) {
	p := map[string]float64{"a": 1.0}
	q := map[string]float64{"b": 1.0}
	d := autonomy.JS(p, q)
	if d < 0.99 || d > 1.0+1e-6 {
		t.Fatalf("JS for disjoint should be ~1, got %f", d)
	}
}

func TestTVD_SymmetricAndBounded(t *testing.T) {
	p := map[string]float64{"a": 0.7, "b": 0.3}
	q := map[string]float64{"a": 0.5, "b": 0.5}
	d := autonomy.TVD(p, q)
	if math.Abs(d-0.2) > 1e-6 {
		t.Fatalf("TVD expected 0.2, got %f", d)
	}
}

func TestDivergence_Dispatches(t *testing.T) {
	p := map[string]float64{"a": 0.5, "b": 0.5}
	q := map[string]float64{"a": 0.4, "b": 0.6}
	if autonomy.Divergence(intelligence.DivergenceKL, p, q) == 0 {
		t.Fatal("KL should not be zero for different distributions")
	}
	if autonomy.Divergence(intelligence.DivergenceJS, p, q) == 0 {
		t.Fatal("JS should not be zero for different distributions")
	}
	if autonomy.Divergence(intelligence.DivergenceTVD, p, q) == 0 {
		t.Fatal("TVD should not be zero for different distributions")
	}
}

func TestBurnRate_AndMultiWindow(t *testing.T) {
	// SLO target 99.9%; observed 99.5% → loss 0.5%; budget 0.1%; burn = 5.
	if br := autonomy.BurnRate(0.995, 0.999); math.Abs(br-5) > 1e-6 {
		t.Fatalf("burn rate: %f", br)
	}
	// Catastrophic burn.
	if sev := autonomy.MultiWindow(20, 8, 2); sev != "critical" {
		t.Fatalf("expected critical, got %s", sev)
	}
	// Warn.
	if sev := autonomy.MultiWindow(7, 4, 1.5); sev != "warn" {
		t.Fatalf("expected warn, got %s", sev)
	}
	// Healthy.
	if sev := autonomy.MultiWindow(0.5, 0.5, 0.5); sev != "" {
		t.Fatalf("expected healthy, got %s", sev)
	}
}

func TestSignalRouter_RoutesToLongestPrefix(t *testing.T) {
	r := autonomy.NewSignalRouter()
	var driftHits, anyHits atomic.Int64
	r.Register("autonomy.signal", func(_ context.Context, _ autonomy.Fire) error { anyHits.Add(1); return nil })
	r.Register("autonomy.signal.drift", func(_ context.Context, _ autonomy.Fire) error { driftHits.Add(1); return nil })
	if !r.Route(context.Background(), "autonomy.signal.drift", "t-1", "s-1") {
		t.Fatal("expected accept")
	}
	if driftHits.Load() != 1 || anyHits.Load() != 0 {
		t.Fatalf("hits: drift=%d any=%d", driftHits.Load(), anyHits.Load())
	}
}
